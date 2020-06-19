package tee

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/primitive"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy/backendtest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const listenFor = 10 * time.Millisecond

func waitForAll(handlers... *backendtest.BackendRecorderHandler) {
	for _, h := range handlers {
		<-h.Done
	}
}

func matchRequestsCount(handler *backendtest.BackendRecorderHandler, count int)bool {
	return len(handler.GetRequests()) == count
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
	original := backendtest.NewBackendRecorder(listenFor)
	split := backendtest.NewBackendRecorder(listenFor)
	shadow := backendtest.NewBackendRecorder(listenFor)

	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, original.GetURL(), split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	waitForAll(split, original, shadow)
	if !matchRequestsCount(shadow, 1) || !matchRequestsCount(split, 1) {
		t.Errorf("teeloopback: expected to receive 1 requests in split and shadow backend but got Split: %d, Shadow: %d", len(split.GetRequests()), len(shadow.GetRequests()))
	}
	if !matchRequestsCount(original, 0) {
		t.Errorf("teeloopback: backend of original route should not receive requests but got %d", len(original.GetRequests()))
	}
}

func TestOriginalBackendServeEvenWhenShadowDoesNotReply(t *testing.T) {
	const routeDoc = `
		original: Path("/foo") -> "%v";
	 	split: Path("/foo") && True()  -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && True() && Tee("A") -> "%v";
	`
	original := backendtest.NewBackendRecorder(listenFor)
	split := backendtest.NewBackendRecorder(listenFor)
	shadow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second * 120)
	}))
	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, split.GetURL(), split.GetURL(), shadow.URL))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	<-split.Done
	<-original.Done
	if !matchRequestsCount(split, 1) {
		t.Errorf("teeloopback: expected to receive 1 requests in split but got %d", len(split.GetRequests()))
	}

	if !matchRequestsCount(original, 0) {
		t.Errorf("teeloopback: backend of original route should not receive requests but got %d", len(original.GetRequests()))
	}
}

func TestOriginalBackendServeEvenWhenShadowIsDown(t *testing.T) {
	const routeDoc = `
		original: Path("/foo") -> "%v";
	 	split: Path("/foo") && True() -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && True() && Tee("A") -> "%v";
	`
	split := backendtest.NewBackendRecorder(listenFor)
	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, split.GetURL(), split.GetURL(), "http://fakeurl"))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	<-split.Done
	if !matchRequestsCount(split, 1) {
		t.Errorf("teeloopback: backend of split route should receive 1 request but got %d", len(split.GetRequests()))
	}
}

func TestInfiniteLoopback(t *testing.T) {
	const routeDoc = `
	 	split: Path("/foo") -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && Tee("A") -> teeLoopback("A") -> "%v";
	`
	listenFor := 30 * time.Millisecond
	split := backendtest.NewBackendRecorder(listenFor)
	shadow := backendtest.NewBackendRecorder(listenFor)

	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			primitive.NewTrue(),
		},
	}, routes...)
	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	waitForAll(split, shadow)
	if !matchRequestsCount(shadow, 9) || !matchRequestsCount(split, 1) {
		t.Errorf("teeloopback: expected to receive 1 requests in the split backend and 9 in shadow backend but got Split: %d, Shadow: %d", len(split.GetRequests()), len(shadow.GetRequests()))
	}
}
