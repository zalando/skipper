/*
Package circuit provides filters to control the circuit breaker settings on the route level.

For detailed documentation of the circuit breakers, see https://pkg.go.dev/github.com/zalando/skipper/circuit.
*/
package circuit

import (
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.ConsecutiveBreakerName instead
	ConsecutiveBreakerName = filters.ConsecutiveBreakerName
	// Deprecated, use filters.RateBreakerName instead
	RateBreakerName = filters.RateBreakerName
	// Deprecated, use filters.DisableBreakerName instead
	DisableBreakerName = filters.DisableBreakerName

	RouteSettingsKey = "#circuitbreakersettings"
)

type spec struct {
	typ circuit.BreakerType
}

type filter struct {
	settings circuit.BreakerSettings
}

func getIntArg(a interface{}) (int, error) {
	if i, ok := a.(int); ok {
		return i, nil
	}

	if f, ok := a.(float64); ok {
		return int(f), nil
	}

	return 0, filters.ErrInvalidFilterParameters
}

func getDurationArg(a interface{}) (time.Duration, error) {
	if s, ok := a.(string); ok {
		return time.ParseDuration(s)
	}

	i, err := getIntArg(a)
	return time.Duration(i) * time.Millisecond, err
}

// NewConsecutiveBreaker creates a filter specification to instantiate consecutiveBreaker() filters.
//
// These filters set a breaker for the current route that open if the backend failures for the route reach a
// value of N, where N is a mandatory argument of the filter:
//
//	consecutiveBreaker(15)
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
//	rateBreaker(30, 300)
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
		return filters.ConsecutiveBreakerName
	case circuit.FailureRate:
		return filters.RateBreakerName
	default:
		return filters.DisableBreakerName
	}
}

func consecutiveFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	failures, err := getIntArg(args[0])
	if err != nil {
		return nil, err
	}

	var timeout time.Duration
	if len(args) > 1 {
		timeout, err = getDurationArg(args[1])
		if err != nil {
			return nil, err
		}
	}

	var halfOpenRequests int
	if len(args) > 2 {
		halfOpenRequests, err = getIntArg(args[2])
		if err != nil {
			return nil, err
		}
	}

	var idleTTL time.Duration
	if len(args) > 3 {
		idleTTL, err = getDurationArg(args[3])
		if err != nil {
			return nil, err
		}
	}

	return &filter{
		settings: circuit.BreakerSettings{
			Type:             circuit.ConsecutiveFailures,
			Failures:         failures,
			Timeout:          timeout,
			HalfOpenRequests: halfOpenRequests,
			IdleTTL:          idleTTL,
		},
	}, nil
}

func rateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 2 || len(args) > 5 {
		return nil, filters.ErrInvalidFilterParameters
	}

	failures, err := getIntArg(args[0])
	if err != nil {
		return nil, err
	}

	window, err := getIntArg(args[1])
	if err != nil {
		return nil, err
	}

	var timeout time.Duration
	if len(args) > 2 {
		timeout, err = getDurationArg(args[2])
		if err != nil {
			return nil, err
		}
	}

	var halfOpenRequests int
	if len(args) > 3 {
		halfOpenRequests, err = getIntArg(args[3])
		if err != nil {
			return nil, err
		}
	}

	var idleTTL time.Duration
	if len(args) > 4 {
		idleTTL, err = getDurationArg(args[4])
		if err != nil {
			return nil, err
		}
	}

	return &filter{
		settings: circuit.BreakerSettings{
			Type:             circuit.FailureRate,
			Failures:         failures,
			Window:           window,
			Timeout:          timeout,
			HalfOpenRequests: halfOpenRequests,
			IdleTTL:          idleTTL,
		},
	}, nil
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
