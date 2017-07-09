package circuit

import (
	"sync"
	"time"
)

const (
	DefaultIdleTTL   = time.Hour
	RouteSettingsKey = "#circuitbreakersettings"
)

type Options struct {
	Defaults     BreakerSettings
	HostSettings []BreakerSettings
}

type Registry struct {
	defaults     BreakerSettings
	hostSettings map[string]BreakerSettings
	lookup       map[BreakerSettings]*Breaker
	access       *list
	mx           *sync.Mutex
}

func NewRegistry(o Options) *Registry {
	if o.Defaults.IdleTTL <= 0 {
		o.Defaults.IdleTTL = DefaultIdleTTL
	}

	hs := make(map[string]BreakerSettings)
	for _, s := range o.HostSettings {
		hs[s.Host] = s.mergeSettings(o.Defaults)
	}

	return &Registry{
		defaults:     o.Defaults,
		hostSettings: hs,
		lookup:       make(map[BreakerSettings]*Breaker),
		access:       &list{},
		mx:           &sync.Mutex{},
	}
}

func (r *Registry) mergeDefaults(s BreakerSettings) BreakerSettings {
	defaults, ok := r.hostSettings[s.Host]
	if !ok {
		defaults = r.defaults
	}

	return s.mergeSettings(defaults)
}

func (r *Registry) dropIdle(now time.Time) {
	drop, _ := r.access.dropHeadIf(func(b *Breaker) bool {
		return b.idle(now)
	})

	for drop != nil {
		delete(r.lookup, drop.settings)
		drop = drop.next
	}
}

func (r *Registry) get(s BreakerSettings) *Breaker {
	r.mx.Lock()
	defer r.mx.Unlock()

	now := time.Now()

	b, ok := r.lookup[s]
	if !ok || b.idle(now) {
		// if doesn't exist or idle, cleanup and create a new one
		r.access.remove(b, b)

		// check if there is any other to evict, evict if yes
		r.dropIdle(now)

		// create a new one
		b = newBreaker(s)
		r.lookup[s] = b
	}

	// append/move the breaker to the last position of the access history
	b.ts = now
	r.access.appendLast(b)

	return b
}

func (r *Registry) Get(s BreakerSettings) *Breaker {
	// we check for host, because we don't want to use shared global breakers
	if s.Type == BreakerDisabled || s.Host == "" {
		return nil
	}

	s = r.mergeDefaults(s)
	if s.Type == BreakerNone {
		return nil
	}

	return r.get(s)
}
