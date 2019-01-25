package ratelimit

import (
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

	t.Run("local", func(t *testing.T) {
		rl := NewLocalRatelimit()
		t.Run("missing", testErr(rl, nil))
	})

	t.Run("disable", func(t *testing.T) {
		rl := NewDisableRatelimit()
		t.Run("no args, ok", testOK(rl))
	})
}

func TestRateLimit(t *testing.T) {
	test := func(
		s func() filters.Spec,
		expect []ratelimit.Settings,
		args ...interface{},
	) func(*testing.T) {
		return func(t *testing.T) {
			s := s()

			f, err := s.CreateFilter(args)
			if err != nil {
				t.Fatalf("failed to create filter from args %v: %v", args, err)
			}
			if f == nil {
				t.Fatalf("filter is nil, args %v: %v", args, err)
			}

			ctx := &filtertest.Context{
				FStateBag: map[string]interface{}{
					RouteSettingsKey: ratelimit.Settings{},
				},
				FRequest: &http.Request{},
			}

			f.Request(ctx)

			settings, ok := ctx.StateBag()[RouteSettingsKey]
			if !ok {
				t.Error("failed to set the ratelimit settings")
			}

			if !reflect.DeepEqual(settings, expect) {
				t.Error("invalid settings")
				t.Log("got     ", settings)
				t.Log("expected", expect)
			}
		}
	}

	t.Run("ratelimit service", test(
		NewRatelimit,
		[]ratelimit.Settings{
			{
				Type:       ratelimit.ServiceRatelimit,
				MaxHits:    3,
				TimeWindow: 1 * time.Second,
				Lookuper:   ratelimit.NewSameBucketLookuper(),
			},
		},
		3,
		"1s",
	))

	t.Run("ratelimit local", test(
		NewLocalRatelimit,
		[]ratelimit.Settings{
			{
				Type:          ratelimit.LocalRatelimit,
				MaxHits:       3,
				TimeWindow:    1 * time.Second,
				CleanInterval: 10 * time.Second,
				Lookuper:      ratelimit.NewXForwardedForLookuper(),
			},
		},
		3,
		"1s",
	))

	t.Run("ratelimit disable", test(
		NewDisableRatelimit,
		[]ratelimit.Settings{{Type: ratelimit.DisableRatelimit}},
	))
}
