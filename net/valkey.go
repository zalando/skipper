package net

import (
	"context"
	"strings"
	"sync"
	"time"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"

	"github.com/opentracing/opentracing-go"
	"github.com/valkey-io/valkey-go"
)

const ringSize = 10000

func NewScript(sourcs string) *valkey.Lua {
	return valkey.NewLuaScript(sourcs)
}

// ValkeyOptions is used to configure the ValkeyRing
//
// Many options are named like
// https://pkg.go.dev/github.com/valkey-io/valkey-go#ClientOption,
// which we pass to the valkey.Client on creation
type ValkeyOptions struct {
	// Addrs are the list of valkey shards
	Addrs []string

	// AddrUpdater is a func that is regularly called to update
	// valkey address list. This func should return a list of valkey
	// shards.
	AddrUpdater func() ([]string, error)

	// UpdateInterval is the time.Duration that AddrUpdater is
	// triggered and SetAddrs be used to update the valkey shards
	UpdateInterval time.Duration

	// Username used to connect to the Valkey server
	Username string
	// Password is the password needed to connect to Valkey server
	Password string

	// ConnWriteTimeout for valkey socket read,write,dial timeouts https://pkg.go.dev/github.com/valkey-io/valkey-go#ClientOption
	ConnWriteTimeout time.Duration

	// ConnLifetime connections will close after passing lifetime, see https://pkg.go.dev/github.com/valkey-io/valkey-go#ClientOption
	ConnLifetime time.Duration

	// ConnMetricsInterval defines the frequency of updating the valkey
	// connection related metrics. Defaults to 60 seconds.
	ConnMetricsInterval time.Duration
	// MetricsPrefix is the prefix for valkey ring client metrics,
	// defaults to "swarm.valkey." if not set
	MetricsPrefix string
	// Tracer provides OpenTracing for Valkey queries.
	Tracer opentracing.Tracer
	// Log is the logger that is used
	Log logging.Logger

	// HashAlgorithm is one of rendezvous, rendezvousVnodes, jump, mpchash, defaults to rendezvous
	HashAlgorithm string
}

func createValkeyClient(addr string, opt *ValkeyOptions) (valkey.Client, error) {
	// TODO(sszuecs): OTel: use valkeyotel.NewClient instead
	// TODO(sszuecs): do we need a hook? https://github.com/valkey-io/valkey-go/tree/v1.0.69/valkeyhook
	cli, err := valkey.NewClient(valkey.ClientOption{
		Username:    opt.Username,
		Password:    opt.Password,
		InitAddress: []string{addr},

		ConnWriteTimeout: opt.ConnWriteTimeout, // Write,Read,Dial Timeout is the same
		ConnLifetime:     opt.ConnLifetime,

		MaxFlushDelay: 20 * time.Microsecond, // reduce CPU load without much impact, ref: https://github.com/redis/rueidis/issues/156

		DisableRetry: true,

		// DisableCache: true, // always use BlockingPool
		// think about maybe needed and what values?
		// BlockingPoolCleanup: 0,
		// BlockingPoolMinSize: 0,
		// BlockingPoolSize:    0,
		// BlockingPipeline:    0,
	})

	// TODO(sszuecs): do we want to have a similar interface as go-redis?
	// compat := valkeycompat.NewAdapter(client)
	// return compat,err

	return cli, err
}

type valkeyRing struct {
	opt *ValkeyOptions

	// maps int to addr for sharding, trades memory for concurrent access
	// worst case: 1 string full ipv6 addr, "[]", ":" and 5 digits port size: 39+2+1+5 = 47 byte, so 47*10000 = 470kB
	// TODO(sszuecs): maybe we can use [ringSize]valkey.Client as optimization
	// TODO(sszuecs): check how others are doing this (IIRC valkey-go code has something like this called "ring" for cluster mode).
	shards [ringSize]string

	mu        sync.RWMutex
	clientMap map[string]valkey.Client // map["10.5.1.43:6379"]valkey.Client
}

func newValkeyRing(opt *ValkeyOptions) (*valkeyRing, error) {
	ring := &valkeyRing{
		opt:       opt,
		clientMap: make(map[string]valkey.Client),
	}
	for _, ep := range opt.Addrs {
		cl, err := createValkeyClient(ep, opt)
		if err != nil {
			return nil, err
		}
		ring.mu.Lock()
		ring.clientMap[ep] = cl
		ring.mu.Unlock()
	}

	if len(opt.Addrs) == 0 {
		return ring, nil
	}

	cur := -1
	shardSize := computeShardSize(len(opt.Addrs))
	for i := range ringSize {
		if i%shardSize == 0 {
			cur++
		}
		ring.shards[i] = opt.Addrs[cur]
	}

	return ring, nil
}

func computeShardSize(i int) int {
	if i == 0 {
		return ringSize
	}
	return ringSize / i
}

func (vr *valkeyRing) Len() int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return len(vr.clientMap)
}

func (vr *valkeyRing) SetAddr(addr []string) error {
	current := make([]string, 0, len(vr.clientMap))

	vr.mu.RLock()
	for k := range vr.clientMap {
		current = append(current, k)
	}
	vr.mu.RUnlock()

	intersection := intersect(addr, current)
	if len(addr) == len(intersection) && len(current) == len(intersection) {
		return nil
	}

	newAddr := difference(addr, intersection)
	oldAddr := difference(current, intersection)

	// create new clients for newAddr
	for _, addr := range newAddr {
		cli, err := createValkeyClient(addr, vr.opt)
		if err != nil {
			return err
		}
		vr.mu.Lock()
		vr.clientMap[addr] = cli
		vr.mu.Unlock()
	}

	// close old clients
	for _, addr := range oldAddr {
		vr.mu.Lock()
		cli, ok := vr.clientMap[addr]
		delete(vr.clientMap, addr)
		vr.mu.Unlock()
		if ok {
			cli.Close()
		}
	}
	return nil
}

func (vr *valkeyRing) ShardForKey(key string) valkey.Client {
	i := xxhash.Sum64String(key)
	shard := vr.shards[i%ringSize]

	vr.mu.RLock()
	cli := vr.clientMap[shard]
	vr.mu.RUnlock()

	return cli
}

// PingAll TODO(sszuecs): is slow if we need to use it anywhere else than tests we have to optimize it
func (vr *valkeyRing) PingAll(ctx context.Context) map[string]valkey.ValkeyResult {
	res := make(map[string]valkey.ValkeyResult)
	logrus.Infof("vr.clientMap: %d", len(vr.clientMap))
	vr.mu.RLock()
	for k, cli := range vr.clientMap {
		res[k] = cli.Do(ctx, cli.B().Ping().Build())
	}
	vr.mu.RUnlock()
	return res
}

func (vr *valkeyRing) Ping(ctx context.Context, shard string) error {
	vr.mu.RLock()
	cli, ok := vr.clientMap[shard]
	vr.mu.RUnlock()
	if !ok {
		return nil
	}
	res := cli.Do(ctx, cli.B().Ping().Build())
	return res.Error()
}

func (vr *valkeyRing) Expire(ctx context.Context, key string, expire time.Duration) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Expire().Key(key).Seconds(int64(expire.Seconds())).Build())
}

func (vr *valkeyRing) Get(ctx context.Context, key string) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Get().Key(key).Build())
}

func (vr *valkeyRing) Set(ctx context.Context, key, val string) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Set().Key(key).Value(val).Build())
}
func (vr *valkeyRing) SetWithExpire(ctx context.Context, key, val string, expire time.Duration) []valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.DoMulti(ctx,
		cli.B().Set().Key(key).Value(val).Build(),
		cli.B().Expire().Key(key).Seconds(int64(expire.Seconds())).Build(),
	)
}

func (vr *valkeyRing) ZAdd(ctx context.Context, key, val string, score float64) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Zadd().Key(key).ScoreMember().ScoreMember(score, val).Build())
}

func (vr *valkeyRing) ZCard(ctx context.Context, key string) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Zcard().Key(key).Build())
}

func (vr *valkeyRing) ZRem(ctx context.Context, key string, members ...string) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Zrem().Key(key).Member(members...).Build())
}

func (vr *valkeyRing) ZRemRangeByScore(ctx context.Context, key, min, max string) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Zremrangebyscore().Key(key).Min(min).Max(max).Build())
}

func (vr *valkeyRing) ZRangeByScoreWithScoresFirst(ctx context.Context, key, min, max string, offset, count int64) valkey.ValkeyResult {
	cli := vr.ShardForKey(key)
	return cli.Do(ctx, cli.B().Zrangebyscore().Key(key).Min(min).Max(max).Withscores().Limit(offset, count).Build())
}

func (vr *valkeyRing) RunScript(ctx context.Context, script *valkey.Lua, keys []string, args ...string) valkey.ValkeyResult {

	cli := vr.ShardForKey(strings.Join(keys, ""))
	return script.Exec(ctx, cli, keys, args)
}

// ValkeyRingClient is a wrapper aroung valkey.Client that does access valkey shard by
// computing a ring hash. It logs to the logging.Logger interface,
// that you can pass. It adds metrics and operations are traced with
// opentracing. You can set timeouts and the defaults are set to be ok
// to be in the hot path of low latency production requests.
type ValkeyRingClient struct {
	ring          *valkeyRing
	log           logging.Logger
	metrics       metrics.Metrics
	metricsPrefix string             // TODO(sszuecs): do we need this?
	options       *ValkeyOptions     // TODO(sszuecs): do we need this?
	tracer        opentracing.Tracer // likely we need an OTel.Tracer..
	quit          chan struct{}
	once          sync.Once
	closed        bool
}

func NewValkeyRingClient(opt *ValkeyOptions) (*ValkeyRingClient, error) {
	const backOffTime = 2 * time.Second
	const retryCount = 5

	if opt.Log == nil {
		opt.Log = &logging.DefaultLog{}
	}

	// initially run address updater and pass opt.Addrs on success
	if opt.AddrUpdater != nil {
		address, err := opt.AddrUpdater()
		for range retryCount {
			if err == nil {
				break
			}
			time.Sleep(backOffTime)
			address, err = opt.AddrUpdater()
		}
		if err != nil {
			opt.Log.Errorf("Failed at valkey client startup %v", err)
		} else {
			opt.Addrs = address
		}
	}

	valkeyRingClient, err := newValkeyRing(opt)
	if err != nil {
		return nil, err
	}

	mtr := metrics.Default // TODO(sszuecs): use opt.MetricsPrefix
	quitCH := make(chan struct{})

	vrc := &ValkeyRingClient{
		ring:          valkeyRingClient,
		log:           opt.Log,
		metrics:       mtr,
		metricsPrefix: opt.MetricsPrefix,
		options:       opt,
		tracer:        opt.Tracer,
		quit:          quitCH,
		once:          sync.Once{},
	}

	if opt.AddrUpdater != nil {
		if opt.UpdateInterval == 0 {
			opt.UpdateInterval = DefaultUpdateInterval
		}
		go vrc.startUpdater(context.Background())
	}

	return vrc, nil
}

func (vrc *ValkeyRingClient) Close() error {
	if vrc.closed {
		return nil
	}
	vrc.once.Do(func() {
		vrc.closed = true
		close(vrc.quit)
		vrc.ring.mu.Lock()
		for _, cli := range vrc.ring.clientMap {
			cli.Close()
		}
		vrc.ring.mu.Unlock()
	})
	return nil
}

func (vrc *ValkeyRingClient) startUpdater(ctx context.Context) {
	old := vrc.options.Addrs
	vrc.log.Infof("Start goroutine to update valkey instances every %s", vrc.options.UpdateInterval)
	defer vrc.log.Info("Stopped goroutine to update valkey")

	ticker := time.NewTicker(vrc.options.UpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-vrc.quit:
			return
		case <-ticker.C:
		}

		addrs, err := vrc.options.AddrUpdater()
		if err != nil {
			vrc.log.Errorf("Failed to run valkey updater: %v", err)
			continue
		}
		if len(difference(addrs, old)) != 0 {
			vrc.log.Infof("Valkey updater updating old(%d) != new(%d)", len(old), len(addrs))
			vrc.SetAddrs(ctx, addrs)
			vrc.metrics.UpdateGauge(vrc.metricsPrefix+"shards", float64(vrc.ring.Len()))

			old = addrs
		}
	}
}

func (vrc *ValkeyRingClient) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	return vrc.tracer.StartSpan(operationName, opts...)
}

func (vrc *ValkeyRingClient) SetAddrs(ctx context.Context, addrs []string) {
	if len(addrs) == 0 {
		return
	}

	terminateCli := make(map[string]struct{})
	vrc.ring.mu.RLock()
	for addr := range vrc.ring.clientMap {
		terminateCli[addr] = struct{}{}
	}
	vrc.ring.mu.RUnlock()

	newMap := make(map[string]valkey.Client)
	for _, addr := range addrs {
		vrc.ring.mu.RLock()
		cl, ok := vrc.ring.clientMap[addr]
		vrc.ring.mu.RUnlock()

		if !ok {
			cli, err := createValkeyClient(addr, vrc.options)
			if err != nil {
				vrc.log.Errorf("Failed to create valkey client: %v", err)
				continue
			}
			newMap[addr] = cli
		} else {
			delete(terminateCli, addr)
			newMap[addr] = cl
		}
	}

	vrc.ring.mu.Lock()
	oldCliMap := vrc.ring.clientMap
	vrc.ring.clientMap = newMap
	vrc.ring.mu.Unlock()

	for addr := range terminateCli {
		oldCliMap[addr].Close()
	}
}

func (vrc *ValkeyRingClient) RingAvailable(ctx context.Context) bool {
	return len(vrc.ring.clientMap) > 0 && vrc.PingAll(ctx) == nil
}

func (vrc *ValkeyRingClient) PingAll(ctx context.Context) error {
	res := vrc.ring.PingAll(ctx)
	for _, v := range res {
		if err := v.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (vrc *ValkeyRingClient) Ping(ctx context.Context, shard string) error {
	return vrc.ring.Ping(ctx, shard)
}

func (vrc *ValkeyRingClient) Expire(ctx context.Context, key string, d time.Duration) (bool, error) {
	res := vrc.ring.Expire(ctx, key, d)
	return res.ToBool()
}

func (vrc *ValkeyRingClient) Get(ctx context.Context, key string) (string, error) {
	res := vrc.ring.Get(ctx, key)
	return res.ToString()
}

func (vrc *ValkeyRingClient) Set(ctx context.Context, key, val string) (string, error) {
	res := vrc.ring.Set(ctx, key, val)
	return res.ToString()
}

func (vrc *ValkeyRingClient) SetWithExpire(ctx context.Context, key string, value string, expire time.Duration) error {
	results := vrc.ring.SetWithExpire(ctx, key, value, expire)
	for _, res := range results {
		if err := res.Error(); err != nil {
			return err
		}
	}
	return results[len(results)-1].Error()
}

func (vrc *ValkeyRingClient) ZAdd(ctx context.Context, key, val string, score float64) (int64, error) {
	res := vrc.ring.ZAdd(ctx, key, val, score)
	logrus.Infof("ZADD(%v, %v, %v) res: %+v", key, val, score, res)
	return res.ToInt64()
}

func (vrc *ValkeyRingClient) ZCard(ctx context.Context, key string) (int64, error) {
	res := vrc.ring.ZCard(ctx, key)
	return res.ToInt64()
}

func (vrc *ValkeyRingClient) ZRem(ctx context.Context, key string, members ...string) (int64, error) {
	res := vrc.ring.ZRem(ctx, key, members...)
	return res.ToInt64()
}

func (vrc *ValkeyRingClient) ZRemRangeByScore(ctx context.Context, key, min, max string) (int64, error) {
	res := vrc.ring.ZRemRangeByScore(ctx, key, min, max)
	return res.ToInt64()
}

// TODO(sszuecs): check required return values
func (vrc *ValkeyRingClient) ZRangeByScoreWithScoresFirst(ctx context.Context, key, min, max string, offset, count int64) (any, error) {
	res := vrc.ring.ZRangeByScoreWithScoresFirst(ctx, key, min, max, offset, count)
	ary, err := res.ToArray()
	if err != nil {
		return nil, err
	}
	if len(ary) == 0 {
		return nil, nil
	}
	return ary, nil
}

// TODO(sszuecs): check required return values
func (vrc *ValkeyRingClient) RunScript(ctx context.Context, script *valkey.Lua, keys []string, args ...string) any {
	res := vrc.ring.RunScript(ctx, script, keys, args...)
	// res.AsFloat64() res.AsFloatSlice() res.String() res.ToInt64()
	return res
}

func difference(a, b []string) []string {
	bMap := make(map[string]struct{})
	for _, item := range b {
		bMap[item] = struct{}{}
	}

	var result []string
	for _, item := range a {
		if _, exists := bMap[item]; !exists {
			result = append(result, item)
		}
	}

	return result
}
func intersect(slice1, slice2 []string) []string {
	set := make(map[string]struct{})
	for _, item := range slice1 {
		set[item] = struct{}{}
	}

	var result []string
	for _, item := range slice2 {
		if _, exists := set[item]; exists {
			result = append(result, item)
			delete(set, item)
		}
	}

	return result
}
