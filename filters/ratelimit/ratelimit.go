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
// it would allow 100 req/s to the backend. A third argument can be
// used to set which part of the request should be used to find the
// same user. Third argument defaults to XForwardedForLookuper,
// meaning X-Forwarded-For Header.
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> localRatelimit(20, "1m")
//    -> "https://foo.backend.net";
//
// Example rate limit per Authorization Header:
//
//    login: Path("/login")
//    -> localRatelimit(3, "1m", "auth")
//    -> "https://login.backend.net";
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
	if !(len(args) == 2 || len(args) == 3) {
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

	var lookuper ratelimit.Lookuper
	if len(args) > 2 {
		lookuperName, err := getStringArg(args[2])
		if err != nil {
			return nil, err
		}
		switch lookuperName {
		case "auth":
			lookuper = ratelimit.NewAuthLookuper()
		default:
			lookuper = ratelimit.NewXForwardedForLookuper()
		}
	} else {
		lookuper = ratelimit.NewXForwardedForLookuper()
	}

	return &filter{
		settings: ratelimit.Settings{
			Type:          ratelimit.LocalRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    timeWindow,
			CleanInterval: 10 * timeWindow,
			Lookuper:      lookuper,
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

func getStringArg(a interface{}) (string, error) {
	if s, ok := a.(string); ok {
		return s, nil
	}

	return "", filters.ErrInvalidFilterParameters
}

func getDurationArg(a interface{}) (time.Duration, error) {
	if s, ok := a.(string); ok {
		return time.ParseDuration(s)
	}

	i, err := getIntArg(a)
	return time.Duration(i) * time.Second, err
}

// Request stores the configured ratelimit.Settings in the state bag,
// such that it can be used in the proxy to check ratelimit.
func (f *filter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[RouteSettingsKey] = f.settings
}

func (f *filter) Response(filters.FilterContext) {}
