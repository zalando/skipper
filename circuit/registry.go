package circuit

import "time"

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
	sync         chan *Registry
}

func NewRegistry(o Options) *Registry {
	hs := make(map[string]BreakerSettings)
	for _, s := range o.HostSettings {
		hs[s.Host] = applySettings(s, o.Defaults)
	}

	if o.IdleTTL <= 0 {
		o.IdleTTL = time.Hour
	}

	r := &Registry{
		defaults:     o.Defaults,
		hostSettings: hs,
		idleTTL:      o.IdleTTL,
		lookup:       make(map[BreakerSettings]*Breaker),
		access:       &list{},
		sync:         make(chan *Registry, 1),
	}

	r.sync <- r
	return r
}

func (r *Registry) synced(f func()) {
	r = <-r.sync
	f()
	r.sync <- r
}

func (r *Registry) applySettings(s BreakerSettings) BreakerSettings {
	config, ok := r.hostSettings[s.Host]
	if !ok {
		config = r.defaults
	}

	return applySettings(s, config)
}

func (r *Registry) dropLookup(b *Breaker) {
	for b != nil {
		delete(r.lookup, b.settings)
		b = b.next
	}
}

func (r *Registry) Get(s BreakerSettings) *Breaker {
	// we check for host, because we don't want to use shared global breakers
	if s.Disabled || s.Host == "" {
		return nil
	}

	// apply host specific and global defaults when not set in the request
	s = r.applySettings(s)
	if s.Type == BreakerNone {
		return nil
	}

	var b *Breaker
	r.synced(func() {
		now := time.Now()

		var ok bool
		b, ok = r.lookup[s]
		if !ok {
			// if the breaker doesn't exist with the requested settings,
			// check if there is any to evict, evict if yet, and create
			// a new one

			drop, _ := r.access.dropHeadIf(func(b *Breaker) bool {
				return now.Sub(b.ts) > r.idleTTL
			})

			r.dropLookup(drop)
			b = newBreaker(s)
			r.lookup[s] = b
		}

		// append/move the breaker to the last position of the access history
		b.ts = now
		r.access.appendLast(b)
	})

	return b
}
