package teeloopback

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy/backendtest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"net/http"
	"sync"
	"testing"
	"time"
)

func waitForAll(channels []backendtest.Done, done chan struct{}) {
	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(ch backendtest.Done) {
			<-ch
			wg.Done()
		}(ch)
	}
	wg.Wait()
	close(done)
}

func TestLoopbackAndMatchPredicate(t *testing.T) {
	// Test split and shadow routes are used:
	// the backend set in the split route should serve the request issued by the client
	// the backend set in the shadow route should serve the request issued by the teeLoopback
	// the backend set in the original route should not be called
	const routeDoc = `
		original: Path("/foo") -> "%v";
	 	split: Path("/foo") && True()  -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && True() && Tee("A") -> "%v";
	`
	original := backendtest.NewBackendRecorder(0)
	split := backendtest.NewBackendRecorder(1)
	shadow := backendtest.NewBackendRecorder(1)

	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, original.GetURL(), split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			tee.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	allDone := make(chan struct{}, 3)
	go waitForAll([]backendtest.Done{
		shadow.Done,
		split.Done,
		original.Done,
	}, allDone)
	select {
	case <-allDone:
	case <-time.After(time.Second * 1):
		t.Error("teeloopback: time exceeded while waiting for requests")
	}
	if shadow.GetServedRequests() != 1 && split.GetServedRequests() != 1 {
		t.Errorf("teeloopback: expected to receive 1 requests in split and shadow backend. Split: %d, Shadow: %d", split.GetServedRequests(), shadow.GetServedRequests())
	}
	if original.GetServedRequests() > 0 {
		t.Error("teeloopback: backend of original route should not receive requests")
	}
}

func TestPreventInfiniteLoopback(t *testing.T) {
	// Test split route does not do recursive lookups when teeLoopback calls resolves to the same route.
	// the backend set in the split route should serve no more than 2 requests
	// the backend set in the shadow route should not serve any request.
	const routeDoc = `
	 	split: Path("/foo") && True() -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && True() && Tee("B") -> "%v";
	`
	split := backendtest.NewBackendRecorder(2)
	shadow := backendtest.NewBackendRecorder(0)
	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			tee.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	allDone := make(chan struct{}, 2)
	go waitForAll([]backendtest.Done{
		shadow.Done,
		split.Done,
	}, allDone)
	select {
	case <-allDone:
	case <-time.After(time.Second * 1):
		t.Error("teeloopback: time exceeded while waiting for requests")
	}
	if shadow.GetServedRequests() != 0 && split.GetServedRequests() != 2 {
		t.Errorf("teeloopback: expected to receive 2 requests in split and 0 in shadow backend. Split: %d, Shadow: %d", split.GetServedRequests(), shadow.GetServedRequests())
	}
}
