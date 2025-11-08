package tee

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/source"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/backendtest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

const listenFor = 10 * time.Millisecond

func waitForAll(handlers ...*backendtest.BackendRecorderHandler) {
	for _, h := range handlers {
		h.Done()
	}
}

func matchRequestsCount(handler *backendtest.BackendRecorderHandler, count int) bool {
	return len(handler.GetRequests()) == count
}

func TestLoopbackAndMatchPredicate(t *testing.T) {
	// Test split and shadow routes are used:
	// the backend set in the split route should serve the request issued by the client
	// the backend set in the shadow route should serve the request issued by the teeLoopback
	// the backend set in the original route should not be called
	const routeDoc = `
		original: Path("/foo") -> "%v";
	 	split: Path("/foo") && Traffic(1) -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && Traffic(1) && Tee("A") -> "%v";
	`
	original := backendtest.NewBackendRecorder(listenFor)
	split := backendtest.NewBackendRecorder(listenFor)
	shadow := backendtest.NewBackendRecorder(listenFor)

	routes := eskip.MustParse(fmt.Sprintf(routeDoc, original.GetURL(), split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			traffic.New(),
		},
	}, routes...)
	defer p.Close()

	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	assert.Eventually(t, func() bool {
		return matchRequestsCount(split, 1) && matchRequestsCount(shadow, 1)
	}, 100*time.Millisecond, 10*time.Millisecond, "teeloopback: expected to receive 1 requests in split and shadow backend but got Split: %d, Shadow: %d", len(split.GetRequests()), len(shadow.GetRequests()))
	waitForAll(split, original, shadow)
	if !matchRequestsCount(original, 0) {
		t.Errorf("teeloopback: backend of original route should not receive requests but got %d", len(original.GetRequests()))
	}
}

func TestOriginalBackendServeEvenWhenShadowDoesNotReply(t *testing.T) {
	const routeDoc = `
		original: Path("/foo") -> "%v";
	 	split: Path("/foo") && Traffic(1)  -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && Traffic(1) && Tee("A") -> "%v";
	`
	original := backendtest.NewBackendRecorder(listenFor)
	split := backendtest.NewBackendRecorder(listenFor)

	const responseTimeout = 2 * time.Second
	shadow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * responseTimeout)
	}))
	defer shadow.Close()

	routes := eskip.MustParse(fmt.Sprintf(routeDoc, split.GetURL(), split.GetURL(), shadow.URL))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithParamsAndRoutingOptions(registry,
		proxy.Params{
			ResponseHeaderTimeout: responseTimeout,
			CloseIdleConnsPeriod:  -time.Second,
		},
		routing.Options{
			Predicates: []routing.PredicateSpec{
				teePredicate.New(),
				traffic.New(),
			},
		}, routes...)
	defer p.Close()

	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	waitForAll(original, split)

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
	 	split: Path("/foo") && Traffic(1) -> teeLoopback("A") ->"%v";
		shadow: Path("/foo") && Traffic(1) && Tee("A") -> "%v";
	`
	split := backendtest.NewBackendRecorder(listenFor)
	routes := eskip.MustParse(fmt.Sprintf(routeDoc, split.GetURL(), split.GetURL(), "http://fakeurl"))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			traffic.New(),
		},
	}, routes...)
	defer p.Close()

	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	split.Done()
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

	routes := eskip.MustParse(fmt.Sprintf(routeDoc, split.GetURL(), shadow.GetURL()))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
		},
	}, routes...)
	defer p.Close()

	_, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Error("teeloopback: failed to execute the request.", err)
	}
	waitForAll(split, shadow)
	if !matchRequestsCount(shadow, 9) || !matchRequestsCount(split, 1) {
		t.Errorf("teeloopback: expected to receive 1 requests in the split backend and 9 in shadow backend but got Split: %d, Shadow: %d", len(split.GetRequests()), len(shadow.GetRequests()))
	}
}

func TestLoopbackWithClientIP(t *testing.T) {
	const routeFmt = `
		split: Path("/foo") && ClientIP("0.0.0.0/0", "::/0") -> teeLoopback("A") -> "%v";
		shadow: Path("/foo") && ClientIP("0.0.0.0/0", "::/0") && Tee("A") -> "%v";
	`

	const listenFor = 30 * time.Millisecond
	split := backendtest.NewBackendRecorder(listenFor)
	shadow := backendtest.NewBackendRecorder(listenFor)

	routeDoc := fmt.Sprintf(routeFmt, split.GetURL(), shadow.GetURL())
	routes := eskip.MustParse(routeDoc)

	filterRegistry := make(filters.Registry)
	filterRegistry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(filterRegistry, routing.Options{
		Predicates: []routing.PredicateSpec{
			teePredicate.New(),
			source.NewClientIP(),
		},
	}, routes...)
	defer p.Close()

	rsp, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()

	waitForAll(split, shadow)
	if !matchRequestsCount(split, 1) || !matchRequestsCount(shadow, 1) {
		t.Errorf(
			"failed to receive the right number of requests on the backend; split: %d; shadow: %d",
			len(split.GetRequests()),
			len(shadow.GetRequests()),
		)
	}
}
