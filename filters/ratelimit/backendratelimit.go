package ratelimit

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

type backendRatelimit struct {
	settings ratelimit.Settings
}

// NewBackendRatelimit creates a filter Spec, whose instances
// instruct proxy to limit request rate towards a particular backend endpoint
func NewBackendRatelimit() filters.Spec { return &backendRatelimit{} }

func (*backendRatelimit) Name() string { return "backendRatelimit" }

func (*backendRatelimit) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 3 {
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

	s := ratelimit.Settings{
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      "backend." + group,
		MaxHits:    maxHits,
		TimeWindow: timeWindow,
	}

	return &backendRatelimit{settings: s}, nil
}

func (f *backendRatelimit) Request(ctx filters.FilterContext) {
	// allows overwrite
	ctx.StateBag()[filters.BackendRatelimit] = f.settings
}

func (*backendRatelimit) Response(filters.FilterContext) {}
