package apiusagemonitoring

import (
	"testing"

	"github.com/zalando/skipper/routing"
)

// When api usage monitoring is disabled the spec is a noop, but it must still
// implement routing.PostProcessor so the caller can register it unconditionally
// (no type-assertion guard). The noop Do returns the routes unchanged.
func TestDisabledSpecImplementsPostProcessor(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")

	pp, ok := spec.(routing.PostProcessor)
	if !ok {
		t.Fatal("disabled api usage monitoring spec must implement routing.PostProcessor")
	}

	routes := []*routing.Route{{}, {}}
	got := pp.Do(routes)

	if len(got) != len(routes) {
		t.Fatalf("noop Do must return the routes unchanged: got len %d, want %d", len(got), len(routes))
	}
	for i := range routes {
		if got[i] != routes[i] {
			t.Fatalf("noop Do must return the same route pointers: index %d differs", i)
		}
	}
}
