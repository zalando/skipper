package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
)

const (
	DefaultMaxhits       = 20
	DefaultTimeWindow    = 1 * time.Second
	DefaultCleanInterval = 60 * time.Second
)

// Registry objects hold the active ratelimiters, ensure synchronized
// access to them, apply default settings and recycle the idle
// ratelimiters.
type Registry struct {
	sync.Mutex
	once      sync.Once
	global    Settings
	lookup    map[Settings]*Ratelimit
	swarm     Swarmer
	redisRing *net.RedisRingClient
}

// NewRegistry initializes a registry with the provided default settings.
func NewRegistry(settings ...Settings) *Registry {
	return NewSwarmRegistry(nil, nil, settings...)
}

// NewSwarmRegistry initializes a registry with an optional swarm and
// the provided default settings. If swarm is nil, clusterRatelimits
// will be replaced by voidRatelimit, which is a noop limiter implementation.
func NewSwarmRegistry(swarm Swarmer, ro *net.RedisOptions, settings ...Settings) *Registry {
	defaults := Settings{
		Type:          DisableRatelimit,
		MaxHits:       DefaultMaxhits,
		TimeWindow:    DefaultTimeWindow,
		CleanInterval: DefaultCleanInterval,
	}

	if ro != nil && ro.MetricsPrefix == "" {
		ro.MetricsPrefix = redisMetricsPrefix
	}

	r := &Registry{
		once:      sync.Once{},
		global:    defaults,
		lookup:    make(map[Settings]*Ratelimit),
		swarm:     swarm,
		redisRing: net.NewRedisRingClient(ro),
	}
	if ro != nil {
		r.redisRing.StartMetricsCollection()
	}

	if len(settings) > 0 {
		r.global = settings[0]
	}

	return r
}

// Close teardown Registry and dependent resources
func (r *Registry) Close() {
	r.once.Do(func() {
		r.redisRing.Close()
		for _, rl := range r.lookup {
			rl.Close()
		}
	})
}

func (r *Registry) get(s Settings) *Ratelimit {
	r.Lock()
	defer r.Unlock()

	rl, ok := r.lookup[s]
	if !ok {
		rl = newRatelimit(s, r.swarm, r.redisRing)
		r.lookup[s] = rl
	}

	return rl
}

// Get returns a Ratelimit instance for provided Settings
func (r *Registry) Get(s Settings) *Ratelimit {
	if s.Type == DisableRatelimit || s.Type == NoRatelimit {
		return nil
	}
	return r.get(s)
}

// Check returns Settings used and the retry-after duration in case of
// request is ratelimitted. Otherwise, return the Settings and 0. It is
// only used in the global ratelimit facility.
func (r *Registry) Check(req *http.Request) (Settings, int) {
	if r == nil {
		return Settings{}, 0
	}

	s := r.global

	rlimit := r.Get(s)

	switch s.Type {
	case ClusterServiceRatelimit:
		fallthrough
	case ServiceRatelimit:
		if rlimit.Allow(context.Background(), "") {
			return s, 0
		}
		return s, rlimit.RetryAfter("")

	case LocalRatelimit:
		log.Warning("LocalRatelimit is deprecated, please use ClientRatelimit instead")
		fallthrough
	case ClusterClientRatelimit:
		fallthrough
	case ClientRatelimit:
		ip := net.RemoteHost(req)
		if !rlimit.Allow(context.Background(), ip.String()) {
			return s, rlimit.RetryAfter(ip.String())
		}
	}

	return Settings{}, 0
}
