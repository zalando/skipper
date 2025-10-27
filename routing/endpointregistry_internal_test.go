package routing

import (
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
)

func TestStats(t *testing.T) {
	e := newEntry()
	slot := e.curSlot.Load()
	e.totalRequests[slot].Store(10)
	e.totalFailedRoundTrips[slot].Store(8)
	e.IncRequests(IncRequestsOptions{
		FailedRoundTrip: true,
	})

	nextSlot := (slot + 1) % 2
	e.totalRequests[nextSlot].Store(5)
	e.totalFailedRoundTrips[nextSlot].Store(4)

	reg := NewEndpointRegistry(RegistryOptions{
		LastSeenTimeout:               30 * time.Second,
		PassiveHealthCheckEnabled:     false, // do not start goroutine
		StatsResetPeriod:              30 * time.Second,
		MinRequests:                   3,
		MinHealthCheckDropProbability: 0.5,
		MaxHealthCheckDropProbability: 0.9,
	})
	defer reg.Close()

	ep := "10.0.0.5:8080"
	reg.data.Store(ep, e)

	routes := []*Route{
		{
			Route: eskip.Route{
				BackendType: eskip.NetworkBackend,
			},
			Host: ep,
		},
	}
	reg.Do(routes)

	mtr := reg.GetMetrics(ep)
	if p := mtr.HealthCheckDropProbability(); p > 0.0 {
		t.Fatalf("Failed to get 0 drop probability at start got: %0.2f", p)
	}

	// populate values
	go reg.updateStats()
	time.Sleep(100 * time.Millisecond)

	mtr = reg.GetMetrics(ep)
	if p := mtr.HealthCheckDropProbability(); p < 0.5 || p > 0.9 {
		t.Fatalf("Failed to get drop probability: %0.2f", p)
	}

	// clear slots
	go reg.updateStats()
	time.Sleep(100 * time.Millisecond)
	go reg.updateStats()
	time.Sleep(100 * time.Millisecond)

	mtr = reg.GetMetrics(ep)
	if p := mtr.HealthCheckDropProbability(); p > 0.0 {
		t.Fatalf("Failed to reset drop probability got: %0.2f", p)
	}
}
