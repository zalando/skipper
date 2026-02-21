package accesslog

import (
	"github.com/zalando/skipper/filters"
	"maps"
)

const (
	// Deprecated, use filters.DisableAccessLogName instead
	DisableAccessLogName = filters.DisableAccessLogName

	// Deprecated, use filters.EnableAccessLogName instead
	EnableAccessLogName = filters.EnableAccessLogName

	// AccessLogEnabledKey is the key used in the state bag to pass the access log state to the proxy.
	AccessLogEnabledKey = "statebag:access_log:proxy:enabled"

	// AccessLogAdditionalDataKey is the key used in the state bag to pass extra data to access log
	AccessLogAdditionalDataKey = "statebag:access_log:additional"

	// KeyMaskedQueryParams is the key used to store and retrieve masked query parameters
	// from the additional data.
	KeyMaskedQueryParams = "maskedQueryParams"
)

// AccessLogFilter stores access log state
type AccessLogFilter struct {
	// Enable represents whether or not the access log is enabled.
	Enable bool
	// Prefixes contains the list of response code prefixes.
	Prefixes []int
	// MaskedQueryParams contains the set of query parameters (keys) that are masked/obfuscated in the access log.
	MaskedQueryParams map[string]struct{}
}

func (al *AccessLogFilter) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[AccessLogEnabledKey] = al

	if al.MaskedQueryParams != nil {
		additionalData, ok := bag[AccessLogAdditionalDataKey].(map[string]any)
		if !ok {
			additionalData = make(map[string]any)
			bag[AccessLogAdditionalDataKey] = additionalData
		}
		maskedQueryParams, ok := additionalData[KeyMaskedQueryParams].(map[string]struct{})
		if !ok {
			maskedQueryParams = make(map[string]struct{})
			additionalData[KeyMaskedQueryParams] = maskedQueryParams
		}
		maps.Copy(maskedQueryParams, al.MaskedQueryParams)

	}
}

func (*AccessLogFilter) Response(filters.FilterContext) {}

func extractFilterValues(args []any, enable bool) (filters.Filter, error) {
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

func (al *disableAccessLog) CreateFilter(args []any) (filters.Filter, error) {
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

func (al *enableAccessLog) CreateFilter(args []any) (filters.Filter, error) {
	return extractFilterValues(args, true)
}

type maskAccessLogQuery struct{}

// NewMaskAccessLogQuery creates a filter spec to mask specific query parameters from the access log for a specific route.
// Takes in query param keys as arguments. When provided, the value of these keys are masked (i.e., hashed).
//
//	maskAccessLogQuery("key_1", "key_2") to mask the value of provided keys in the access log.
func NewMaskAccessLogQuery() filters.Spec {
	return &maskAccessLogQuery{}
}

func (*maskAccessLogQuery) Name() string { return filters.MaskAccessLogQueryName }

func (al *maskAccessLogQuery) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	keys := make(map[string]struct{}, len(args))
	for _, arg := range args {
		if key, ok := arg.(string); ok && key != "" {
			keys[key] = struct{}{}
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &AccessLogFilter{Enable: true, MaskedQueryParams: keys}, nil
}
