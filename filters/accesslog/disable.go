package accesslog

import (
	"github.com/zalando/skipper/filters"
)

const (
	// AccessLogDisabledName is the filter name seen by the user
	AccessLogDisabledName = "accessLogDisabled"

	// AccessLogDisabledKey is the key used in the state bag to pass the access log state to the proxy.
	AccessLogDisabledKey = "statebag:access_log:proxy:disabled"
)

type accessLogDisabled struct {
	disabled bool
}

// NewAccessLogDisabled creates a filter spec for overriding the state of the AccessLogDisabled setting. (By default global setting is used.)
//
// 	accessLogDisabled("false")
func NewAccessLogDisabled() filters.Spec {
	return &accessLogDisabled{}
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
	bag[AccessLogDisabledKey] = al.disabled
}

func (*accessLogDisabled) Response(filters.FilterContext) {}
