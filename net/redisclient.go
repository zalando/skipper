package net

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"log"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
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

	// HashAlgorithm is one of rendezvousVnodes, jump, mpchash, defaults to rendezvous which is chosen by github.com/go-redis/redis
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
)

// jump
// https://arxiv.org/pdf/1406.2294.pdf
type JumpHash struct {
	hash   hash.Hash64
	shards []string
}

func NewJumpHash(shards []string) redis.ConsistentHash {
	return &JumpHash{
		hash:   fnv.New64(),
		shards: shards,
	}
}

func (j *JumpHash) Get(k string) string {
	_, err := j.hash.Write([]byte(k))
	if err != nil {
		log.Fatalf("Failed to write %s to hash: %v", k, err)
	}

	key := j.hash.Sum64()
	h := jump.Hash(key, len(j.shards)) //func Hash(key uint64, numBuckets int) int32
	//fmt.Printf("h: %d\n", h)
	return j.shards[int(h)%len(j.shards)]
}

// Multi-probe consistent hashing - mpchash
// https://arxiv.org/pdf/1505.00062.pdf
type multiprobe struct {
	hash   *mpchash.Multi
	shards []string
}

func NewMultiprobe(shards []string) redis.ConsistentHash {
	return &multiprobe{
		// 2 seeds and k=21
		hash: mpchash.New(shards, siphash64seed, [2]uint64{1, 2}, 21),
		// 2 seeds and k=41
		//hash:   mpchash.New(shards, siphash64seed, [2]uint64{1, 2}, 41),
		shards: shards,
	}
}

func (mc *multiprobe) Get(k string) string {
	return mc.hash.Hash(k)
}
func siphash64seed(b []byte, s uint64) uint64 { return siphash.Hash(s, 0, b) }

// rendezvous vnodes
type rendezvousVnodes struct {
	*rendezvous.Rendezvous
	table map[string]string
}

func (w rendezvousVnodes) Get(key string) string {
	k := w.Lookup(key)
	v, ok := w.table[k]
	if !ok {
		log.Printf("not found: %s in table for input: %s, so return %s", k, key, v)
	}
	return v
}

func NewRendezvousVnodes(shards []string) redis.ConsistentHash {
	N := 100
	vshards := make([]string, N*len(shards), N*len(shards))
	table := make(map[string]string)
	for i := 0; i < N; i++ {
		for j, shard := range shards {
			//vshard := fmt.Sprintf("%d%s", i, shard) // prefix
			vshard := fmt.Sprintf("%s%d", shard, i) // suffix
			table[vshard] = shard
			vshards[i*len(shards)+j] = vshard
		}
	}
	return rendezvousVnodes{rendezvous.New(vshards, xxhash.Sum64String), table}
}

func NewRedisRingClient(ro *RedisOptions) *RedisRingClient {
	r := new(RedisRingClient)
	r.quit = make(chan struct{})
	r.metrics = metrics.Default
	r.tracer = &opentracing.NoopTracer{}

	ringOptions := &redis.RingOptions{
		Addrs: map[string]string{},
	}
	switch ro.HashAlgorithm {
	case "rendezvousVnodes":
		ringOptions.NewConsistentHash = NewRendezvousVnodes
	case "jump":
		ringOptions.NewConsistentHash = NewJumpHash
	case "mpchash":
		ringOptions.NewConsistentHash = NewMultiprobe
	}

	if ro != nil {
		for idx, addr := range ro.Addrs {
			ringOptions.Addrs[fmt.Sprintf("redis%d", idx)] = addr
		}
		ringOptions.ReadTimeout = ro.ReadTimeout
		ringOptions.WriteTimeout = ro.WriteTimeout
		ringOptions.PoolTimeout = ro.PoolTimeout
		ringOptions.DialTimeout = ro.DialTimeout
		ringOptions.MinIdleConns = ro.MinIdleConns
		ringOptions.PoolSize = ro.MaxIdleConns
		ringOptions.Password = ro.Password

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
	}

	return r
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
		for {
			select {
			case <-time.After(r.options.ConnMetricsInterval):
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
	if r != nil {
		close(r.quit)
	}
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
	res := r.ring.ZAdd(ctx, key, &redis.Z{Member: val, Score: score})
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
	opt := &redis.ZRangeBy{
		Min:    fmt.Sprint(min),
		Max:    fmt.Sprint(max),
		Offset: offset,
		Count:  count,
	}
	res := r.ring.ZRangeByScoreWithScores(ctx, key, opt)
	zs, err := res.Result()
	if err != nil {
		return nil, err
	}
	if len(zs) == 0 {
		return nil, nil
	}

	return zs[0].Member, nil
}
