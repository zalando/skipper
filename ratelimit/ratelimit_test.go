package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

func checkRatelimited(t *testing.T, rl *Ratelimit, client string) {
	if rl.Allow(context.Background(), client) {
		t.Errorf("request is allowed for %s, but expected to be rate limited", client)
	}
}

func checkNotRatelimited(t *testing.T, rl *Ratelimit, client string) {
	if !rl.Allow(context.Background(), client) {
		t.Errorf("request is rate limited for %s, but expected to be allowed", client)
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
		checkNotRatelimited(t, nil, client1)
	})

	t.Run("new service ratelimitter", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		checkNotRatelimited(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls, all clients are ratelimited", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimited(t, rl, client1)
		}

		checkRatelimited(t, rl, client1)
		checkRatelimited(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		for i := 0; i < s.MaxHits-1; i++ {
			checkNotRatelimited(t, rl, client1)
		}
		waitClean()
		checkNotRatelimited(t, rl, client1)
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
		rl := newRatelimit(s, nil, nil, nil)
		defer rl.Close()

		checkNotRatelimited(t, rl, client1)
	})

	t.Run("does not rate limit unless we have enough calls", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		defer rl.Close()

		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimited(t, rl, client1)
		}

		checkRatelimited(t, rl, client1)
		checkNotRatelimited(t, rl, client2)
	})

	t.Run("does not rate limit if TimeWindow is over", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		defer rl.Close()

		for i := 0; i < s.MaxHits-1; i++ {
			checkNotRatelimited(t, rl, client1)
		}
		waitClean()
		checkNotRatelimited(t, rl, client1)
	})

	t.Run("max hits 0", func(t *testing.T) {
		s := s
		s.MaxHits = 0
		rl := newRatelimit(s, nil, nil, nil)
		defer rl.Close()

		checkRatelimited(t, rl, client1)
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
		rl := newRatelimit(s, nil, nil, nil)
		checkNotRatelimited(t, rl, client1)
	})

	t.Run("disable ratelimitter should never rate limit", func(t *testing.T) {
		rl := newRatelimit(s, nil, nil, nil)
		for i := 0; i < s.MaxHits; i++ {
			checkNotRatelimited(t, rl, client1)
		}
		checkNotRatelimited(t, rl, client1)
	})
}

func TestSameBucketLookuper(t *testing.T) {
	req, err := http.NewRequest("GET", "/foo", nil)
	if err != nil {
		t.Errorf("Could not create request: %v", err)
	}

	lookuper := NewSameBucketLookuper()
	if s := lookuper.Lookup(req); s != sameBucket {
		t.Errorf("Failed to lookup request %s != %s", s, sameBucket)
	}

	if s := lookuper.String(); s != "SameBucketLookuper" {
		t.Errorf("Failed to lookuper.String(): %s", s)
	}
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

		if s := lookuper.String(); s != "XForwardedForLookuper" {
			t.Errorf("Failed to lookuper.String(): %s", s)
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

		if s := authLookuper.String(); s != "HeaderLookuper" {
			t.Errorf("Failed to authLookuper.String(): %s", s)
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

		if s := tupleLookuper.String(); s != "TupleLookuper" {
			t.Errorf("Failed to tupleLookuper.String(): %s", s)
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

	t.Run("nil tuple lookuper", func(t *testing.T) {
		tupleLookuper := NewTupleLookuper()
		tupleLookuper.l = nil
		if s := tupleLookuper.Lookup(req); s != "" {
			t.Errorf("Failed to get empty result for nil lookuper: %s", s)
		}
	})
}

func TestRoundRobinLookuper(t *testing.T) {
	for _, tc := range []struct {
		n, concurrency, iterations int
	}{
		{1, 1, 1},
		{1, 100, 100},
		{2, 100, 100},
		{3, 100, 100},
		{10, 100, 100},
		{11, 100, 100},
		{13, 17, 23},
	} {
		t.Run(fmt.Sprintf("n=%d, concurrency=%d, iterations=%d", tc.n, tc.concurrency, tc.iterations), func(t *testing.T) {
			lookuper := NewRoundRobinLookuper(uint64(tc.n))
			if l, _ := lookuper.(*RoundRobinLookuper); l.String() != "RoundRobinLookuper" {
				t.Errorf("Failed to lookuper.String(): %s", l.String())
			}

			buckets := testRoundRobinLookuper(lookuper, tc.concurrency, tc.iterations)
			if len(buckets) != tc.n {
				t.Errorf("expected %d buckets, got %d", tc.n, len(buckets))
			}
			maxPerBucket := (tc.concurrency * tc.iterations / tc.n) + 1
			for key, count := range buckets {
				if count > maxPerBucket {
					t.Errorf("expected max %d request for bucket %s, got %d", maxPerBucket, key, count)
				}
			}
		})
	}
}

func testRoundRobinLookuper(lookuper Lookuper, concurrency, iterations int) map[string]int {
	ch := make(chan map[string]int, concurrency)
	var wg sync.WaitGroup
	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buckets := make(map[string]int)
			for i := 0; i < iterations; i++ {
				r, _ := http.NewRequest("GET", "/foo", nil)
				buckets[lookuper.Lookup(r)]++
			}
			ch <- buckets
		}()
	}
	wg.Wait()
	close(ch)

	result := make(map[string]int)
	for b := range ch {
		for key, count := range b {
			result[key] += count
		}
	}
	return result
}

func BenchmarkServiceRatelimit(b *testing.B) {
	maxint := 1 << 21
	s := Settings{
		Type:       ServiceRatelimit,
		MaxHits:    maxint,
		TimeWindow: 1 * time.Second,
	}

	rl := newRatelimit(s, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(context.Background(), "")
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

	rl := newRatelimit(s, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(context.Background(), client)
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

	rl := newRatelimit(s, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(context.Background(), client)
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

	rl := newRatelimit(s, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(context.Background(), clients[i%count])
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

	rl := newRatelimit(s, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		rl.Allow(context.Background(), clients[i%count])
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

func TestUnmarshalYaml(t *testing.T) {
	for _, tt := range []struct {
		name    string
		yml     string
		wantErr bool
		want    Settings
	}{
		{
			name: "test deprecated local ratelimit",
			yml: `type: local
max-hits: 100
time-window: 10s`,
			wantErr: false,
			want: Settings{
				Type:          ClientRatelimit,
				MaxHits:       100,
				TimeWindow:    10 * time.Second,
				Group:         "",
				CleanInterval: 10 * time.Second * 10,
			},
		},
		{
			name: "test ratelimit",
			yml: `type: service
max-hits: 100
time-window: 10s`,
			wantErr: false,
			want: Settings{
				Type:          ServiceRatelimit,
				MaxHits:       100,
				TimeWindow:    10 * time.Second,
				Group:         "",
				CleanInterval: 10 * time.Second * 10,
			},
		},
		{
			name: "test client ratelimit",
			yml: `type: client
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: Settings{
				Type:          ClientRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test cluster client ratelimit",
			yml: `type: clusterClient
group: foo
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: Settings{
				Type:          ClusterClientRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "foo",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test cluster ratelimit",
			yml: `type: clusterService
group: foo
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: Settings{
				Type:          ClusterServiceRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "foo",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test disabled ratelimit",
			yml: `type: disabled
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: Settings{
				Type:          DisableRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test invalid type",
			yml: `type: invalid
max-hits: 50
time-window: 2m`,
			wantErr: true,
		},
		{
			name:    "test invalid yaml",
			yml:     `type=invalid`,
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var rt Settings
			if err := yaml.Unmarshal([]byte(tt.yml), &rt); err != nil && !tt.wantErr {
				t.Fatalf("Failed to unmarshal: %v", err)
			} else if tt.wantErr && err == nil {
				t.Fatal("Failed to get unmarshal error")
			}

			if got := rt.Type.String(); got != tt.want.Type.String() {
				t.Errorf("Failed to get the right ratelimit type want %s, got %s", tt.want.Type.String(), got)
			}

			if got := rt.String(); got != tt.want.String() {
				t.Errorf("Failed to get the right ratelimit want %s, got %s", tt.want.String(), got)
			}
		})
	}
}

func TestVoidRatelimit(t *testing.T) {
	l := voidRatelimit{}
	defer l.Close()
	l.Resize("s", 5) // should do nothing
	now := time.Now()
	for i := 0; i < 100; i++ {
		if l.Allow(context.Background(), "s") != true {
			t.Error("voidratelimit should always allow")
		}
		if l.RetryAfter("s") != 0 {
			t.Error("voidratelimit should always be retryable")
		}
		if d := l.Delta("s"); d >= 0 {
			t.Errorf("There was a delta found %v, but should not", d)
		}
		if ti := l.Oldest("s"); now.Before(ti) {
			t.Errorf("oldest should never have a new time: %v", ti)
		}
	}
}
func TestZeroRatelimit(t *testing.T) {
	l := zeroRatelimit{}
	defer l.Close()
	l.Resize("s", 5) // should do nothing
	now := time.Now()
	for i := 0; i < 100; i++ {
		if l.Allow(context.Background(), "s") != false {
			t.Error("zerolimit should always deny")
		}
		if l.RetryAfter("s") != zeroRetry {
			t.Error("zerolimit should always never be retryable")
		}
		if d := l.Delta("s"); d != zeroDelta {
			t.Errorf("There was a wrong delta found %v, but should %v", d, zeroDelta)
		}
		if ti := l.Oldest("s"); now.Before(ti) {
			t.Errorf("oldest should never have a new time: %v", ti)
		}
	}
}

func TestRatelimitImpl(t *testing.T) {
	settings := Settings{
		Type:          ServiceRatelimit,
		MaxHits:       100,
		TimeWindow:    10 * time.Second,
		Group:         "",
		CleanInterval: 10 * time.Second * 10,
	}

	rl := newRatelimit(settings, nil, nil, nil)
	defer rl.Close()
	rl.Resize("", 5)

	for i := 0; i < 5; i++ {
		if rl.Allow(context.Background(), "") != true {
			t.Error("service ratelimit should allow 5")
		}
	}

	if rl.Allow(context.Background(), "") == true {
		t.Error("After 5 allows we should get a deny")
	}
	if rl.RetryAfter("") == 0 {
		t.Error("After 5 allows we should get a non zero value")
	}
	if d := rl.Delta(""); d == 0 {
		t.Errorf("There was no delta found %v, but should", d)
	}

	rl = nil
	if rl.Allow(context.Background(), "") != true {
		t.Error("nil ratelimiter should always allow")
	}
	if rl.RetryAfter("") != 0 {
		t.Error("nil ratelimiter should always allow to retry")
	}
}

func TestHeaders(t *testing.T) {
	h := Headers(1, time.Hour, 5)
	t.Logf("h: %v", h)
	if s := h.Get("X-Rate-Limit"); s != "1" {
		t.Errorf("Failed to get X-Rate-Limit Header value: %s", s)
	}
	if s := h.Get("Retry-After"); s != "5" {
		t.Errorf("Failed to get Retry-After Header value: %s", s)
	}
}
