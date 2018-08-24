/*
Package accesslog provides a request filter that gives ability to override global access log setting.
*/
package accesslog

import (
	"github.com/zalando/skipper/filters"
)

const (
	// AccessLogName is the filter name seen by the user
	AccessLogName = "accessLog"

	// AccessLogEnabledKey is the key used in the state bag to pass the access log state to the proxy.
	AccessLogEnabledKey = "statebag:access_log:proxy:enabled"
)

type accessLog struct {
	status bool
}

// NewAccessLog creates a filter spec for overriding the state of the access log. (By default global setting is used.)
//
// 	accessLog("false")
//
// EXPERIMENTAL: this filter is experimental, and the name and the arguments can change until marked as stable.
func NewAccessLog() filters.Spec {
	return &accessLog{}
}

func (al *accessLog) Name() string { return AccessLogName }

func (al *accessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if a, ok := args[0].(string); ok && a == "true" || a == "false" {
		return &accessLog{a == "true"}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (al *accessLog) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = al.status
}

func (al *accessLog) Response(_ filters.FilterContext) {}
