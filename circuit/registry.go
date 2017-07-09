package circuit

import (
	"sync"
	"time"
)

// DefaultIdleTTL is used to recycle those unused circuit breakers that don't have this value configured and it
// is not set globally.
const DefaultIdleTTL = time.Hour

// Registry objects hold the active circuit breakers, ensure synchronized access to them, apply default settings
// and recycle the idle ones.
type Registry struct {
	defaults     BreakerSettings
	hostSettings map[string]BreakerSettings
	lookup       map[BreakerSettings]*Breaker
	access       *list
	mx           *sync.Mutex
}

// NewRegistry initializes a registry with the provided default settings. Settings with the same host value are
// merged together, and settings with an empty host field are merged into each.
func NewRegistry(settings ...BreakerSettings) *Registry {
	var (
		defaults     BreakerSettings
		hostSettings []BreakerSettings
	)

	for _, s := range settings {
		if s.Host == "" {
			defaults = defaults.mergeSettings(s)
			continue
		}

		hostSettings = append(hostSettings, s)
	}

	if defaults.IdleTTL <= 0 {
		defaults.IdleTTL = DefaultIdleTTL
	}

	hs := make(map[string]BreakerSettings)
	for _, s := range hostSettings {
		if sh, ok := hs[s.Host]; ok {
			hs[s.Host] = s.mergeSettings(sh)
		} else {
			hs[s.Host] = s.mergeSettings(defaults)
		}
	}

	return &Registry{
		defaults:     defaults,
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

// Get returns a circuit breaker for the provided settings. The BreakerSettings object is used here as a key,
// but typically it is enough to just set its Host field:
//
// 	r.Get(BreakerSettings{Host: backendHost})
//
// The key will be filled up with the defaults and the matching circuit breaker will be returned if it exists or
// a new one will be created.
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
