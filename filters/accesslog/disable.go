package accesslog

import (
	"github.com/zalando/skipper/filters"
)

const (
	// AccessLogDisabledName is the filter name seen by the user
	AccessLogDisabledName = "accessLogDisabled"
)

type accessLogDisabled struct {
	disabled bool
}

func (*accessLogDisabled) Name() string { return AccessLogDisabledName }

func (*accessLogDisabled) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if a, ok := args[0].(string); ok && a == "true" || a == "false" {
		return &accessLogDisabled{a == "true"}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (al *accessLogDisabled) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = &AccessLogFilter{!al.disabled, nil}
}

func (*accessLogDisabled) Response(filters.FilterContext) {}
