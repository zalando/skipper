package ratelimit

import (
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
		rl := newRatelimit(s)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls", func(t *testing.T) {
		rl := newRatelimit(s)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}

		checkRatelimitted(t, rl, client1)
		checkNotRatelimitted(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s)
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
		rl := newRatelimit(s)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("disable ratelimitter should never rate limit", func(t *testing.T) {
		rl := newRatelimit(s)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		checkNotRatelimitted(t, rl, client1)
	})
}
