package accesslog

import "github.com/zalando/skipper/filters"

const (
	// Deprecated, use filters.DisableAccessLogName instead
	DisableAccessLogName = filters.DisableAccessLogName

	// Deprecated, use filters.EnableAccessLogName instead
	EnableAccessLogName = filters.EnableAccessLogName

	// AccessLogEnabledKey is the key used in the state bag to pass the access log state to the proxy.
	AccessLogEnabledKey = "statebag:access_log:proxy:enabled"

	// AccessLogAdditionalDataKey is the key used in the state bag to pass extra data to access log
	AccessLogAdditionalDataKey = "statebag:access_log:additional"
)

// AccessLogFilter stores access log state
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
		var intPref int
		switch p := prefix.(type) {
		case float32:
			intPref = int(p)
		case float64:
			intPref = int(p)
		default:
			var ok bool
			intPref, ok = prefix.(int)
			if !ok {
				return nil, filters.ErrInvalidFilterParameters
			}
		}

		prefixes = append(prefixes, intPref)
	}

	return &AccessLogFilter{Enable: enable, Prefixes: prefixes}, nil
}

type disableAccessLog struct{}

// NewDisableAccessLog creates a filter spec to disable access log for specific route.
// Optionally takes in response code prefixes as arguments. When provided, access log is disabled
// only if response code matches one of the arguments.
//
//	disableAccessLog() or
//	disableAccessLog(1, 20, 301)  to disable logs for 1xx, 20x and 301 codes
func NewDisableAccessLog() filters.Spec {
	return &disableAccessLog{}
}

func (*disableAccessLog) Name() string { return filters.DisableAccessLogName }

func (al *disableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return extractFilterValues(args, false)
}

type enableAccessLog struct{}

// NewEnableAccessLog creates a filter spec to enable access log for specific route
// Optionally takes in response code prefixes as arguments. When provided, access log is enabled
// only if response code matches one of the arguments.
//
//	enableAccessLog()
//	enableAccessLog(1, 20, 301)  to enable logs for 1xx, 20x and 301 codes
func NewEnableAccessLog() filters.Spec {
	return &enableAccessLog{}
}

func (*enableAccessLog) Name() string { return filters.EnableAccessLogName }

func (al *enableAccessLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	return extractFilterValues(args, true)
}
