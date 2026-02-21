package accesslog

import (
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated: use DisableAccessLogName or EnableAccessLogName
	AccessLogDisabledName = "accessLogDisabled"
)

type accessLogDisabled struct {
	disabled bool
}

// NewAccessLogDisabled creates a filter spec for overriding the state of the AccessLogDisabled setting. (By default global setting is used.)
//
//	accessLogDisabled("false")
//
// Deprecated: use disableAccessLog or enableAccessLog
func NewAccessLogDisabled() filters.Spec {
	return &accessLogDisabled{}
}

func (*accessLogDisabled) Name() string { return AccessLogDisabledName }

func (*accessLogDisabled) CreateFilter(args []any) (filters.Filter, error) {
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
	bag[AccessLogEnabledKey] = &AccessLogFilter{!al.disabled, nil, nil}
}

func (*accessLogDisabled) Response(filters.FilterContext) {}
