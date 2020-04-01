package builtin

import (
	"fmt"
	"strings"

	"github.com/zalando/skipper/filters"
)

type headerType int

const (
	setRequestHeader headerType = iota
	appendRequestHeader
	dropRequestHeader
	setResponseHeader
	appendResponseHeader
	dropResponseHeader
	setContextRequestHeader
	appendContextRequestHeader
	setContextResponseHeader
	appendContextResponseHeader
)

// common structure for requestHeader, responseHeader specifications and
// filters
type headerFilter struct {
	typ        headerType
	key, value string
}

// verifies that the filter config has two string parameters
func headerFilterConfig(typ headerType, config []interface{}) (string, string, error) {
	switch typ {
	case dropRequestHeader, dropResponseHeader:
		if len(config) != 1 {
			return "", "", filters.ErrInvalidFilterParameters
		}
	default:
		if len(config) != 2 {
			return "", "", filters.ErrInvalidFilterParameters
		}
	}

	key, ok := config[0].(string)
	if !ok {
		return "", "", filters.ErrInvalidFilterParameters
	}

	var value string
	if len(config) == 2 {
		value, ok = config[1].(string)
		if !ok {
			return "", "", filters.ErrInvalidFilterParameters
		}
	}

	return key, value, nil
}

// Returns a filter specification that is used to set headers for requests.
// Instances expect two parameters: the header name and the header value.
// Name: "setRequestHeader".
//
// If the header name is 'Host', the filter uses the `SetOutgoingHost()`
// method to set the header in addition to the standard `Request.Header`
// map.
func NewSetRequestHeader() filters.Spec {
	return &headerFilter{typ: setRequestHeader}
}

// Returns a filter specification that is used to append headers for requests.
// Instances expect two parameters: the header name and the header value.
// Name: "appendRequestHeader".
//
// If the header name is 'Host', the filter uses the `SetOutgoingHost()`
// method to set the header in addition to the standard `Request.Header`
// map.
func NewAppendRequestHeader() filters.Spec {
	return &headerFilter{typ: appendRequestHeader}
}

// Returns a filter specification that is used to delete headers for requests.
// Instances expect one parameter: the header name.
// Name: "dropRequestHeader".
func NewDropRequestHeader() filters.Spec {
	return &headerFilter{typ: dropRequestHeader}
}

// Returns a filter specification that is used to set headers for responses.
// Instances expect two parameters: the header name and the header value.
// Name: "setResponseHeader".
func NewSetResponseHeader() filters.Spec {
	return &headerFilter{typ: setResponseHeader}
}

// Returns a filter specification that is used to append headers for responses.
// Instances expect two parameters: the header name and the header value.
// Name: "appendResponseHeader".
func NewAppendResponseHeader() filters.Spec {
	return &headerFilter{typ: appendResponseHeader}
}

// Returns a filter specification that is used to delete headers for responses.
// Instances expect one parameter: the header name.
// Name: "dropResponseHeader".
func NewDropResponseHeader() filters.Spec {
	return &headerFilter{typ: dropResponseHeader}
}

// NewSetContextRequestHeader returns a filter specification used to set
// request headers with a given name and a value taken from the filter
// context state bag identified by its key.
func NewSetContextRequestHeader() filters.Spec {
	return &headerFilter{typ: setContextRequestHeader}
}

// NewAppendContextRequestHeader returns a filter specification used to append
// request headers with a given name and a value taken from the filter
// context state bag identified by its key.
func NewAppendContextRequestHeader() filters.Spec {
	return &headerFilter{typ: appendContextRequestHeader}
}

// NewSetContextResponseHeader returns a filter specification used to set
// response headers with a given name and a value taken from the filter
// context state bag identified by its key.
func NewSetContextResponseHeader() filters.Spec {
	return &headerFilter{typ: setContextResponseHeader}
}

// NewAppendContextResponseHeader returns a filter specification used to append
// response headers with a given name and a value taken from the filter
// context state bag identified by its key.
func NewAppendContextResponseHeader() filters.Spec {
	return &headerFilter{typ: appendContextResponseHeader}
}

func (spec *headerFilter) Name() string {
	switch spec.typ {
	case setRequestHeader:
		return SetRequestHeaderName
	case appendRequestHeader:
		return AppendRequestHeaderName
	case dropRequestHeader:
		return DropRequestHeaderName
	case setResponseHeader:
		return SetResponseHeaderName
	case appendResponseHeader:
		return AppendResponseHeaderName
	case dropResponseHeader:
		return DropResponseHeaderName
	case setContextRequestHeader:
		return SetContextRequestHeaderName
	case appendContextRequestHeader:
		return AppendContextRequestHeaderName
	case setContextResponseHeader:
		return SetContextResponseHeaderName
	case appendContextResponseHeader:
		return AppendContextResponseHeaderName
	default:
		panic("invalid header type")
	}
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *headerFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
	key, value, err := headerFilterConfig(spec.typ, config)
	return &headerFilter{typ: spec.typ, key: key, value: value}, err
}

func valueFromContext(
	ctx filters.FilterContext,
	headerName,
	contextKey string,
	isRequest bool,
	apply func(string, string),
) {
	contextValue, ok := ctx.StateBag()[contextKey]
	if !ok {
		return
	}

	stringValue := fmt.Sprint(contextValue)
	apply(headerName, stringValue)
	if isRequest && strings.ToLower(headerName) == "host" {
		ctx.SetOutgoingHost(stringValue)
	}
}

func (f *headerFilter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case setRequestHeader:
		ctx.Request().Header.Set(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case appendRequestHeader:
		ctx.Request().Header.Add(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case dropRequestHeader:
		ctx.Request().Header.Del(f.key)
	case setContextRequestHeader:
		valueFromContext(ctx, f.key, f.value, true, ctx.Request().Header.Set)
	case appendContextRequestHeader:
		valueFromContext(ctx, f.key, f.value, true, ctx.Request().Header.Add)
	}
}

func (f *headerFilter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case setResponseHeader:
		ctx.Response().Header.Set(f.key, f.value)
	case appendResponseHeader:
		ctx.Response().Header.Add(f.key, f.value)
	case dropResponseHeader:
		ctx.Response().Header.Del(f.key)
	case setContextResponseHeader:
		valueFromContext(ctx, f.key, f.value, false, ctx.Response().Header.Set)
	case appendContextResponseHeader:
		valueFromContext(ctx, f.key, f.value, false, ctx.Response().Header.Add)
	}
}
