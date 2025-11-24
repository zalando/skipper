package ratelimit

import (
	"net/http"
	"testing"
	"time"
)

// no checks, used for race detector
func TestRegistry(t *testing.T) {
	createSettings := func(maxHits int) Settings {
		return Settings{
			Type:          LocalRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    1 * time.Second,
			CleanInterval: 5 * time.Second,
		}
	}

	checkNil := func(t *testing.T, rl *Ratelimit) {
		if rl != nil {
			t.Error("unexpected ratelimit")
		}
	}

	checkNotNil := func(t *testing.T, rl *Ratelimit) {
		if rl == nil {
			t.Error("failed to receive a ratelimit")
		}
	}

	t.Run("no settings", func(t *testing.T) {
		r := NewRegistry(Settings{})
		defer r.Close()

		rl := r.Get(Settings{})
		checkNil(t, rl)
	})
	t.Run("with settings", func(t *testing.T) {
		s := createSettings(3)
		r := NewRegistry(s)
		defer r.Close()

		rl := r.Get(s)
		checkNotNil(t, rl)
	})
}

func TestCheck(t *testing.T) {
	createSettings := func(typ RatelimitType) Settings {
		return Settings{
			Type:          typ,
			MaxHits:       1,
			TimeWindow:    10 * time.Second,
			CleanInterval: 5 * time.Second,
		}
	}

	t.Run("local ratelimit", func(t *testing.T) {
		s := createSettings(LocalRatelimit)
		r := NewRegistry(s)
		defer r.Close()

		req, err := http.NewRequest("GET", "http://rate.test:1234/", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Add("X-Forwarded-For", "127.0.0.1")

		_, i := r.Check(req)
		if i != 0 {
			t.Fatalf("First request should not be rate limited: %d", i)
		}
		_, j := r.Check(req)
		if j != 10 {
			t.Fatalf("Second request should be rate limited, and show that we have %s to wait: %d", s.TimeWindow, j)
		}

		req, err = http.NewRequest("GET", "http://rate2.test:1234/", nil)
		if err != nil {
			t.Fatalf("Failed to create 2nd request: %v", err)
		}
		req.Header.Add("X-Forwarded-For", "127.0.0.2")
		_, i = r.Check(req)
		if i != 0 {
			t.Fatalf("First try on 2nd request should not be rate limited for new host: %d", i)
		}
	})

	t.Run("service ratelimit", func(t *testing.T) {
		s := createSettings(ServiceRatelimit)
		r := NewRegistry(s)
		defer r.Close()

		req, err := http.NewRequest("GET", "http://service-rate.test:1234/", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Add("X-Forwarded-For", "127.0.0.3")
		_, i := r.Check(req)
		if i != 0 {
			t.Fatalf("First request should not be rate limited: %d", i)
		}
		_, j := r.Check(req)
		if j != 10 {
			t.Fatalf("Second request should be rate limited, and show that we have %s to wait: %d", s.TimeWindow, j)
		}

		req, err = http.NewRequest("GET", "http://service-rate2.test:1234/", nil)
		if err != nil {
			t.Fatalf("Failed to create 2nd request: %v", err)
		}
		req.Header.Add("X-Forwarded-For", "127.0.0.4")
		_, i = r.Check(req)
		if i != 10 {
			t.Fatalf("First request to another host should be rate limited: %d", i)
		}
	})
}
