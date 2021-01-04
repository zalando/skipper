package ratelimit

import (
	"fmt"
	"net/http"
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

	t.Run("no nil dereference ratelimitter", func(t *testing.T) {
		checkNotRatelimitted(t, nil, client1)
	})

	t.Run("new service ratelimitter", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls, all clients are ratelimitted", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}

		checkRatelimitted(t, rl, client1)
		checkRatelimitted(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
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
		rl := newRatelimit(s, nil, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}

		checkRatelimitted(t, rl, client1)
		checkNotRatelimitted(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
		for i := 0; i < s.MaxHits-1; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		waitClean()
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("max hits 0", func(t *testing.T) {
		s := s
		s.MaxHits = 0
		rl := newRatelimit(s, nil, nil)
		checkRatelimitted(t, rl, client1)
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
		rl := newRatelimit(s, nil, nil)
		checkNotRatelimitted(t, rl, client1)
	})

	t.Run("disable ratelimitter should never rate limit", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimitted(t, rl, client1)
		}
		checkNotRatelimitted(t, rl, client1)
	})
}

func TestXForwardedForLookuper(t *testing.T) {
	req, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Errorf("Could not create request: %v", err)
	}

	t.Run("header lookuper X-Forwarded-For header", func(t *testing.T) {
		req.Header.Add("X-Forwarded-For", "127.0.0.3")
		lookuper := NewXForwardedForLookuper()
		if lookuper.Lookup(req) != "127.0.0.3" {
			t.Errorf("Failed to lookup request")
		}
	})

	t.Run("header lookuper X-Forwarded-For without header", func(t *testing.T) {
		lookuper := NewXForwardedForLookuper()
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Add("X-Forwarded-For", "127.0.0.2")
		req.Header.Add("X-Forwarded-For", "127.0.0.3")
		if s := lookuper.Lookup(req); s != "127.0.0.1" {
			t.Errorf("Failed to lookup request, got: %s", s)
		}
	})

}

func TestHeaderLookuper(t *testing.T) {
	req, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Errorf("Could not create request: %v", err)
	}

	t.Run("header lookuper authorization header", func(t *testing.T) {
		req.Header.Add("authorization", "foo")
		authLookuper := NewHeaderLookuper("authorizatioN")
		if authLookuper.Lookup(req) != "foo" {
			t.Errorf("Failed to lookup request")
		}
	})

	t.Run("header lookuper x header", func(t *testing.T) {
		req.Header.Add("x-blah", "bar")
		xLookuper := NewHeaderLookuper("x-bLAh")
		if xLookuper.Lookup(req) != "bar" {
			t.Errorf("Failed to lookup request")
		}
	})
}

func TestTupleLookuper(t *testing.T) {
	req, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Errorf("Could not create request: %v", err)
	}

	t.Run("header lookuper authorization header", func(t *testing.T) {
		req.Header.Add("authorization", "foo")
		req.Header.Add("bar", "meow")
		tupleLookuper := NewTupleLookuper(
			NewHeaderLookuper("authorizatioN"),
			NewHeaderLookuper("bar"),
		)
		if tupleLookuper.Lookup(req) != "foomeow" {
			t.Errorf("Failed to lookup request")
		}
	})

	t.Run("header lookuper x header", func(t *testing.T) {
		req.Header.Add("foo", "meow")
		req.Header.Add("x-blah", "bar")
		tupleLookuper := NewTupleLookuper(
			NewHeaderLookuper("x-blah"),
			NewHeaderLookuper("foo"),
		)
		if tupleLookuper.Lookup(req) != "barmeow" {
			t.Errorf("Failed to lookup request")
		}
	})
}

func BenchmarkServiceRatelimit(b *testing.B) {
	maxint := 1 << 21
	s := Settings{
		Type:       ServiceRatelimit,
		MaxHits:    maxint,
		TimeWindow: 1 * time.Second,
	}

	rl := newRatelimit(s, nil, nil)
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

	rl := newRatelimit(s, nil, nil)
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

	rl := newRatelimit(s, nil, nil)
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

	rl := newRatelimit(s, nil, nil)
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

	rl := newRatelimit(s, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(clients[i%count])
	}
}

func TestSettingsRatelimit(t *testing.T) {
	t.Run("ratelimit settings empty", func(t *testing.T) {
		s := Settings{}
		if !s.Empty() {
			t.Errorf("setting should be empty: %s", s)
		}

		s = Settings{
			Type:          ServiceRatelimit,
			MaxHits:       3,
			TimeWindow:    3 * time.Second,
			CleanInterval: 4 * time.Second,
		}
		if s.Empty() {
			t.Errorf("setting should not be empty: %s", s)
		}
	})

	t.Run("ratelimit settings stringer", func(t *testing.T) {
		s := Settings{
			Type:          ServiceRatelimit,
			MaxHits:       3,
			TimeWindow:    3 * time.Second,
			CleanInterval: 4 * time.Second,
		}

		if st := s.String(); st == "non" || st == "disable" {
			t.Errorf("Failed to get string version: %s", s)
		}

		s.Type = DisableRatelimit
		if s.String() != "disable" {
			t.Errorf("Failed to get disabled string version: %s", s)
		}
	})
}
