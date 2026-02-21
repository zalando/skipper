package circuit

import (
	"testing"
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestArgs(t *testing.T) {
	test := func(s filters.Spec, fail bool, args ...any) func(*testing.T) {
		return func(t *testing.T) {
			if _, err := s.CreateFilter(args); fail && err == nil {
				t.Error("failed to fail")
			} else if !fail && err != nil {
				t.Error(err)
			}
		}
	}

	testOK := func(s filters.Spec, args ...any) func(*testing.T) { return test(s, false, args...) }
	testErr := func(s filters.Spec, args ...any) func(*testing.T) { return test(s, true, args...) }

	t.Run("consecutive", func(t *testing.T) {
		s := NewConsecutiveBreaker()
		t.Run("missing", testErr(s, nil))
		t.Run("too many", testErr(s, 6, "1m", 12, "30m", 42))
		t.Run("wrong failure count", testErr(s, "6", "1m", 12))
		t.Run("wrong timeout", testErr(s, 6, "foo", 12))
		t.Run("wrong half-open requests", testErr(s, 6, "1m", "foo"))
		t.Run("only failure count", testOK(s, 6))
		t.Run("only failure count and timeout", testOK(s, 6, "1m"))
		t.Run("full", testOK(s, 6, "1m", 12))
		t.Run("timeout as milliseconds", testOK(s, 6, 60000, 12))
		t.Run("with idle ttl", testOK(s, 6, 60000, 12, "30m"))
	})

	t.Run("rate", func(t *testing.T) {
		s := NewRateBreaker()
		t.Run("missing both", testErr(s, nil))
		t.Run("missing window", testErr(s, 30))
		t.Run("too many", testErr(s, 30, 300, "1m", 45, "30m", 42))
		t.Run("wrong failure count", testErr(s, "30", 300, "1m", 45))
		t.Run("wrong window", testErr(s, 30, "300", "1m", 45))
		t.Run("wrong timeout", testErr(s, 30, "300", "foo", 45))
		t.Run("wrong half-open requests", testErr(s, 30, "300", "1m", "foo"))
		t.Run("only failures and window", testOK(s, 30, 300))
		t.Run("only failures, window and timeout", testOK(s, 30, 300, "1m"))
		t.Run("full", testOK(s, 30, 300, "1m", 45))
		t.Run("timeout as milliseconds", testOK(s, 30, 300, 60000, 45))
		t.Run("with idle ttl", testOK(s, 30, 300, 60000, 12, "30m"))
	})

	t.Run("disable", func(t *testing.T) {
		s := NewDisableBreaker()
		t.Run("with args fail", testErr(s, 6))
		t.Run("no args, ok", testOK(s))
	})
}

func TestBreaker(t *testing.T) {
	test := func(
		s func() filters.Spec,
		expect circuit.BreakerSettings,
		args ...any,
	) func(*testing.T) {
		return func(t *testing.T) {
			s := s()

			f, err := s.CreateFilter(args)
			if err != nil {
				t.Error(err)
			}

			ctx := &filtertest.Context{
				FStateBag: make(map[string]any),
			}

			f.Request(ctx)

			settings, ok := ctx.StateBag()[RouteSettingsKey]
			if !ok {
				t.Error("failed to set the breaker settings")
			}

			if settings != expect {
				t.Error("invalid settings")
				t.Log("got", settings)
				t.Log("expected", expect)
			}
		}
	}

	t.Run("consecutive breaker", test(
		NewConsecutiveBreaker,
		circuit.BreakerSettings{
			Type:             circuit.ConsecutiveFailures,
			Failures:         6,
			Timeout:          time.Minute,
			HalfOpenRequests: 12,
		},
		6,
		"1m",
		12,
	))

	t.Run("rate breaker", test(
		NewRateBreaker,
		circuit.BreakerSettings{
			Type:             circuit.FailureRate,
			Failures:         30,
			Window:           300,
			Timeout:          time.Minute,
			HalfOpenRequests: 12,
		},
		30,
		300,
		"1m",
		12,
	))

	t.Run("disable breaker", test(
		NewDisableBreaker,
		circuit.BreakerSettings{
			Type: circuit.BreakerDisabled,
		},
	))
}
