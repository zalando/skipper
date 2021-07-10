/*
Package circuit provides filters to control the circuit breaker settings on the route level.

For detailed documentation of the circuit breakers, see https://godoc.org/github.com/zalando/skipper/circuit.
*/
package circuit

import (
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/filters"
)

const (
	ConsecutiveBreakerName = "consecutiveBreaker"
	RateBreakerName        = "rateBreaker"
	DisableBreakerName     = "disableBreaker"
	RouteSettingsKey       = "#circuitbreakersettings"
)

type spec struct {
	typ circuit.BreakerType
}

type filter struct {
	settings circuit.BreakerSettings
}

// NewConsecutiveBreaker creates a filter specification to instantiate consecutiveBreaker() filters.
//
// These filters set a breaker for the current route that open if the backend failures for the route reach a
// value of N, where N is a mandatory argument of the filter:
//
// 	consecutiveBreaker(15)
//
// The filter accepts the following optional arguments: timeout (milliseconds or duration string),
// half-open-requests (integer), idle-ttl (milliseconds or duration string).
func NewConsecutiveBreaker() filters.Spec {
	return &spec{typ: circuit.ConsecutiveFailures}
}

// NewRateBreaker creates a filter specification to instantiate rateBreaker() filters.
//
// These filters set a breaker for the current route that open if the backend failures for the route reach a
// value of N within a window of the last M requests, where N and M are mandatory arguments of the filter:
//
// 	rateBreaker(30, 300)
//
// The filter accepts the following optional arguments: timeout (milliseconds or duration string),
// half-open-requests (integer), idle-ttl (milliseconds or duration string).
func NewRateBreaker() filters.Spec {
	return &spec{typ: circuit.FailureRate}
}

// NewDisableBreaker disables the circuit breaker for a route. It doesn't accept any arguments.
func NewDisableBreaker() filters.Spec {
	return &spec{}
}

func (s *spec) Name() string {
	switch s.typ {
	case circuit.ConsecutiveFailures:
		return ConsecutiveBreakerName
	case circuit.FailureRate:
		return RateBreakerName
	default:
		return DisableBreakerName
	}
}

func consecutiveFilter(args []interface{}) (filters.Filter, error) {
	a := filters.Args(args)
	return &filter{
		settings: circuit.BreakerSettings{
			Type:             circuit.ConsecutiveFailures,
			Failures:         a.Int(),
			Timeout:          a.DurationOr(0),
			HalfOpenRequests: a.IntOr(0),
			IdleTTL:          a.DurationOr(0),
		},
	}, a.Err()
}

func rateFilter(args []interface{}) (filters.Filter, error) {
	a := filters.Args(args)
	return &filter{
		settings: circuit.BreakerSettings{
			Type:             circuit.FailureRate,
			Failures:         a.Int(),
			Window:           a.Int(),
			Timeout:          a.DurationOr(0),
			HalfOpenRequests: a.IntOr(0),
			IdleTTL:          a.DurationOr(0),
		},
	}, a.Err()
}

func disableFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &filter{
		settings: circuit.BreakerSettings{
			Type: circuit.BreakerDisabled,
		},
	}, nil
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	switch s.typ {
	case circuit.ConsecutiveFailures:
		return consecutiveFilter(args)
	case circuit.FailureRate:
		return rateFilter(args)
	default:
		return disableFilter(args)
	}
}

func (f *filter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[RouteSettingsKey] = f.settings
}

func (f *filter) Response(filters.FilterContext) {}
