package hedge

import (
	"reflect"
	"sync"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

const (
	eligibleRequestsPerRetry = 20
	headerLatencySampleSize  = 1028
	headerLatencySampleAlpha = 0.015
	headerLatencyWarmup      = 100
	headerLatencyPercentile  = 0.95
	idleStateResetDuration   = time.Minute
)

var timeNow = time.Now

type (
	adaptiveState struct {
		mu               sync.Mutex
		eligibleRequests uint8
		retryAvailable   bool
		retryReserved    bool
		headerLatencies  metrics.Sample
	}

	filter struct {
		state *routeState
	}

	requestState struct {
		mu         sync.Mutex
		routeState *routeState
		generation *adaptiveState
	}

	routeState struct {
		mu           sync.Mutex
		generation   *adaptiveState
		lastEligible time.Time
		hasEligible  bool
	}

	routeDefinition struct {
		definition eskip.Route
		filter     *filter
	}

	routeRegistry struct {
		mu          sync.Mutex
		routeStates map[string]routeDefinition
	}

	spec struct {
		registry *routeRegistry
	}
)

// New returns the hedge filter specification.
func New() filters.Spec {
	return &spec{registry: newRouteRegistry()}
}

func (*spec) Name() string {
	return filters.HedgeName
}

func (*spec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &filter{state: newRouteState()}, nil
}

// NewRoutePostProcessor returns an independent postprocessor registry for a route population.
func (*spec) NewRoutePostProcessor() routing.PostProcessor {
	return newRouteRegistry()
}

func (s *spec) Do(routes []*routing.Route) []*routing.Route {
	return s.registry.Do(routes)
}

func (f *filter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.HedgeStateKey] = f.state.requestState()
}

func (*filter) Response(filters.FilterContext) {}

func newRouteState() *routeState {
	return &routeState{generation: newAdaptiveState()}
}

func newAdaptiveState() *adaptiveState {
	return &adaptiveState{
		headerLatencies: metrics.NewExpDecaySample(headerLatencySampleSize, headerLatencySampleAlpha),
	}
}

func (s *routeState) requestState() *requestState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return &requestState{routeState: s, generation: s.generation}
}

func (s *requestState) RecordEligibleRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := timeNow()
	s.routeState.mu.Lock()
	if s.routeState.hasEligible && now.Sub(s.routeState.lastEligible) >= idleStateResetDuration {
		s.routeState.generation = newAdaptiveState()
	}
	s.routeState.hasEligible = true
	s.routeState.lastEligible = now
	s.generation = s.routeState.generation
	generation := s.generation
	s.routeState.mu.Unlock()

	generation.recordEligibleRequest()
}

func (s *requestState) TryReserveRetry() bool {
	return s.currentGeneration().tryReserveRetry()
}

func (s *requestState) CommitRetry() {
	s.currentGeneration().commitRetry()
}

func (s *requestState) ReleaseRetry() {
	s.currentGeneration().releaseRetry()
}

func (s *requestState) ObserveResponseHeaderLatency(latency time.Duration) {
	s.currentGeneration().observeResponseHeaderLatency(latency)
}

func (s *requestState) HedgeDelay() (time.Duration, bool) {
	return s.currentGeneration().hedgeDelay()
}

func (s *requestState) currentGeneration() *adaptiveState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

func (s *adaptiveState) recordEligibleRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.retryAvailable || s.retryReserved {
		return
	}
	s.eligibleRequests++
	if s.eligibleRequests == eligibleRequestsPerRetry {
		s.eligibleRequests = 0
		s.retryAvailable = true
	}
}

func (s *adaptiveState) tryReserveRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.retryAvailable {
		return false
	}
	s.retryAvailable = false
	s.retryReserved = true
	return true
}

func (s *adaptiveState) commitRetry() {
	s.mu.Lock()
	s.retryReserved = false
	s.mu.Unlock()
}

func (s *adaptiveState) releaseRetry() {
	s.mu.Lock()
	if s.retryReserved {
		s.retryReserved = false
		s.retryAvailable = true
	}
	s.mu.Unlock()
}

func (s *adaptiveState) observeResponseHeaderLatency(latency time.Duration) {
	if latency < 0 {
		return
	}
	s.headerLatencies.Update(int64(latency))
}

func (s *adaptiveState) hedgeDelay() (time.Duration, bool) {
	if s.headerLatencies.Count() < headerLatencyWarmup {
		return 0, false
	}
	return time.Duration(s.headerLatencies.Percentile(headerLatencyPercentile)), true
}

func newRouteRegistry() *routeRegistry {
	return &routeRegistry{routeStates: make(map[string]routeDefinition)}
}

func (s *routeRegistry) Do(routes []*routing.Route) []*routing.Route {
	s.mu.Lock()
	defer s.mu.Unlock()

	routeStates := make(map[string]routeDefinition, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}

		var stateFilter *filter
		for _, routeFilter := range route.Filters {
			if routeFilter == nil {
				continue
			}
			if f, ok := routeFilter.Filter.(*filter); ok {
				stateFilter = f
				break
			}
		}
		if stateFilter == nil {
			continue
		}

		definition := route.Route
		definition.Id = ""
		definition.LBEndpoints = nil
		if previous, ok := s.routeStates[route.Id]; ok && reflect.DeepEqual(previous.definition, definition) {
			stateFilter = previous.filter
		}
		for _, routeFilter := range route.Filters {
			if routeFilter == nil {
				continue
			}
			if _, ok := routeFilter.Filter.(*filter); ok {
				routeFilter.Filter = stateFilter
			}
		}
		routeStates[route.Id] = routeDefinition{definition: definition, filter: stateFilter}
	}
	s.routeStates = routeStates
	return routes
}

var (
	_ filters.Filter        = (*filter)(nil)
	_ filters.HedgeState    = (*requestState)(nil)
	_ routing.PostProcessor = (*spec)(nil)
	_ routing.PostProcessor = (*routeRegistry)(nil)
)
