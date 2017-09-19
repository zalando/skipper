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

// Registry objects hold the active ratelimitters, ensure synchronized
// access to them, apply default settings and recycle the idle
// ratelimitters.
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
	if s.Type == DisableRatelimit {
		return nil
	}

	return r.get(s)
}

// Check returns false in case of request is ratelimitted and false
// otherwise. It returns the used Settings as 2nd argument.
func (r *Registry) Check(req *http.Request) (bool, Settings) {
	if r == nil {
		return true, Settings{}
	}

	ip := net.RemoteHost(req)

	if !r.Get(r.global).Allow(ip.String()) {
		return false, r.global
	}
	return true, Settings{}

}
