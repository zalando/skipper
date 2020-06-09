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
	copyRequestHeader
	copyResponseHeader
	copyRequestHeaderDeprecated
	copyResponseHeaderDeprecated

	depRequestHeader
	depResponseHeader
)

const (
	copyRequestHeaderDeprecatedName  = "requestCopyHeader"
	copyResponseHeaderDeprecatedName = "responseCopyHeader"
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

// Deprecated: use setRequestHeader or appendRequestHeader
func NewRequestHeader() filters.Spec {
	return &headerFilter{typ: depRequestHeader}
}

// Deprecated: use setRequestHeader or appendRequestHeader
func NewResponseHeader() filters.Spec {
	return &headerFilter{typ: depResponseHeader}
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

// NewCopyRequestHeader creates a filter specification whose instances
// copies a specified source Header to a defined destination Header
// from the request to the proxy request.
func NewCopyRequestHeader() filters.Spec {
	return &headerFilter{typ: copyRequestHeader}
}

// NewCopyResponseHeader creates a filter specification whose instances
// copies a specified source Header to a defined destination Header
// from the backend response to the proxy response.
func NewCopyResponseHeader() filters.Spec {
	return &headerFilter{typ: copyResponseHeader}
}

func NewCopyRequestHeaderDeprecated() filters.Spec {
	return &headerFilter{typ: copyRequestHeaderDeprecated}
}

func NewCopyResponseHeaderDeprecated() filters.Spec {
	return &headerFilter{typ: copyResponseHeaderDeprecated}
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
	case depRequestHeader:
		return RequestHeaderName
	case depResponseHeader:
		return ResponseHeaderName
	case setContextRequestHeader:
		return SetContextRequestHeaderName
	case appendContextRequestHeader:
		return AppendContextRequestHeaderName
	case setContextResponseHeader:
		return SetContextResponseHeaderName
	case appendContextResponseHeader:
		return AppendContextResponseHeaderName
	case copyRequestHeader:
		return CopyRequestHeaderName
	case copyResponseHeader:
		return CopyResponseHeaderName
	case copyRequestHeaderDeprecated:
		return copyRequestHeaderDeprecatedName
	case copyResponseHeaderDeprecated:
		return copyResponseHeaderDeprecatedName
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
	header := ctx.Request().Header
	switch f.typ {
	case setRequestHeader:
		header.Set(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case appendRequestHeader, depRequestHeader:
		header.Add(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case dropRequestHeader:
		header.Del(f.key)
	case setContextRequestHeader:
		valueFromContext(ctx, f.key, f.value, true, header.Set)
	case appendContextRequestHeader:
		valueFromContext(ctx, f.key, f.value, true, header.Add)
	case copyRequestHeader, copyRequestHeaderDeprecated:
		if _, ok := header[f.key]; ok {
			headerValue := header.Get(f.key)
			header.Set(f.value, headerValue)
			if strings.ToLower(f.value) == "host" {
				ctx.SetOutgoingHost(headerValue)
			}
		}
	}
}

func (f *headerFilter) Response(ctx filters.FilterContext) {
	header := ctx.Response().Header
	switch f.typ {
	case setResponseHeader:
		header.Set(f.key, f.value)
	case appendResponseHeader, depResponseHeader:
		header.Add(f.key, f.value)
	case dropResponseHeader:
		header.Del(f.key)
	case setContextResponseHeader:
		valueFromContext(ctx, f.key, f.value, false, header.Set)
	case appendContextResponseHeader:
		valueFromContext(ctx, f.key, f.value, false, header.Add)
	case copyResponseHeader, copyResponseHeaderDeprecated:
		if _, ok := header[f.key]; ok {
			header.Set(f.value, header.Get(f.key))
		}
	}
}
