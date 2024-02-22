package routing

import (
	"math"
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

	TotalRequests() int64
	IncTotalRequests()
	ResetTotalRequests()
	FailedRequests() int64
	IncFailedRequests()
	ResetFailedRequests()

	FailProbability() float64
	SetFailProbability(float64)
}

type entry struct {
	detected         atomic.Value // time.Time
	lastSeen         atomic.Value // time.Time
	inflightRequests atomic.Int64
	totalRequests    atomic.Int64
	failedRequests   atomic.Int64
	failProbability  atomic.Uint64 // float64, see https://github.com/golang/go/issues/39356
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

func (e *entry) TotalRequests() int64 {
	return e.totalRequests.Load()
}

func (e *entry) IncTotalRequests() {
	e.totalRequests.Add(1)
}

func (e *entry) ResetTotalRequests() {
	e.totalRequests.Store(0)
}

func (e *entry) FailedRequests() int64 {
	return e.failedRequests.Load()
}

func (e *entry) IncFailedRequests() {
	e.failedRequests.Add(1)
}

func (e *entry) ResetFailedRequests() {
	e.totalRequests.Store(0)
}

func (e *entry) FailProbability() float64 {
	v := e.failProbability.Load()
	return math.Float64frombits(v)
}

func (e *entry) SetFailProbability(p float64) {
	e.failProbability.Store(math.Float64bits(p))
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
	data            sync.Map // map[string]*entry
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
		} else if route.BackendType == eskip.NetworkBackend {
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

func (r *EndpointRegistry) GetMetrics(hostPort string) Metrics {
	// https://github.com/golang/go/issues/44159#issuecomment-780774977
	e, ok := r.data.Load(hostPort)
	if !ok {
		e, _ = r.data.LoadOrStore(hostPort, newEntry())
	}
	return e.(*entry)
}

func (r *EndpointRegistry) Visit(f func(Metrics)) {
	r.data.Range(func(_, v any) bool {
		f(v.(Metrics))
		return true
	})
}

func (r *EndpointRegistry) allMetrics() map[string]Metrics {
	result := make(map[string]Metrics)
	r.data.Range(func(k, v any) bool {
		result[k.(string)] = v.(Metrics)
		return true
	})
	return result
}
