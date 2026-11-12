package hedge_test

import (
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/hedge"
	"github.com/zalando/skipper/routing"
)

type hedgeAdaptiveState interface {
	filters.HedgeState
}

func TestHedgeFilterUsesObservedHeaderP95AfterWarmup(t *testing.T) {
	filter, err := hedge.New().CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &filtertest.Context{FStateBag: make(map[string]any)}
	filter.Request(ctx)
	state, ok := ctx.FStateBag[filters.HedgeStateKey].(hedgeAdaptiveState)
	if !ok {
		t.Fatal("hedge filter state does not expose response-header sampling")
	}

	for sample := 1; sample < 100; sample++ {
		state.ObserveResponseHeaderLatency(time.Duration(sample) * time.Millisecond)
	}
	if _, ok := state.HedgeDelay(); ok {
		t.Fatal("hedge delay became available before 100 response-header samples")
	}

	state.ObserveResponseHeaderLatency(100 * time.Millisecond)
	delay, ok := state.HedgeDelay()
	if !ok {
		t.Fatal("hedge delay is unavailable after 100 response-header samples")
	}
	if delay < 95*time.Millisecond || delay > 96*time.Millisecond {
		t.Fatalf("hedge delay = %v, want the observed p95 near 95 ms", delay)
	}
}

func TestHedgeStateContinuesAcrossEndpointUpdates(t *testing.T) {
	spec := hedge.New()
	postProcessor, ok := spec.(routing.PostProcessor)
	if !ok {
		t.Fatal("hedge filter specification does not preserve state across route updates")
	}

	createRoute := func(endpoints ...string) *routing.Route {
		filter, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &routing.Route{
			Route: eskip.Route{
				Id:          "items",
				Method:      "GET",
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: eskip.NewLBEndpoints(endpoints),
			},
			Filters: []*routing.RouteFilter{{Filter: filter}},
		}
	}
	stateFor := func(route *routing.Route) hedgeAdaptiveState {
		ctx := &filtertest.Context{FStateBag: make(map[string]any)}
		route.Filters[0].Request(ctx)
		state, ok := ctx.FStateBag[filters.HedgeStateKey].(hedgeAdaptiveState)
		if !ok {
			t.Fatal("hedge filter state is unavailable through the public Filter interface")
		}
		return state
	}

	initial := postProcessor.Do([]*routing.Route{createRoute("http://one.example")})[0]
	initialState := stateFor(initial)
	for sample := 1; sample <= 100; sample++ {
		initialState.ObserveResponseHeaderLatency(time.Duration(sample) * time.Millisecond)
	}
	for request := 0; request < 20; request++ {
		initialState.RecordEligibleRequest()
	}

	updated := postProcessor.Do([]*routing.Route{createRoute("http://one.example", "http://two.example")})[0]
	updatedState := stateFor(updated)
	if delay, ok := updatedState.HedgeDelay(); !ok || delay == 0 {
		t.Fatalf("updated route delay = %v, available = %v; want retained response-header history", delay, ok)
	}
	if !updatedState.TryReserveRetry() {
		t.Fatal("updated route did not retain the earned retry allowance")
	}
	updatedState.CommitRetry()
	for request := 0; request < 20; request++ {
		updatedState.RecordEligibleRequest()
	}

	shrunk := postProcessor.Do([]*routing.Route{createRoute("http://two.example")})[0]
	shrunkState := stateFor(shrunk)
	if delay, ok := shrunkState.HedgeDelay(); !ok || delay == 0 {
		t.Fatalf("shrunk route delay = %v, available = %v; want retained response-header history", delay, ok)
	}
	if !shrunkState.TryReserveRetry() {
		t.Fatal("shrunk route did not retain the earned retry allowance")
	}
}

func TestHedgePostProcessorSeparatesRoutePopulations(t *testing.T) {
	spec := hedge.New()
	normalPostProcessor := spec.(routing.PostProcessor)
	factory, ok := spec.(interface {
		NewRoutePostProcessor() routing.PostProcessor
	})
	if !ok {
		t.Fatal("hedge specification does not create an isolated route-population postprocessor")
	}
	priorityPostProcessor := factory.NewRoutePostProcessor()

	createRoute := func() *routing.Route {
		filter, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &routing.Route{
			Route: eskip.Route{
				Id:          "shared-route-id",
				Method:      "GET",
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: eskip.NewLBEndpoints([]string{"http://backend.example"}),
			},
			Filters: []*routing.RouteFilter{{Filter: filter}},
		}
	}
	stateFor := func(route *routing.Route) filters.HedgeState {
		ctx := &filtertest.Context{FStateBag: make(map[string]any)}
		route.Filters[0].Request(ctx)
		state, ok := ctx.FStateBag[filters.HedgeStateKey].(filters.HedgeState)
		if !ok {
			t.Fatal("hedge state is unavailable through the public Filter interface")
		}
		return state
	}
	seed := func(state filters.HedgeState) {
		for sample := 1; sample <= 100; sample++ {
			state.ObserveResponseHeaderLatency(time.Duration(sample) * time.Millisecond)
		}
		for request := 0; request < 20; request++ {
			state.RecordEligibleRequest()
		}
	}

	normal := normalPostProcessor.Do([]*routing.Route{createRoute()})[0]
	normalState := stateFor(normal)
	seed(normalState)

	priority := priorityPostProcessor.Do([]*routing.Route{createRoute()})[0]
	priorityState := stateFor(priority)
	if _, ok := priorityState.HedgeDelay(); ok {
		t.Fatal("priority route shared history with a normal route of the same ID")
	}
	if priorityState.TryReserveRetry() {
		t.Fatal("priority route shared retry allowance with a normal route of the same ID")
	}
	seed(priorityState)

	priorityPostProcessor.Do(nil)
	normalUpdated := normalPostProcessor.Do([]*routing.Route{createRoute()})[0]
	normalUpdatedState := stateFor(normalUpdated)
	if _, ok := normalUpdatedState.HedgeDelay(); !ok {
		t.Fatal("priority route pruning removed normal route history")
	}
	if !normalUpdatedState.TryReserveRetry() {
		t.Fatal("priority route pruning removed normal route retry allowance")
	}
	normalUpdatedState.CommitRetry()
	for request := 0; request < 20; request++ {
		normalUpdatedState.RecordEligibleRequest()
	}

	priorityFresh := priorityPostProcessor.Do([]*routing.Route{createRoute()})[0]
	priorityFreshState := stateFor(priorityFresh)
	if _, ok := priorityFreshState.HedgeDelay(); ok {
		t.Fatal("priority route retained state after its population was deleted")
	}
	seed(priorityFreshState)

	normalPostProcessor.Do(nil)
	priorityUpdated := priorityPostProcessor.Do([]*routing.Route{createRoute()})[0]
	priorityUpdatedState := stateFor(priorityUpdated)
	if _, ok := priorityUpdatedState.HedgeDelay(); !ok {
		t.Fatal("normal route pruning removed priority route history")
	}
	if !priorityUpdatedState.TryReserveRetry() {
		t.Fatal("normal route pruning removed priority route retry allowance")
	}
}

func TestHedgeStateResetsForChangedDefinitionAndIsolatedByRouteID(t *testing.T) {
	spec := hedge.New()
	postProcessor := spec.(routing.PostProcessor)
	createRoute := func(id, method string) *routing.Route {
		filter, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &routing.Route{
			Route: eskip.Route{
				Id:          id,
				Method:      method,
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: eskip.NewLBEndpoints([]string{"http://backend.example"}),
			},
			Filters: []*routing.RouteFilter{{Filter: filter}},
		}
	}
	stateFor := func(route *routing.Route) hedgeAdaptiveState {
		ctx := &filtertest.Context{FStateBag: make(map[string]any)}
		route.Filters[0].Request(ctx)
		state, ok := ctx.FStateBag[filters.HedgeStateKey].(hedgeAdaptiveState)
		if !ok {
			t.Fatal("hedge filter state is unavailable through the public Filter interface")
		}
		return state
	}

	oldRoute := postProcessor.Do([]*routing.Route{createRoute("items", "GET")})[0]
	oldState := stateFor(oldRoute)
	for sample := 1; sample <= 100; sample++ {
		oldState.ObserveResponseHeaderLatency(time.Duration(sample) * time.Millisecond)
	}
	for request := 0; request < 20; request++ {
		oldState.RecordEligibleRequest()
	}

	routes := postProcessor.Do([]*routing.Route{
		createRoute("items", "HEAD"),
		createRoute("other-items", "GET"),
	})
	changedState := stateFor(routes[0])
	if _, ok := changedState.HedgeDelay(); ok {
		t.Fatal("changed route retained response-header history")
	}
	if changedState.TryReserveRetry() {
		t.Fatal("changed route retained the old retry allowance")
	}
	isolatedState := stateFor(routes[1])
	if _, ok := isolatedState.HedgeDelay(); ok {
		t.Fatal("different route ID shared response-header history")
	}
	if isolatedState.TryReserveRetry() {
		t.Fatal("different route ID shared the old retry allowance")
	}
	oldState.ObserveResponseHeaderLatency(101 * time.Millisecond)
	if _, ok := changedState.HedgeDelay(); ok {
		t.Fatal("completion from the old route contaminated reset state")
	}

	postProcessor.Do(nil)
	oldState.ObserveResponseHeaderLatency(102 * time.Millisecond)
	if _, ok := oldState.HedgeDelay(); !ok {
		t.Fatal("deleting the route invalidated state held by an in-flight request")
	}
	recreated := postProcessor.Do([]*routing.Route{createRoute("items", "HEAD")})[0]
	if _, ok := stateFor(recreated).HedgeDelay(); ok {
		t.Fatal("recreated route retained state from its deleted instance")
	}
}
