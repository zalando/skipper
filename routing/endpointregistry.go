package routing

import (
	"sync"
	"time"

	"github.com/zalando/skipper/eskip"
)

const lastSeenTimeout = 1 * time.Minute

// Metrics describe the data about endpoint that could be
// used to perform better load balancing, fadeIn, etc.
type Metrics interface {
	DetectedTime() time.Time
	InflightRequests() int64
}

type entry struct {
	detected         time.Time
	inflightRequests int64
}

var _ Metrics = &entry{}

func (e *entry) DetectedTime() time.Time {
	return e.detected
}

func (e *entry) InflightRequests() int64 {
	return e.inflightRequests
}

type EndpointRegistry struct {
	lastSeen map[string]time.Time

	mu sync.Mutex

	data map[string]*entry
}

var _ PostProcessor = &EndpointRegistry{}

type RegistryOptions struct {
}

func (r *EndpointRegistry) Do(routes []*Route) []*Route {
	now := time.Now()

	for _, route := range routes {
		if route.BackendType == eskip.LBBackend {
			for _, epi := range route.LBEndpoints {
				metrics := r.GetMetrics(epi.Host)
				if metrics.DetectedTime().IsZero() {
					r.SetDetectedTime(epi.Host, now)
				}

				r.lastSeen[epi.Host] = now
			}
		}
	}

	for host, ts := range r.lastSeen {
		if ts.Add(lastSeenTimeout).Before(now) {
			delete(r.lastSeen, host)
			r.mu.Lock()
			delete(r.data, host)
			r.mu.Unlock()
		}
	}

	return routes
}

func NewEndpointRegistry(o RegistryOptions) *EndpointRegistry {
	return &EndpointRegistry{
		data:     map[string]*entry{},
		lastSeen: map[string]time.Time{},
	}
}

func (r *EndpointRegistry) GetMetrics(key string) Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	copy := &entry{}
	*copy = *e
	return copy
}

func (r *EndpointRegistry) SetDetectedTime(key string, detected time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.detected = detected
}

func (r *EndpointRegistry) IncInflightRequest(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.inflightRequests++
}

func (r *EndpointRegistry) DecInflightRequest(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.inflightRequests--
}

// getOrInitEntryLocked returns pointer to endpoint registry entry
// which contains the information about endpoint representing the
// following key. r.mu must be held while calling this function and
// using of the entry returned. In general, key represents the "host:port"
// string
func (r *EndpointRegistry) getOrInitEntryLocked(key string) *entry {
	e, ok := r.data[key]
	if !ok {
		e = &entry{}
		r.data[key] = e
	}
	return e
}
