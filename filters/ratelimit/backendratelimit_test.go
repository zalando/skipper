package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/ratelimit"
)

func TestBackendRatelimit(t *testing.T) {
	spec := NewBackendRatelimit()
	if spec.Name() != "backendRatelimit" {
		t.Error("wrong name")
	}

	f, err := spec.CreateFilter([]interface{}{"api", 22, "7s"})
	if err != nil {
		t.Fatal("failed to create filter")
	}

	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: make(map[string]interface{})}
	f.Request(c)

	settings, ok := c.FStateBag[filters.BackendRatelimit].(ratelimit.Settings)
	if !ok {
		t.Fatal("settings expected")
	}
	expected := ratelimit.Settings{
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      "backend.api",
		MaxHits:    22,
		TimeWindow: 7 * time.Second,
	}
	if settings != expected {
		t.Fatalf("wrong settings, expected: %v, got %v", expected, settings)
	}

	// second filter overwrites
	f, _ = spec.CreateFilter([]interface{}{"api2", 355, "113s"})
	f.Request(c)

	settings, ok = c.FStateBag[filters.BackendRatelimit].(ratelimit.Settings)
	if !ok {
		t.Fatal("settings overwrite expected")
	}
	expected = ratelimit.Settings{
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      "backend.api2",
		MaxHits:    355,
		TimeWindow: 113 * time.Second,
	}
	if settings != expected {
		t.Fatalf("wrong settings overwrite, expected: %v, got %v", expected, settings)
	}
}
