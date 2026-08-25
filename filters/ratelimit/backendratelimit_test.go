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

	f, err := spec.CreateFilter([]any{"api", 22, "7s"})
	if err != nil {
		t.Fatal("failed to create filter")
	}

	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: make(map[string]any)}
	f.Request(c)

	limit, ok := c.FStateBag[filters.BackendRatelimit].(*BackendRatelimit)
	if !ok {
		t.Fatal("BackendRatelimit expected")
	}
	expected := ratelimit.Settings{
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      "backend.api",
		MaxHits:    22,
		TimeWindow: 7 * time.Second,
	}
	if limit.Settings != expected {
		t.Fatalf("wrong settings, expected: %v, got %v", expected, limit.Settings)
	}
	if limit.StatusCode != 503 {
		t.Fatalf("wrong status code, expected: 503, got %v", limit.StatusCode)
	}

	// second filter overwrites
	f, _ = spec.CreateFilter([]any{"api2", 355, "113s", 429})
	f.Request(c)

	limit, ok = c.FStateBag[filters.BackendRatelimit].(*BackendRatelimit)
	if !ok {
		t.Fatal("BackendRatelimit overwrite expected")
	}
	expected = ratelimit.Settings{
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      "backend.api2",
		MaxHits:    355,
		TimeWindow: 113 * time.Second,
	}
	if limit.Settings != expected {
		t.Fatalf("wrong settings overwrite, expected: %v, got %v", expected, limit.Settings)
	}
	if limit.StatusCode != 429 {
		t.Fatalf("wrong status code, expected: 429, got %v", limit.StatusCode)
	}
}
