package circuit

import (
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/filters"
)

const (
	ConsecutiveBreakerName = "consecutiveBreaker"
	RateBreakerName        = "rateBreaker"
	DisableBreakerName     = "disableBreaker"
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

func NewConsecutiveBreaker() filters.Spec {
	return &spec{typ: circuit.ConsecutiveFailures}
}

func NewRateBreaker() filters.Spec {
	return &spec{typ: circuit.FailureRate}
}

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
	ctx.StateBag()[circuit.RouteSettingsKey] = f.settings
}

func (f *filter) Response(filters.FilterContext) {}
