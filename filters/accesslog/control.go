package accesslog

import "github.com/zalando/skipper/filters"

const (
	// DisableAccessLogName is the filter name seen by the user
	DisableAccessLogName = "disableAccessLog"

	// EnableAccessLogName is the filter name seen by the user
	EnableAccessLogName = "enableAccessLog"

	// AccessLogEnabledKey is the key used in the state bag to pass the access log state to the proxy.
	AccessLogEnabledKey = "statebag:access_log:proxy:enabled"
)

type disableAccessLog struct{}

// NewDisableAccessLog creates a filter spec to disable access log for specific route
//
// 	disableAccessLog()
func NewDisableAccessLog() filters.Spec {
	return &disableAccessLog{}
}

func (*disableAccessLog) Name() string { return DisableAccessLogName }

func (al *disableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return al, nil
}

func (al *disableAccessLog) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = false
}

func (*disableAccessLog) Response(filters.FilterContext) {}

type enableAccessLog struct{}

// NewEnableAccessLog creates a filter spec to enable access log for specific route
//
// 	enableAccessLog()
func NewEnableAccessLog() filters.Spec {
	return &enableAccessLog{}
}

func (*enableAccessLog) Name() string { return EnableAccessLogName }

func (al *enableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return al, nil
}

func (al *enableAccessLog) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = true
}

func (*enableAccessLog) Response(filters.FilterContext) {}
