package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/swarm"
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
	defaults     Settings
	global       Settings
	lookup       map[Settings]*Ratelimit
	swarm        Swarmer
	swimOptions  *swarm.Options
	redisOptions *RedisOptions
}

// NewRegistry initializes a registry with the provided default settings.
func NewRegistry(settings ...Settings) *Registry {
	return NewSwarmRegistry(nil, nil, nil, settings...)
}

// NewSwarmRegistry initializes a registry with an optional swarm and
// the provided default settings. If swarm is nil, clusterRatelimits
// will be replaced by voidRatelimit, which is a noop limiter implementation.
func NewSwarmRegistry(swarm Swarmer, swimOptions *swarm.Options, redisOptions *RedisOptions, settings ...Settings) *Registry {
	defaults := Settings{
		Type:          DisableRatelimit,
		MaxHits:       DefaultMaxhits,
		TimeWindow:    DefaultTimeWindow,
		CleanInterval: DefaultCleanInterval,
	}

	r := &Registry{
		defaults:     defaults,
		global:       defaults,
		lookup:       make(map[Settings]*Ratelimit),
		swarm:        swarm,
		swimOptions:  swimOptions,
		redisOptions: redisOptions,
	}

	if len(settings) > 0 {
		r.global = settings[0]
	}

	return r
}

func (r *Registry) get(s Settings) *Ratelimit {
	r.Lock()
	defer r.Unlock()

	rl, ok := r.lookup[s]
	if !ok {
		rl = newRatelimit(s, r.swarm, r.swimOptions, r.redisOptions)
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

// Check returns Settings used and the retry-after duration in case of request is
// ratelimitted. Otherwise return the Settings and 0.
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
		if rlimit.Allow("") {
			return s, 0
		}
		return s, rlimit.RetryAfter("")

	case ClusterClientRatelimit:
		fallthrough
	case LocalRatelimit: // TODO(sszuecs): name should be dropped if we do a breaking change
		fallthrough
	case ClientRatelimit:
		ip := net.RemoteHost(req)
		if !rlimit.Allow(ip.String()) {
			return s, rlimit.RetryAfter(ip.String())
		}
	}

	return Settings{}, 0
}
