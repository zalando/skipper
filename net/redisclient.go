package net

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/opentracing/opentracing-go"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"

	xxhash "github.com/cespare/xxhash/v2"
	rendezvous "github.com/dgryski/go-rendezvous"

	jump "github.com/dgryski/go-jump"

	"github.com/dchest/siphash"
	mpchash "github.com/dgryski/go-mpchash"
)

// RedisOptions is used to configure the redis.Ring
type RedisOptions struct {
	// Addrs are the list of redis shards
	Addrs []string

	// AddrUpdater is a func that is regularly called to update
	// redis address list. This func should return a list of redis
	// shards.
	AddrUpdater func() ([]string, error)

	// UpdateInterval is the time.Duration that AddrUpdater is
	// triggered and SetAddrs be used to update the redis shards
	UpdateInterval time.Duration

	// Username used to connect to the Redis server
	Username string
	// Password is the password needed to connect to Redis server
	Password string

	// ReadTimeout for redis socket reads
	ReadTimeout time.Duration
	// WriteTimeout for redis socket writes
	WriteTimeout time.Duration
	// DialTimeout is the max time.Duration to dial a new connection
	DialTimeout time.Duration

	// PoolTimeout is the max time.Duration to get a connection from pool
	PoolTimeout time.Duration
	// IdleTimeout requires a non 0 IdleCheckFrequency
	IdleTimeout time.Duration
	// IdleCheckFrequency - reaper frequency, only used if IdleTimeout > 0
	IdleCheckFrequency time.Duration
	// MaxConnAge
	MaxConnAge time.Duration
	// MinIdleConns is the minimum number of socket connections to redis
	MinIdleConns int
	// MaxIdleConns is the maximum number of socket connections to redis
	MaxIdleConns int

	// HeartbeatFrequency frequency of PING commands sent to check
	// shards availability.
	HeartbeatFrequency time.Duration

	// ConnMetricsInterval defines the frequency of updating the redis
	// connection related metrics. Defaults to 60 seconds.
	ConnMetricsInterval time.Duration
	// MetricsPrefix is the prefix for redis ring client metrics,
	// defaults to "swarm.redis." if not set
	MetricsPrefix string
	// Tracer provides OpenTracing for Redis queries.
	Tracer opentracing.Tracer
	// Log is the logger that is used
	Log logging.Logger

	// HashAlgorithm is one of rendezvous, rendezvousVnodes, jump, mpchash, defaults to github.com/go-redis/redis default
	HashAlgorithm string
}

// RedisRingClient is a redis client that does access redis by
// computing a ring hash. It logs to the logging.Logger interface,
// that you can pass. It adds metrics and operations are traced with
// opentracing. You can set timeouts and the defaults are set to be ok
// to be in the hot path of low latency production requests.
type RedisRingClient struct {
	ring          *redis.Ring
	log           logging.Logger
	metrics       metrics.Metrics
	metricsPrefix string
	options       *RedisOptions
	tracer        opentracing.Tracer
	quit          chan struct{}
	once          sync.Once
	closed        bool
}

type RedisScript struct {
	script *redis.Script
}

const (
	// DefaultReadTimeout is the default socket read timeout
	DefaultReadTimeout = 25 * time.Millisecond
	// DefaultWriteTimeout is the default socket write timeout
	DefaultWriteTimeout = 25 * time.Millisecond
	// DefaultPoolTimeout is the default timeout to access the connection pool
	DefaultPoolTimeout = 25 * time.Millisecond
	// DefaultDialTimeout is the default dial timeout
	DefaultDialTimeout = 25 * time.Millisecond
	// DefaultMinConns is the default minimum of connections
	DefaultMinConns = 100
	// DefaultMaxConns is the default maximum of connections
	DefaultMaxConns = 100

	defaultConnMetricsInterval = 60 * time.Second
	DefaultUpdateInterval      = 10 * time.Second
	DefaultHeartbeatFrequency  = 500 * time.Millisecond // https://github.com/redis/go-redis/blob/452eb3d15f9ccdb8e4ed3876cafc88c3d35e0e13/ring.go#L167C28-L167C50
)

// https://arxiv.org/pdf/1406.2294.pdf
type jumpHash struct {
	shards []string
}

func NewJumpHash(shards []string) redis.ConsistentHash {
	return &jumpHash{
		shards: shards,
	}
}

func (j *jumpHash) Get(k string) string {
	key := xxhash.Sum64String(k)
	h := jump.Hash(key, len(j.shards))
	return j.shards[int(h)]
}

// Multi-probe consistent hashing - mpchash
// https://arxiv.org/pdf/1505.00062.pdf
type multiprobe struct {
	hash *mpchash.Multi
}

func NewMultiprobe(shards []string) redis.ConsistentHash {
	return &multiprobe{
		// 2 seeds and k=21 got from library
		hash: mpchash.New(shards, siphash64seed, [2]uint64{1, 2}, 21),
	}
}

func (mc *multiprobe) Get(k string) string {
	return mc.hash.Hash(k)
}
func siphash64seed(b []byte, s uint64) uint64 { return siphash.Hash(s, 0, b) }

// rendezvous copied from github.com/go-redis/redis/v8@v8.3.3/ring.go
type rendezvousWrapper struct {
	*rendezvous.Rendezvous
}

func (w rendezvousWrapper) Get(key string) string {
	return w.Lookup(key)
}

func NewRendezvous(shards []string) redis.ConsistentHash {
	return rendezvousWrapper{rendezvous.New(shards, xxhash.Sum64String)}
}

// rendezvous vnodes
type rendezvousVnodes struct {
	*rendezvous.Rendezvous
	table map[string]string
}

const vnodePerShard = 100

func (w rendezvousVnodes) Get(key string) string {
	k := w.Lookup(key)
	v, ok := w.table[k]
	if !ok {
		log.Printf("not found: %s in table for input: %s, so return %s", k, key, v)
	}
	return v
}

func NewRendezvousVnodes(shards []string) redis.ConsistentHash {
	vshards := make([]string, vnodePerShard*len(shards))
	table := make(map[string]string)
	for i := 0; i < vnodePerShard; i++ {
		for j, shard := range shards {
			vshard := fmt.Sprintf("%s%d", shard, i) // suffix
			table[vshard] = shard
			vshards[i*len(shards)+j] = vshard
		}
	}
	return rendezvousVnodes{rendezvous.New(vshards, xxhash.Sum64String), table}
}

func NewRedisRingClient(ro *RedisOptions) *RedisRingClient {
	const backOffTime = 2 * time.Second
	const retryCount = 5
	r := &RedisRingClient{
		once:    sync.Once{},
		quit:    make(chan struct{}),
		metrics: metrics.Default,
		tracer:  &opentracing.NoopTracer{},
	}

	ringOptions := &redis.RingOptions{
		Addrs: map[string]string{},
		NewClient: func(opt *redis.Options) *redis.Client {
			if ro != nil {
				opt.Username = ro.Username
				opt.Password = ro.Password
				opt.ReadTimeout = ro.ReadTimeout
				opt.WriteTimeout = ro.WriteTimeout
				opt.PoolTimeout = ro.PoolTimeout
				opt.DialTimeout = ro.DialTimeout
				opt.MinIdleConns = ro.MinIdleConns
				opt.PoolSize = ro.MaxIdleConns

			}

			// https://github.com/redis/go-redis/issues/3536
			// Explicitly disable maintenance notifications
			// This prevents the client from sending CLIENT MAINT_NOTIFICATIONS ON
			opt.MaintNotificationsConfig = &maintnotifications.Config{
				Mode: maintnotifications.ModeDisabled,
			}

			return redis.NewClient(opt)
		},
	}
	if ro != nil {
		if ro.HeartbeatFrequency != 0 {
			ringOptions.HeartbeatFrequency = ro.HeartbeatFrequency
		}
		switch ro.HashAlgorithm {
		case "rendezvous":
			ringOptions.NewConsistentHash = NewRendezvous
		case "rendezvousVnodes":
			ringOptions.NewConsistentHash = NewRendezvousVnodes
		case "jump":
			ringOptions.NewConsistentHash = NewJumpHash
		case "mpchash":
			ringOptions.NewConsistentHash = NewMultiprobe
		}

		if ro.Log == nil {
			ro.Log = &logging.DefaultLog{}
		}

		if ro.AddrUpdater != nil {
			address, err := ro.AddrUpdater()
			for range retryCount {
				if err == nil {
					break
				}
				time.Sleep(backOffTime)
				address, err = ro.AddrUpdater()
			}
			if err != nil {
				ro.Log.Errorf("Failed at redisclient startup %v", err)
			}
			ringOptions.Addrs = createAddressMap(address)
		} else {
			ringOptions.Addrs = createAddressMap(ro.Addrs)
		}

		ro.Log.Infof("Created ring with addresses: %v", ro.Addrs)

		if ro.ConnMetricsInterval <= 0 {
			ro.ConnMetricsInterval = defaultConnMetricsInterval
		}
		if ro.Tracer != nil {
			r.tracer = ro.Tracer
		}

		r.options = ro
		r.ring = redis.NewRing(ringOptions)
		r.log = ro.Log
		r.metricsPrefix = ro.MetricsPrefix

		if ro.AddrUpdater != nil {
			if ro.UpdateInterval == 0 {
				ro.UpdateInterval = DefaultUpdateInterval
			}
			go r.startUpdater(context.Background())
		}
	}

	return r
}

func createAddressMap(addrs []string) map[string]string {
	res := make(map[string]string)
	for _, addr := range addrs {
		res[addr] = addr
	}
	return res
}

func hasAll(a []string, set map[string]struct{}) bool {
	if len(a) != len(set) {
		return false
	}
	for _, w := range a {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

func (r *RedisRingClient) startUpdater(ctx context.Context) {
	old := make(map[string]struct{})
	for _, addr := range r.options.Addrs {
		old[addr] = struct{}{}
	}

	r.log.Infof("Start goroutine to update redis instances every %s", r.options.UpdateInterval)
	defer r.log.Info("Stopped goroutine to update redis")

	time.Sleep(time.Duration(rand.Int63n(int64(r.options.UpdateInterval))))
	ticker := time.NewTicker(r.options.UpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.quit:
			return
		default:
		}

		addrs, err := r.options.AddrUpdater()
		if err == nil {
			if !hasAll(addrs, old) {
				r.log.Infof("Redis updater updating old(%d) != new(%d)", len(old), len(addrs))
				r.SetAddrs(ctx, addrs)
				r.metrics.UpdateGauge(r.metricsPrefix+"shards", float64(r.ring.Len()))

				old = make(map[string]struct{})
				for _, addr := range addrs {
					old[addr] = struct{}{}
				}
			}
		} else {
			r.log.Errorf("Failed to update redis addresses: %v", err)
		}

		select {
		case <-r.quit:
			return
		case <-ticker.C:
		}
	}
}

func (r *RedisRingClient) RingAvailable() bool {
	var err error
	err = backoff.Retry(func() error {
		_, err = r.ring.Ping(context.Background()).Result()
		if err != nil {
			r.log.Infof("Failed to ping redis, retry with backoff: %v", err)
		}
		return err
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 7))

	return err == nil
}

func (r *RedisRingClient) StartMetricsCollection() {
	go func() {
		ticker := time.NewTicker(r.options.ConnMetricsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := r.ring.PoolStats()
				// counter values
				r.metrics.UpdateGauge(r.metricsPrefix+"hits", float64(stats.Hits))
				r.metrics.UpdateGauge(r.metricsPrefix+"misses", float64(stats.Misses))
				r.metrics.UpdateGauge(r.metricsPrefix+"timeouts", float64(stats.Timeouts))
				// counter of reaped staleconns which were closed
				r.metrics.UpdateGauge(r.metricsPrefix+"staleconns", float64(stats.StaleConns))

				// gauges
				r.metrics.UpdateGauge(r.metricsPrefix+"idleconns", float64(stats.IdleConns))
				r.metrics.UpdateGauge(r.metricsPrefix+"totalconns", float64(stats.TotalConns))
			case <-r.quit:
				return
			}
		}
	}()
}

func (r *RedisRingClient) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	return r.tracer.StartSpan(operationName, opts...)
}

func (r *RedisRingClient) Close() {
	r.once.Do(func() {
		r.closed = true
		close(r.quit)
		if r.ring != nil {
			r.ring.Close()
		}
	})
}

func (r *RedisRingClient) SetAddrs(ctx context.Context, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	r.ring.SetAddrs(createAddressMap(addrs))
}

func (r *RedisRingClient) Get(ctx context.Context, key string) (string, error) {
	res := r.ring.Get(ctx, key)
	return res.Val(), res.Err()
}

func (r *RedisRingClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) (string, error) {
	res := r.ring.Set(ctx, key, value, expiration)
	return res.Result()
}

func (r *RedisRingClient) ZAdd(ctx context.Context, key string, val int64, score float64) (int64, error) {
	res := r.ring.ZAdd(ctx, key, redis.Z{Member: val, Score: score})
	return res.Val(), res.Err()
}

func (r *RedisRingClient) ZRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	res := r.ring.ZRem(ctx, key, members...)
	return res.Val(), res.Err()
}

func (r *RedisRingClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	res := r.ring.Expire(ctx, key, expiration)
	return res.Val(), res.Err()
}

func (r *RedisRingClient) ZRemRangeByScore(ctx context.Context, key string, min, max float64) (int64, error) {
	res := r.ring.ZRemRangeByScore(ctx, key, fmt.Sprint(min), fmt.Sprint(max))
	return res.Val(), res.Err()
}

func (r *RedisRingClient) ZCard(ctx context.Context, key string) (int64, error) {
	res := r.ring.ZCard(ctx, key)
	return res.Val(), res.Err()
}

func (r *RedisRingClient) ZRangeByScoreWithScoresFirst(ctx context.Context, key string, min, max float64, offset, count int64) (interface{}, error) {
	opt := redis.ZRangeBy{
		Min:    fmt.Sprint(min),
		Max:    fmt.Sprint(max),
		Offset: offset,
		Count:  count,
	}
	res := r.ring.ZRangeByScoreWithScores(ctx, key, &opt)
	zs, err := res.Result()
	if err != nil {
		return nil, err
	}
	if len(zs) == 0 {
		return nil, nil
	}

	return zs[0].Member, nil
}

func (r *RedisRingClient) NewScript(source string) *RedisScript {
	return &RedisScript{redis.NewScript(source)}
}

func (r *RedisRingClient) RunScript(ctx context.Context, s *RedisScript, keys []string, args ...interface{}) (interface{}, error) {
	return s.script.Run(ctx, r.ring, keys, args...).Result()
}
