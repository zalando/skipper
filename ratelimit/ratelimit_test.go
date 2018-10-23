package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

func checkRatelimitted(t *testing.T, rl *Ratelimit, client string) {
	if rl.Allow(client) {
		t.Errorf("request is allowed for %s, but expected to be rate limitted", client)
	}
}

func checkNotRatelimitted(t *testing.T, rl *Ratelimit, client string) {
	if !rl.Allow(client) {
		t.Errorf("request is rate limitted for %s, but expected to be allowed", client)
	}
}

func TestServiceRatelimit(t *testing.T) {
	s := Settings{
		Type:          ServiceRatelimit,
		MaxHits:       3,
		TimeWindow:    3 * time.Second,
		CleanInterval: 4 * time.Second,
	}
	client1 := "foo"
	client2 := "bar"

	waitClean := func() {
		time.Sleep(s.TimeWindow)
	}

	t.Run("new service ratelimitter", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls, all clients are ratelimitted", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}

		checkRatelimitted(t, rl, client1)
		checkRatelimitted(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		for i := 0; i < s.MaxHits-1; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		waitClean()
		checkNotRatelimitted(t, rl, client1)
	})
}

func TestLocalRatelimit(t *testing.T) {
	s := Settings{
		Type:          LocalRatelimit,
		MaxHits:       3,
		TimeWindow:    3 * time.Second,
		CleanInterval: 4 * time.Second,
	}
	client1 := "foo"
	client2 := "bar"

	waitClean := func() {
		time.Sleep(s.TimeWindow)
	}

	t.Run("new local ratelimitter", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}

		checkRatelimitted(t, rl, client1)
		checkNotRatelimitted(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		for i := 0; i < s.MaxHits-1; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		waitClean()
		checkNotRatelimitted(t, rl, client1)
	})
}

func TestDisableRatelimit(t *testing.T) {
	s := Settings{
		Type:          DisableRatelimit,
		MaxHits:       6,
		TimeWindow:    3 * time.Second,
		CleanInterval: 6 * time.Second,
	}

	client1 := "foo"

	t.Run("new disabled ratelimitter", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("disable ratelimitter should never rate limit", func(t *testing.T) {
		rl := newRatelimit(s, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		checkNotRatelimitted(t, rl, client1)
	})
}

func BenchmarkServiceRatelimit(b *testing.B) {
	maxint := 1 << 21
	s := Settings{
		Type:       ServiceRatelimit,
		MaxHits:    maxint,
		TimeWindow: 1 * time.Second,
	}

	rl := newRatelimit(s, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow("")
	}
}

func BenchmarkLocalRatelimit(b *testing.B) {
	maxint := 1 << 21
	s := Settings{
		Type:          LocalRatelimit,
		MaxHits:       maxint,
		TimeWindow:    1 * time.Second,
		CleanInterval: 30 * time.Second,
	}
	client := "foo"

	rl := newRatelimit(s, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(client)
	}
}

func BenchmarkLocalRatelimitWithCleaner(b *testing.B) {
	maxint := 100
	s := Settings{
		Type:          LocalRatelimit,
		MaxHits:       maxint,
		TimeWindow:    100 * time.Millisecond,
		CleanInterval: 300 * time.Millisecond,
	}
	client := "foo"

	rl := newRatelimit(s, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(client)
	}
}

func BenchmarkLocalRatelimitClients1000(b *testing.B) {
	s := Settings{
		Type:          LocalRatelimit,
		MaxHits:       100,
		TimeWindow:    1 * time.Second,
		CleanInterval: 30 * time.Second,
	}
	client := "foo"
	count := 1000
	clients := make([]string, 0, count)
	for i := 0; i < count; i++ {
		clients = append(clients, fmt.Sprintf("%s-%d", client, i))
	}

	rl := newRatelimit(s, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(clients[i%count])
	}
}

func BenchmarkLocalRatelimitWithCleanerClients1000(b *testing.B) {
	maxint := 100
	s := Settings{
		Type:          LocalRatelimit,
		MaxHits:       maxint,
		TimeWindow:    100 * time.Millisecond,
		CleanInterval: 300 * time.Millisecond,
	}
	client := "foo"
	count := 1000
	clients := make([]string, 0, count)
	for i := 0; i < count; i++ {
		clients = append(clients, fmt.Sprintf("%s-%d", client, i))
	}

	rl := newRatelimit(s, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(clients[i%count])
	}
}
