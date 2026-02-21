package ratelimit

import (
	"net/http"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

type BackendRatelimit struct {
	Settings   ratelimit.Settings
	StatusCode int
}

// NewBackendRatelimit creates a filter Spec, whose instances
// instruct proxy to limit request rate towards a particular backend endpoint
func NewBackendRatelimit() filters.Spec { return &BackendRatelimit{} }

func (*BackendRatelimit) Name() string {
	return filters.BackendRateLimitName
}

func (*BackendRatelimit) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 3 && len(args) != 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	group, err := getStringArg(args[0])
	if err != nil {
		return nil, err
	}

	maxHits, err := getIntArg(args[1])
	if err != nil {
		return nil, err
	}

	timeWindow, err := getDurationArg(args[2])
	if err != nil {
		return nil, err
	}

	f := &BackendRatelimit{
		Settings: ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			Group:      "backend." + group,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
		},
		StatusCode: http.StatusServiceUnavailable,
	}

	if len(args) == 4 {
		code, err := getIntArg(args[3])
		if err != nil {
			return nil, err
		}
		f.StatusCode = code
	}
	return f, nil
}

func (limit *BackendRatelimit) Request(ctx filters.FilterContext) {
	// allows overwrite
	ctx.StateBag()[filters.BackendRatelimit] = limit
}

func (*BackendRatelimit) Response(filters.FilterContext) {}
