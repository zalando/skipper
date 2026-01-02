package net

import (
	"context"
	"sync"
	"time"

	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"

	"github.com/opentracing/opentracing-go"
	"github.com/valkey-io/valkey-go"
)

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

		MaxFlushDelay: 20 * time.Microsecond, // reduce CPU load without much impace, ref: https://github.com/redis/rueidis/issues/156

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
	clientMap map[string]valkey.Client // TODO(sszuecs): is this the right data structure?
}

func (vr *valkeyRing) ShardForKey(key string) valkey.Client {
	// TODO(sszuecs): use xxhash.Sum64String to map key to client
	return nil
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
	valkeyRingClient := &valkeyRing{
		clientMap: make(map[string]valkey.Client),
	}

	for _, ep := range opt.Addrs {
		cl, err := createValkeyClient(ep, opt)
		if err != nil {
			return nil, err
		}
		valkeyRingClient.clientMap[ep] = cl
	}

	mtr := metrics.Default // TODO(sszuecs): use opt.MetricsPrefix
	quitCH := make(chan struct{})

	return &ValkeyRingClient{
		ring:          valkeyRingClient,
		log:           opt.Log,
		metrics:       mtr,
		metricsPrefix: opt.MetricsPrefix,
		options:       opt,
		tracer:        opt.Tracer,
		quit:          quitCH,
		once:          sync.Once{},
	}, nil
}

func (vrc *ValkeyRingClient) Close() error {
	vrc.once.Do(func() {
		vrc.closed = true
		close(vrc.quit)
		for _, cli := range vrc.ring.clientMap {
			cli.Close()
		}
	})
	return nil
}

func (vrc *ValkeyRingClient) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	return vrc.tracer.StartSpan(operationName, opts...)
}

func (vrc *ValkeyRingClient) SetAddrs(ctx context.Context, addrs []string) {
	if len(addrs) == 0 {
		return
	}

	terminateCli := make(map[string]struct{})
	for addr := range vrc.ring.clientMap {
		terminateCli[addr] = struct{}{}
	}

	newMap := make(map[string]valkey.Client)
	for _, addr := range addrs {
		if cl, ok := vrc.ring.clientMap[addr]; !ok {
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

	oldCliMap := vrc.ring.clientMap
	vrc.ring.clientMap = newMap

	for addr := range terminateCli {
		oldCliMap[addr].Close()
	}
}

func (vrc *ValkeyRingClient) Get(ctx context.Context, key string) (string, error) {
	res := vrc.ring.Get(ctx, key)
	return res.Val(), res.Err()
}

func (vrc *ValkeyRingClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) (string, error) {
	res := vrc.ring.Set(ctx, key, value, expiration)
	return res.Result()
}
