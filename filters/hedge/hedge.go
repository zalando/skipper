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
		routeState       *routeState
	}

	filter struct {
		state *routeState
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

// New returns the hedge filter specification and its route-state registry. When
// building routing options directly, register the specification as a routing
// postprocessor to preserve state across route updates.
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

// NewRoutePostProcessor returns an independent state registry for another
// route population, such as priority routes. Apply it only to that population.
func (*spec) NewRoutePostProcessor() routing.PostProcessor {
	return newRouteRegistry()
}

func (s *spec) Do(routes []*routing.Route) []*routing.Route {
	return s.registry.Do(routes)
}

func (f *filter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.HedgeStateKey] = f.state.currentGeneration()
}

func (*filter) Response(filters.FilterContext) {}

func newRouteState() *routeState {
	s := &routeState{}
	s.generation = newAdaptiveState(s)
	return s
}

func newAdaptiveState(routeState *routeState) *adaptiveState {
	return &adaptiveState{
		headerLatencies: metrics.NewExpDecaySample(headerLatencySampleSize, headerLatencySampleAlpha),
		routeState:      routeState,
	}
}

func (s *routeState) currentGeneration() *adaptiveState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

func (s *routeState) recordEligibleRequest() *adaptiveState {
	now := timeNow()
	s.mu.Lock()
	if s.hasEligible && now.Sub(s.lastEligible) >= idleStateResetDuration {
		s.generation = newAdaptiveState(s)
	}
	s.hasEligible = true
	s.lastEligible = now
	generation := s.generation
	s.mu.Unlock()

	generation.recordEligibleRequest()
	return generation
}

func (s *adaptiveState) RecordEligibleRequest() filters.HedgeState {
	return s.routeState.recordEligibleRequest()
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

func (s *adaptiveState) TryReserveRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.retryAvailable {
		return false
	}
	s.retryAvailable = false
	s.retryReserved = true
	return true
}

func (s *adaptiveState) CommitRetry() {
	s.mu.Lock()
	s.retryReserved = false
	s.mu.Unlock()
}

func (s *adaptiveState) ReleaseRetry() {
	s.mu.Lock()
	if s.retryReserved {
		s.retryReserved = false
		s.retryAvailable = true
	}
	s.mu.Unlock()
}

func (s *adaptiveState) ObserveResponseHeaderLatency(latency time.Duration) {
	if latency < 0 {
		return
	}
	s.headerLatencies.Update(int64(latency))
}

func (s *adaptiveState) HedgeDelay() (time.Duration, bool) {
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
	_ filters.HedgeState    = (*adaptiveState)(nil)
	_ routing.PostProcessor = (*spec)(nil)
	_ routing.PostProcessor = (*routeRegistry)(nil)
)
