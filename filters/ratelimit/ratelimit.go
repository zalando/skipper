/*
Package ratelimit provides filters to control the rate limitter settings on the route level.

For detailed documentation of the ratelimit, see https://godoc.org/github.com/zalando/skipper/ratelimit.
*/
package ratelimit

import (
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

// RouteSettingsKey is used as key in the context state bag
const RouteSettingsKey = "#ratelimitsettings"

type spec struct {
	typ        ratelimit.Type
	filterName string
}

type filter struct {
	settings ratelimit.Settings
}

// NewLocalRatelimit creates a local measured rate limiting, that is
// only aware of itself. If you have 5 instances with 20 req/s, then
// it would allow 100 req/s to the backend.
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> localRatelimit(20, "1m")
//    -> "https://foo.backend.net";
func NewLocalRatelimit() filters.Spec {
	return &spec{typ: ratelimit.LocalRatelimit, filterName: ratelimit.LocalRatelimitName}
}

// NewDisableRatelimit disables rate limiting
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> disableRatelimit()
//    -> "https://foo.backend.net";
func NewDisableRatelimit() filters.Spec {
	return &spec{typ: ratelimit.DisableRatelimit, filterName: ratelimit.DisableRatelimitName}
}

func (s *spec) Name() string {
	return s.filterName
}

func localRatelimitFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var err error
	var maxHits int
	if len(args) > 0 {
		maxHits, err = getIntArg(args[0])
		if err != nil {
			return nil, err
		}
	}

	var timeWindow time.Duration
	if len(args) > 1 {
		timeWindow, err = getDurationArg(args[1])
		if err != nil {
			return nil, err
		}
	}

	return &filter{
		settings: ratelimit.Settings{
			Type:          ratelimit.LocalRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    timeWindow,
			CleanInterval: 10 * timeWindow,
		},
	}, nil
}

func disableFilter(args []interface{}) (filters.Filter, error) {
	return &filter{
		settings: ratelimit.Settings{
			Type: ratelimit.DisableRatelimit,
		},
	}, nil
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	switch s.typ {
	case ratelimit.LocalRatelimit:
		return localRatelimitFilter(args)
	default:
		return disableFilter(args)
	}
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
	return time.Duration(i) * time.Second, err
}

// TODO(sszuecs): think about middleware integration
func (f *filter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[RouteSettingsKey] = f.settings
}

func (f *filter) Response(filters.FilterContext) {}
