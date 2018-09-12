package ratelimit

import (
	"net/http"
	"sync"
	"time"

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
	defaults Settings
	global   Settings
	//routeSettings map[string]Settings
	lookup map[Settings]*Ratelimit
}

// NewRegistry initializes a registry with the provided default settings.
func NewRegistry(settings ...Settings) *Registry {
	defaults := Settings{
		Type:          DisableRatelimit,
		MaxHits:       DefaultMaxhits,
		TimeWindow:    DefaultTimeWindow,
		CleanInterval: DefaultCleanInterval,
	}

	r := &Registry{
		defaults: defaults,
		global:   defaults,
		lookup:   make(map[Settings]*Ratelimit),
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
		rl = newRatelimit(s)
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

	registry := r.Get(s)
	switch s.Type {
	case ServiceRatelimit:
		if registry.Allow("") {
			return s, 0
		}
		return s, registry.RetryAfter("")

	case LocalRatelimit:
		ip := net.RemoteHost(req)
		if !registry.Allow(ip.String()) {
			return s, registry.RetryAfter(ip.String())
		}
	}

	return Settings{}, 0
}
