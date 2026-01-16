package net

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/opentracing/opentracing-go"
	"github.com/valkey-io/valkey-go"

	"github.com/valkey-io/valkey-go/valkeyhook"
	"github.com/valkey-io/valkey-go/valkeyotel"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
)

const ringSize = 10000

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

	// Hook see https://pkg.go.dev/github.com/valkey-io/valkey-go/valkeyhook
	Hook valkeyhook.Hook

	// EnableOTel enables OpenTelemetry adapter, see https://pkg.go.dev/github.com/valkey-io/valkey-go/valkeyotel
	EnableOTel bool
	// OTelOptions
	OTelOptions []valkeyotel.Option
	// Metrics collector
	Metrics metrics.Metrics
	// MetricsPrefix is the prefix for valkey ring client metrics,
	// defaults to "swarm.valkey." if not set
	MetricsPrefix string
	// Tracer provides OpenTracing for Valkey queries.
	Tracer opentracing.Tracer
	// Log is the logger that is used
	Log logging.Logger
}

func createValkeyClient(addr string, opt *ValkeyOptions) (valkey.Client, error) {
	clientOptions := valkey.ClientOption{
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

	}
	var (
		cli valkey.Client
		err error
	)

	if opt.EnableOTel {
		valkeyotel.NewClient(clientOptions, opt.OTelOptions...)
	} else {
		cli, err = valkey.NewClient(clientOptions)
	}

	if opt.Hook != nil {
		cli = valkeyhook.WithHook(cli, opt.Hook)
	}
	return cli, err
}

type valkeyRing struct {
	opt *ValkeyOptions

	// maps int to client for sharding, trades memory for concurrent access
	// most operations only have to use this lock-free structure
	shards       [ringSize]valkey.Client
	activeShards int

	// clientMap is used for Ping operations and to simplify update shards
	mu        sync.Mutex
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
		ring.clientMap[ep] = cl
	}

	if len(opt.Addrs) == 0 {
		return ring, nil
	}

	ring.updateShards(opt.Addrs)

	return ring, nil
}

// updateShards needs to be called with holding lock vr.mu
func (vr *valkeyRing) updateShards(addr []string) {
	if len(addr) == 0 {
		return
	}
	cur := -1
	shardSize := computeShardSize(len(addr))
	clients := make([]valkey.Client, 0, len(addr))

	for _, cl := range vr.clientMap {
		clients = append(clients, cl)
	}

	for i := range ringSize {
		if i%shardSize == 0 {
			cur++
		}
		vr.shards[i] = clients[cur]
	}
	vr.activeShards = cur + 1
}

func (vr *valkeyRing) Len() int {
	return vr.activeShards
}

func (vr *valkeyRing) SetAddr(addr []string) error {
	if len(addr) == 0 {
		return nil
	}

	vr.mu.Lock()
	defer vr.mu.Unlock()
	current := make([]string, 0, len(vr.clientMap))

	for k := range vr.clientMap {
		current = append(current, k)
	}

	// set operations
	intersection := intersect(addr, current)
	if len(addr) == len(intersection) && len(current) == len(intersection) {
		return nil
	}
	newAddr := difference(addr, intersection)
	oldAddr := difference(current, intersection)

	// create new clients
	for _, addr := range newAddr {
		cli, err := createValkeyClient(addr, vr.opt)
		if err != nil {
			return fmt.Errorf("failed to create valkey client on SetAddr: %w", err)
		}
		vr.clientMap[addr] = cli
	}

	// close old clients and update current shards
	for _, addr := range oldAddr {
		cli, ok := vr.clientMap[addr]
		delete(vr.clientMap, addr)
		if ok {
			cli.Close()
		}
	}
	// we need to update current shards on delete with the same lock
	curAddr := make([]string, 0, len(vr.clientMap))
	for k := range vr.clientMap {
		curAddr = append(curAddr, k)
	}
	vr.updateShards(curAddr)

	return nil
}

// shardForKey does the lookup for valkey most operations to find the valkey ring shard
func (vr *valkeyRing) shardForKey(key string) valkey.Client {
	return vr.shards[xxhash.Sum64String(key)%ringSize]
}

// PingAll pings all known shards
func (vr *valkeyRing) PingAll(ctx context.Context) map[string]valkey.ValkeyResult {
	res := make(map[string]valkey.ValkeyResult)
	vr.mu.Lock()
	for k, shard := range vr.clientMap {
		res[k] = shard.Do(ctx, shard.B().Ping().Build())
	}
	vr.mu.Unlock()
	return res
}

// Ping pings given shard by address:port
func (vr *valkeyRing) Ping(ctx context.Context, s string) error {
	vr.mu.Lock()
	shard, ok := vr.clientMap[s]
	vr.mu.Unlock()
	if !ok {
		return fmt.Errorf("failed to find client for shard: %q", s)
	}
	res := shard.Do(ctx, shard.B().Ping().Build())
	return res.Error()
}

func (vr *valkeyRing) Expire(ctx context.Context, key string, expire time.Duration) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Expire().Key(key).Seconds(int64(expire.Seconds())).Build())
}

func (vr *valkeyRing) Get(ctx context.Context, key string) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Get().Key(key).Build())
}

func (vr *valkeyRing) Set(ctx context.Context, key, val string) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Set().Key(key).Value(val).Build())
}
func (vr *valkeyRing) SetWithExpire(ctx context.Context, key, val string, expire time.Duration) []valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.DoMulti(ctx,
		shard.B().Set().Key(key).Value(val).Build(),
		shard.B().Expire().Key(key).Seconds(int64(expire.Seconds())).Build(),
	)
}

func (vr *valkeyRing) ZAdd(ctx context.Context, key, val string, score float64) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Zadd().Key(key).ScoreMember().ScoreMember(score, val).Build())
}

func (vr *valkeyRing) ZCard(ctx context.Context, key string) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Zcard().Key(key).Build())
}

func (vr *valkeyRing) ZRem(ctx context.Context, key string, members ...string) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Zrem().Key(key).Member(members...).Build())
}

func (vr *valkeyRing) ZRemRangeByScore(ctx context.Context, key, min, max string) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Zremrangebyscore().Key(key).Min(min).Max(max).Build())
}

func (vr *valkeyRing) ZRangeByScoreWithScoresFirst(ctx context.Context, key, min, max string, offset, count int64) valkey.ValkeyResult {
	shard := vr.shardForKey(key)
	return shard.Do(ctx, shard.B().Zrangebyscore().Key(key).Min(min).Max(max).Withscores().Limit(offset, count).Build())
}

func (vr *valkeyRing) RunScript(ctx context.Context, script *valkey.Lua, keys []string, args ...string) valkey.ValkeyResult {
	shard := vr.shardForKey(strings.Join(keys, ""))
	return script.Exec(ctx, shard, keys, args)
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
	metricsPrefix string
	options       *ValkeyOptions
	tracer        opentracing.Tracer // likely we need an OTel.Tracer..
	quit          chan struct{}
	once          sync.Once
	closed        bool
}

func NewValkeyRingClient(opt *ValkeyOptions) (*ValkeyRingClient, error) {
	const backOffTime = 2 * time.Second
	const retryCount = 5

	// defaults
	if opt.Tracer == nil {
		opt.Tracer = &opentracing.NoopTracer{}
	}
	if opt.Log == nil {
		opt.Log = &logging.DefaultLog{}
	}
	if opt.Metrics == nil {
		opt.Metrics = metrics.Default
	}

	// initially run address updater and pass opt.Addrs on success
	if opt.AddrUpdater != nil {
		if opt.UpdateInterval == 0 {
			opt.UpdateInterval = DefaultUpdateInterval
		}

		address, err := opt.AddrUpdater()
		for range retryCount {
			if err == nil {
				break
			}
			time.Sleep(backOffTime)
			address, err = opt.AddrUpdater()
		}
		if err != nil {
			opt.Log.Errorf("Failed start valkey client: %v", err)
		} else {
			opt.Addrs = address
		}
	}

	valkeyRingClient, err := newValkeyRing(opt)
	if err != nil {
		return nil, err
	}

	quitCH := make(chan struct{})

	vrc := &ValkeyRingClient{
		ring:          valkeyRingClient,
		log:           opt.Log,
		metrics:       opt.Metrics,
		metricsPrefix: opt.MetricsPrefix,
		options:       opt,
		tracer:        opt.Tracer,
		quit:          quitCH,
		once:          sync.Once{},
	}

	if opt.AddrUpdater != nil {
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
	vrc.ring.SetAddr(addrs)
}

func (vrc *ValkeyRingClient) RingAvailable(ctx context.Context) bool {
	return vrc.ring.activeShards > 0 && vrc.PingAll(ctx) == nil
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

func (vrc *ValkeyRingClient) Expire(ctx context.Context, key string, d time.Duration) (int64, error) {
	res := vrc.ring.Expire(ctx, key, d)
	return res.ToInt64()
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

// ZRangeByScoreWithScoresFirst returns the first value as string, count should be set to 1
func (vrc *ValkeyRingClient) ZRangeByScoreWithScoresFirst(ctx context.Context, key, min, max string, offset, count int64) (string, error) {
	res := vrc.ring.ZRangeByScoreWithScoresFirst(ctx, key, min, max, offset, count)
	a, err := res.ToArray()
	if err != nil {
		return "", err
	}
	if len(a) == 0 {
		return "", nil
	}
	msg, err := a[0].ToArray()
	if err != nil {
		return "", err
	}
	if len(msg) == 0 {
		return "", nil
	}
	return msg[0].ToString()
}

func (vrc *ValkeyRingClient) RunScript(ctx context.Context, script *valkey.Lua, keys []string, args ...string) (valkey.ValkeyMessage, error) {
	res := vrc.ring.RunScript(ctx, script, keys, args...)
	return res.ToMessage()
}

func NewScript(src string) *valkey.Lua {
	return valkey.NewLuaScript(src)
}

func computeShardSize(i int) int {
	if i == 0 {
		return ringSize
	}
	return int(math.Ceil(float64(ringSize) / float64(i)))
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
