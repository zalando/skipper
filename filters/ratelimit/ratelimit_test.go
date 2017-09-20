package ratelimit

import (
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
		// t.Run("too many", testErr(s, 6, "1m", 12, "30m", 42))
		// t.Run("wrong failure count", testErr(s, "6", "1m", 12))
		// t.Run("wrong timeout", testErr(s, 6, "foo", 12))
		// t.Run("wrong half-open requests", testErr(s, 6, "1m", "foo"))
		// t.Run("only failure count", testOK(s, 6))
		// t.Run("only failure count and timeout", testOK(s, 6, "1m"))
		// t.Run("full", testOK(s, 6, "1m", 12))
		// t.Run("timeout as milliseconds", testOK(s, 6, 60000, 12))
		// t.Run("with idle ttl", testOK(s, 6, 60000, 12, "30m"))
	})

	t.Run("disable", func(t *testing.T) {
		rl := NewDisableRatelimit()
		//t.Run("with args fail", testErr(rl, 6))
		t.Run("no args, ok", testOK(rl))
	})
}

func TestRateLimit(t *testing.T) {
	test := func(
		s func() filters.Spec,
		expect ratelimit.Settings,
		args ...interface{},
	) func(*testing.T) {
		return func(t *testing.T) {
			s := s()

			f, err := s.CreateFilter(args)
			if err != nil {
				t.Error(err)
			}

			// TODO(sszuecs): do we need state bag?
			ctx := &filtertest.Context{
				FStateBag: make(map[string]interface{}),
			}

			f.Request(ctx)

			settings, ok := ctx.StateBag()[RouteSettingsKey]
			if !ok {
				t.Error("failed to set the ratelimit settings")
			}

			if settings != expect {
				t.Error("invalid settings")
				t.Log("got", settings)
				t.Log("expected", expect)
			}
		}
	}

	t.Run("ratelimit local", test(
		NewLocalRatelimit,
		ratelimit.Settings{
			Type:          ratelimit.LocalRatelimit,
			MaxHits:       3,
			TimeWindow:    1 * time.Second,
			CleanInterval: 1 * time.Minute,
		},
		3,
		"1s",
		"1m",
	))

	t.Run("ratelimit disable", test(
		NewDisableRatelimit,
		ratelimit.Settings{Type: ratelimit.DisableRatelimit},
	))
}
