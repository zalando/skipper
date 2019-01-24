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

// Common filter struct for holding access log state
type AccessLogFilter struct {
	Enable   bool
	Prefixes []int
}

func (al *AccessLogFilter) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = al
}

func (*AccessLogFilter) Response(filters.FilterContext) {}

func extractFilterValues(args []interface{}, enable bool) (filters.Filter, error) {
	prefixes := make([]int, 0)
	for _, prefix := range args {
		intPref, ok := 0, false
		switch prefix.(type) {
		case float32:
			intPref, ok = int(prefix.(float32)), true
		case float64:
			intPref, ok = int(prefix.(float64)), true
		default:
			intPref, ok = prefix.(int)
		}
		if ok {
			prefixes = append(prefixes, intPref)
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	return &AccessLogFilter{Enable: enable, Prefixes: prefixes}, nil
}

type disableAccessLog struct{}

// NewDisableAccessLog creates a filter spec to disable access log for specific route.
// Optionally takes in response code prefixes as arguments. When provided, access log is disabled
// only if response code matches one of the arguments.
//
//  	disableAccessLog() or
//  	disableAccessLog(1, 20, 301)  to disable logs for 1xx, 20x and 301 codes
func NewDisableAccessLog() filters.Spec {
	return &disableAccessLog{}
}

func (*disableAccessLog) Name() string { return DisableAccessLogName }

func (al *disableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return extractFilterValues(args, false)
}

type enableAccessLog struct{}

// NewEnableAccessLog creates a filter spec to enable access log for specific route
// Optionally takes in response code prefixes as arguments. When provided, access log is enabled
// only if response code matches one of the arguments.
//
// 	enableAccessLog()
// 	enableAccessLog(1, 20, 301)  to enable logs for 1xx, 20x and 301 codes
func NewEnableAccessLog() filters.Spec {
	return &enableAccessLog{}
}

func (*enableAccessLog) Name() string { return EnableAccessLogName }

func (al *enableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return extractFilterValues(args, true)
}
