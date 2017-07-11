package circuit

import (
	"sync"
	"time"
)

const DefaultIdleTTL = time.Hour

// Registry objects hold the active circuit breakers, ensure synchronized access to them, apply default settings
// and recycle the idle breakers.
type Registry struct {
	defaults     BreakerSettings
	hostSettings map[string]BreakerSettings
	lookup       map[BreakerSettings]*Breaker
	mx           *sync.Mutex
}

// NewRegistry initializes a registry with the provided default settings. Settings with an empty Host field are
// considered as defaults. Settings with the same Host field are merged together.
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
	for h, b := range r.lookup {
		if b.idle(now) {
			delete(r.lookup, h)
		}
	}
}

func (r *Registry) get(s BreakerSettings) *Breaker {
	r.mx.Lock()
	defer r.mx.Unlock()

	now := time.Now()

	b, ok := r.lookup[s]
	if !ok || b.idle(now) {
		// check if there is any other to evict, evict if yes
		r.dropIdle(now)

		// create a new one
		b = newBreaker(s)
		r.lookup[s] = b
	}

	// set the access timestamp
	b.ts = now

	return b
}

// Get returns a circuit breaker for the provided settings. The BreakerSettings object is used here as a key,
// but typically it is enough to just set its Host field:
//
// 	r.Get(BreakerSettings{Host: backendHost})
//
// The key will be filled up with the defaults and the matching circuit breaker will be returned if it exists,
// or a new one will be created if not.
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
