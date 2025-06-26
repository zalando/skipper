package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/ratelimit"
)

func TestArgs(t *testing.T) {
	test := func(s filters.Spec, fail bool, args ...interface{}) func(*testing.T) {
		return func(t *testing.T) {
			if _, err := s.CreateFilter(args); fail && err == nil {
				t.Error("failed to create filter")
			} else if !fail && err != nil {
				t.Error(err)
			}
		}
	}

	testOK := func(s filters.Spec, args ...interface{}) func(*testing.T) { return test(s, false, args...) }
	testErr := func(s filters.Spec, args ...interface{}) func(*testing.T) { return test(s, true, args...) }

	var provider RatelimitProvider = nil

	t.Run("local", func(t *testing.T) {
		rl := NewLocalRatelimit(provider)
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("service", func(t *testing.T) {
		rl := NewRatelimit(provider)
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("client", func(t *testing.T) {
		rl := NewClientRatelimit(provider)
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("cluster", func(t *testing.T) {
		rl := NewClusterRateLimit(provider)
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("clusterClient", func(t *testing.T) {
		rl := NewClusterClientRateLimit(provider)
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("disable", func(t *testing.T) {
		rl := NewDisableRatelimit(provider)
		t.Run("no args, ok", testOK(rl))
	})
}

type testLimit struct {
	t        *testing.T
	expected ratelimit.Settings
}

func (l *testLimit) get(s ratelimit.Settings) limit {
	if !reflect.DeepEqual(s, l.expected) {
		l.t.Error("settings mismatch")
		l.t.Log("got     ", s)
		l.t.Log("expected", l.expected)
		return nil
	}
	if s.Type == ratelimit.DisableRatelimit || s.Type == ratelimit.NoRatelimit {
		return nil
	}
	return l
}

func (l *testLimit) Allow(context.Context, string) bool { return false }
func (l *testLimit) RetryAfter(string) int              { return 31415 }

func TestRateLimit(t *testing.T) {
	test := func(
		s func(RatelimitProvider) filters.Spec,
		expectedSettings ratelimit.Settings,
		expectedResponse *http.Response,
		args ...interface{},
	) func(*testing.T) {
		return func(t *testing.T) {
			s := s(&testLimit{t, expectedSettings})

			f, err := s.CreateFilter(args)
			if err != nil {
				t.Fatalf("failed to create filter from args %v: %v", args, err)
			}
			if f == nil {
				t.Fatalf("filter is nil, args %v: %v", args, err)
			}

			ctx := &filtertest.Context{
				FRequest: &http.Request{
					Header: http.Header{
						"Authorization":   []string{"foo"},
						"X-Forwarded-For": []string{"127.0.0.3"},
					},
				},
			}

			f.Request(ctx)

			if !reflect.DeepEqual(ctx.FResponse, expectedResponse) {
				t.Error("response mismatch")
				t.Log("got     ", ctx.FResponse)
				t.Log("expected", expectedResponse)
			}
		}
	}

	t.Run("ratelimit service", test(
		NewRatelimit,
		ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    3,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3,
		"1s",
	))

	t.Run("ratelimit service with float", test(
		NewRatelimit,
		ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    3,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3.3,
		"1s",
	))

	t.Run("ratelimit service with response status code", test(
		NewRatelimit,
		ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    3,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		&http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3.3,
		"1s",
		503,
	))

	t.Run("ratelimit local", test(
		NewLocalRatelimit,
		ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       2,
			TimeWindow:    2 * time.Hour,
			CleanInterval: 20 * time.Hour,
			Lookuper:      ratelimit.NewXForwardedForLookuper(),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"2"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"1"},
				"Retry-After":     []string{"31415"},
			},
		},
		2,
		"2h",
	))

	t.Run("ratelimit client", test(
		NewClientRatelimit,
		ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper:      ratelimit.NewXForwardedForLookuper(),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3,
		"1s",
	))

	t.Run("ratelimit client tuple", test(
		NewClientRatelimit,
		ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper: ratelimit.NewTupleLookuper(
				ratelimit.NewHeaderLookuper("Authorization"),
				ratelimit.NewXForwardedForLookuper()),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3,
		"1s",
		"Authorization,X-Forwarded-For",
	))
	t.Run("ratelimit client header", test(
		NewClientRatelimit,
		ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper:      ratelimit.NewHeaderLookuper("Authorization"),
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		3,
		"1s",
		"Authorization",
	))

	t.Run("ratelimit cluster", test(
		NewClusterRateLimit,
		ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			MaxHits:    3,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
			Group:      "mygroup",
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
	))

	t.Run("ratelimit cluster with response status code", test(
		NewClusterRateLimit,
		ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			MaxHits:    3,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
			Group:      "mygroup",
		},
		&http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
		503,
	))

	t.Run("sharded cluster ratelimit", test(
		func(p RatelimitProvider) filters.Spec {
			return NewShardedClusterRateLimit(p, 3)
		},
		ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			MaxHits:    1,
			TimeWindow: 1 * time.Second,
			Lookuper:   ratelimit.NewRoundRobinLookuper(3),
			Group:      "mygroup.3",
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
	))

	t.Run("ratelimit clusterClient", test(
		NewClusterClientRateLimit,
		ratelimit.Settings{
			Type:          ratelimit.ClusterClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper:      ratelimit.NewXForwardedForLookuper(),
			Group:         "mygroup",
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
	))

	t.Run("ratelimit clusterClient tuple", test(
		NewClusterClientRateLimit,
		ratelimit.Settings{
			Type:          ratelimit.ClusterClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper: ratelimit.NewTupleLookuper(
				ratelimit.NewHeaderLookuper("Authorization"),
				ratelimit.NewXForwardedForLookuper()),
			Group: "mygroup",
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
		"Authorization,X-Forwarded-For",
	))

	t.Run("ratelimit clusterClient header", test(
		NewClusterClientRateLimit,
		ratelimit.Settings{
			Type:          ratelimit.ClusterClientRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 10 * time.Second,
			Lookuper:      ratelimit.NewHeaderLookuper("Authorization"),
			Group:         "mygroup",
		},
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Ratelimit-Limit": {"3"},
				"Ratelimit-Reset": {"31415"},
				"X-Rate-Limit":    []string{"10800"},
				"Retry-After":     []string{"31415"},
			},
		},
		"mygroup",
		3,
		"1s",
		"Authorization",
	))

	t.Run("ratelimit disable", test(
		NewDisableRatelimit,
		ratelimit.Settings{Type: ratelimit.DisableRatelimit},
		nil,
	))
}

type noLimit struct {
	nilLimit bool
}

func (n *noLimit) get(ratelimit.Settings) limit {
	if n.nilLimit {
		return nil
	}
	return n
}
func (n *noLimit) Allow(context.Context, string) bool { return true }
func (n *noLimit) RetryAfter(string) int              { panic("unexpected RetryAfter call") }

func TestNilLimit(t *testing.T) {
	f := &filter{provider: &noLimit{nilLimit: true}}
	ctx := &filtertest.Context{FRequest: &http.Request{}}

	f.Request(ctx)

	if ctx.FResponse != nil {
		t.Errorf("unexpected response: %v", ctx.FResponse)
	}
}

func TestNilSettingsLookuper(t *testing.T) {
	f := &filter{settings: ratelimit.Settings{Lookuper: nil}, provider: &noLimit{}}
	ctx := &filtertest.Context{FRequest: &http.Request{}}

	f.Request(ctx)

	if ctx.FResponse != nil {
		t.Errorf("unexpected response: %v", ctx.FResponse)
	}
}

type lookuper struct {
	s string
}

func (l *lookuper) Lookup(*http.Request) string { return l.s }

func TestLookuperNoData(t *testing.T) {
	f := &filter{settings: ratelimit.Settings{Lookuper: &lookuper{""}}, provider: &noLimit{}}
	ctx := &filtertest.Context{FRequest: &http.Request{}}

	f.Request(ctx)

	if ctx.FResponse != nil {
		t.Errorf("unexpected response: %v", ctx.FResponse)
	}
}

func TestAllowsContext(t *testing.T) {
	f := &filter{settings: ratelimit.Settings{Lookuper: &lookuper{"key"}}, provider: &noLimit{}}
	ctx := &filtertest.Context{FRequest: &http.Request{}}

	f.Request(ctx)

	if ctx.FResponse != nil {
		t.Errorf("unexpected response: %v", ctx.FResponse)
	}
}

func TestGetKeyShards(t *testing.T) {
	for _, tc := range []struct {
		maxHits      int
		maxKeyShards int
		want         int
	}{
		{1, 0, 1},
		{1, 1, 1},
		{100, 1, 1},
		{4, 5, 4},
		{5, 5, 5},
		{6, 5, 3},
		{11, 10, 1}, // maxHits is prime
		{12, 10, 6},
		{101, 10, 1},
		{20, 100, 20},
		{99, 100, 99},
		{1000, 100, 100},
		{1009, 100, 1}, // maxHits is prime
	} {
		t.Run(fmt.Sprintf("maxHits=%d, maxKeyShards=%d", tc.maxHits, tc.maxKeyShards), func(t *testing.T) {
			if got := getKeyShards(tc.maxHits, tc.maxKeyShards); got != tc.want {
				t.Errorf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
