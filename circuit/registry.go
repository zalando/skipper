package circuit

import (
	"sync"
	"time"
)

const RouteSettingsKey = "#circuitbreakersettings"

type Options struct {
	Defaults     BreakerSettings
	HostSettings []BreakerSettings
	IdleTTL      time.Duration
}

type Registry struct {
	defaults     BreakerSettings
	hostSettings map[string]BreakerSettings
	idleTTL      time.Duration
	lookup       map[BreakerSettings]*Breaker
	access       *list
	mx           *sync.Mutex
}

func NewRegistry(o Options) *Registry {
	hs := make(map[string]BreakerSettings)
	for _, s := range o.HostSettings {
		hs[s.Host] = s.mergeSettings(o.Defaults)
	}

	if o.IdleTTL <= 0 {
		o.IdleTTL = time.Hour
	}

	return &Registry{
		defaults:     o.Defaults,
		hostSettings: hs,
		idleTTL:      o.IdleTTL,
		lookup:       make(map[BreakerSettings]*Breaker),
		access:       &list{},
		mx:           &sync.Mutex{},
	}
}

func (r *Registry) mergeDefaults(s BreakerSettings) BreakerSettings {
	config, ok := r.hostSettings[s.Host]
	if !ok {
		config = r.defaults
	}

	return s.mergeSettings(config)
}

func (r *Registry) dropIdle(now time.Time) {
	drop, _ := r.access.dropHeadIf(func(b *Breaker) bool {
		return now.Sub(b.ts) > r.idleTTL
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
	if !ok {
		// check if there is any to evict, evict if yes
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
	if s.Disabled || s.Host == "" {
		return nil
	}

	s = r.mergeDefaults(s)
	if s.Type == BreakerNone {
		return nil
	}

	return r.get(s)
}
