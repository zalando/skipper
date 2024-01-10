package routing

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/zalando/skipper/eskip"
)

const defaultLastSeenTimeout = 1 * time.Minute

// Metrics describe the data about endpoint that could be
// used to perform better load balancing, fadeIn, etc.
type Metrics interface {
	DetectedTime() time.Time
	SetDetected(detected time.Time)

	LastSeen() time.Time
	SetLastSeen(lastSeen time.Time)

	InflightRequests() int64
	IncInflightRequest()
	DecInflightRequest()
}

type entry struct {
	detected         atomic.Value // time.Time
	lastSeen         atomic.Value // time.Time
	inflightRequests atomic.Int64
}

var _ Metrics = &entry{}

func (e *entry) DetectedTime() time.Time {
	return e.detected.Load().(time.Time)
}

func (e *entry) LastSeen() time.Time {
	return e.lastSeen.Load().(time.Time)
}

func (e *entry) InflightRequests() int64 {
	return e.inflightRequests.Load()
}

func (e *entry) IncInflightRequest() {
	e.inflightRequests.Add(1)
}

func (e *entry) DecInflightRequest() {
	e.inflightRequests.Add(-1)
}

func (e *entry) SetDetected(detected time.Time) {
	e.detected.Store(detected)
}

func (e *entry) SetLastSeen(ts time.Time) {
	e.lastSeen.Store(ts)
}

func newEntry() *entry {
	result := &entry{}
	result.SetDetected(time.Time{})
	result.SetLastSeen(time.Time{})
	return result
}

type EndpointRegistry struct {
	lastSeenTimeout time.Duration
	now             func() time.Time

	// map[string]*entry
	data sync.Map
}

var _ PostProcessor = &EndpointRegistry{}

type RegistryOptions struct {
	LastSeenTimeout time.Duration
}

func (r *EndpointRegistry) Do(routes []*Route) []*Route {
	now := r.now()

	for _, route := range routes {
		if route.BackendType == eskip.LBBackend {
			for i := range route.LBEndpoints {
				epi := &route.LBEndpoints[i]
				epi.Metrics = r.GetMetrics(epi.Host)
				if epi.Metrics.DetectedTime().IsZero() {
					epi.Metrics.SetDetected(now)
				}

				epi.Metrics.SetLastSeen(now)
			}
		} else {
			entry := r.GetMetrics(route.Host)
			if entry.DetectedTime().IsZero() {
				entry.SetDetected(now)
			}
			entry.SetLastSeen(now)
		}
	}

	removeOlder := now.Add(-r.lastSeenTimeout)
	r.data.Range(func(key, value any) bool {
		e := value.(*entry)
		if e.LastSeen().Before(removeOlder) {
			r.data.Delete(key)
		}

		return true
	})

	return routes
}

func NewEndpointRegistry(o RegistryOptions) *EndpointRegistry {
	if o.LastSeenTimeout == 0 {
		o.LastSeenTimeout = defaultLastSeenTimeout
	}

	return &EndpointRegistry{
		data:            sync.Map{},
		lastSeenTimeout: o.LastSeenTimeout,
		now:             time.Now,
	}
}

func (r *EndpointRegistry) GetMetrics(key string) Metrics {
	e, _ := r.data.LoadOrStore(key, newEntry())
	return e.(*entry)
}
