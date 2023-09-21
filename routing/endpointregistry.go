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
	FadeInDuration(routeId string) time.Duration
}

type entry struct {
	detected         time.Time
	inflightRequests int64
	fadeIn           map[string]fadeInData
}

type fadeInData struct {
	fadeInDuration time.Duration
	fadeInExponent float64
}

var _ Metrics = &entry{}

func (e *entry) DetectedTime() time.Time {
	return e.detected
}

func (e *entry) InflightRequests() int64 {
	return e.inflightRequests
}

func (e *entry) FadeInDuration(routeId string) time.Duration {
	d, ok := e.fadeIn[routeId]
	if !ok {
		return 0
	}

	return d.fadeInDuration
}

type EndpointRegistry struct {
	lastSeen map[string]time.Time
	now      func() time.Time

	mu sync.Mutex

	data map[string]*entry
}

var _ PostProcessor = &EndpointRegistry{}

type RegistryOptions struct {
}

func (r *EndpointRegistry) Do(routes []*Route) []*Route {
	now := r.now()

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
			r.mu.Lock()
			if r.data[host].inflightRequests == 0 {
				delete(r.lastSeen, host)
				delete(r.data, host)
			}
			r.mu.Unlock()
		}
	}

	return routes
}

func NewEndpointRegistry(o RegistryOptions) *EndpointRegistry {
	return &EndpointRegistry{
		data:     map[string]*entry{},
		lastSeen: map[string]time.Time{},
		now:      time.Now,
	}
}

func (r *EndpointRegistry) GetMetrics(key string) Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	copy := &entry{}
	*copy = *e

	copy.fadeIn = make(map[string]fadeInData)
	for k, v := range e.fadeIn {
		copy.fadeIn[k] = v
	}
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

func (r *EndpointRegistry) SetFadeIn(key string, routeId string, duration time.Duration, exponent float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.fadeIn[routeId] = fadeInData{fadeInDuration: duration, fadeInExponent: exponent}
}

// getOrInitEntryLocked returns pointer to endpoint registry entry
// which contains the information about endpoint representing the
// following key. r.mu must be held while calling this function and
// using of the entry returned. In general, key represents the "host:port"
// string
func (r *EndpointRegistry) getOrInitEntryLocked(key string) *entry {
	e, ok := r.data[key]
	if !ok {
		e = &entry{fadeIn: map[string]fadeInData{}}
		r.data[key] = e
	}
	return e
}
