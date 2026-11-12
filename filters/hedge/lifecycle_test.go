package hedge

import (
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/routing"
)

func TestHedgeIdleResetUsesFreshGeneration(t *testing.T) {
	originalTimeNow := timeNow
	currentTime := time.Unix(100, 0)
	timeNow = func() time.Time { return currentTime }
	defer func() { timeNow = originalTimeNow }()

	spec := New()
	postProcessor := spec.(routing.PostProcessor)
	createRoute := func() *routing.Route {
		filter, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &routing.Route{
			Route: eskip.Route{
				Id:          "idle-route",
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

	initial := postProcessor.Do([]*routing.Route{createRoute()})[0]
	oldState := stateFor(initial)
	oldState = oldState.RecordEligibleRequest()
	for sample := 1; sample <= 100; sample++ {
		oldState.ObserveResponseHeaderLatency(time.Duration(sample) * time.Millisecond)
	}
	for request := 1; request < 20; request++ {
		oldState = oldState.RecordEligibleRequest()
	}

	currentTime = currentTime.Add(time.Minute - time.Nanosecond)
	beforeIdle := postProcessor.Do([]*routing.Route{createRoute()})[0]
	beforeIdleState := stateFor(beforeIdle)
	beforeIdleState = beforeIdleState.RecordEligibleRequest()
	if _, ok := beforeIdleState.HedgeDelay(); !ok {
		t.Fatal("route reset before one minute without eligible requests")
	}

	currentTime = currentTime.Add(time.Minute)
	afterIdle := postProcessor.Do([]*routing.Route{createRoute()})[0]
	afterIdleState := stateFor(afterIdle)
	afterIdleState = afterIdleState.RecordEligibleRequest()
	if _, ok := afterIdleState.HedgeDelay(); ok {
		t.Fatal("idle route retained response-header history")
	}
	if afterIdleState.TryReserveRetry() {
		t.Fatal("idle route retained retry allowance")
	}

	beforeIdleState.ObserveResponseHeaderLatency(101 * time.Millisecond)
	if _, ok := afterIdleState.HedgeDelay(); ok {
		t.Fatal("late response from the old generation contaminated reset history")
	}
}

func TestHedgeConcurrentOldAndNewGenerations(t *testing.T) {
	originalTimeNow := timeNow
	currentTime := time.Unix(200, 0)
	timeNow = func() time.Time { return currentTime }
	defer func() { timeNow = originalTimeNow }()

	spec := New()
	postProcessor := spec.(routing.PostProcessor)
	createRoute := func() *routing.Route {
		filter, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &routing.Route{
			Route: eskip.Route{
				Id:          "concurrent-route",
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

	oldRoute := postProcessor.Do([]*routing.Route{createRoute()})[0]
	oldState := stateFor(oldRoute)
	oldState = oldState.RecordEligibleRequest()
	for request := 1; request < 20; request++ {
		oldState = oldState.RecordEligibleRequest()
	}
	if !oldState.TryReserveRetry() {
		t.Fatal("old generation did not earn its allowance")
	}

	currentTime = currentTime.Add(time.Minute)
	newRoute := postProcessor.Do([]*routing.Route{createRoute()})[0]
	newState := stateFor(newRoute)
	newState = newState.RecordEligibleRequest()

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for sample := 0; sample < 100; sample++ {
			oldState.ObserveResponseHeaderLatency(time.Millisecond)
		}
	}()
	go func() {
		defer workers.Done()
		for sample := 0; sample < 99; sample++ {
			newState.ObserveResponseHeaderLatency(time.Millisecond)
		}
	}()
	workers.Wait()

	if _, ok := newState.HedgeDelay(); ok {
		t.Fatal("old generation responses contaminated new generation history")
	}
	newState.ObserveResponseHeaderLatency(time.Millisecond)
	if _, ok := newState.HedgeDelay(); !ok {
		t.Fatal("new generation did not record its own response-header samples")
	}
	if newState.TryReserveRetry() {
		t.Fatal("new generation inherited the old retry allowance")
	}
}
