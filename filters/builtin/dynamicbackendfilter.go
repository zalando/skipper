package builtin

import (
	"net/url"

	"github.com/zalando/skipper/filters"
)

type dynamicBackendFilterType int

const (
	setDynamicBackendHostFromHeader dynamicBackendFilterType = iota
	setDynamicBackendSchemeFromHeader
	setDynamicBackendUrlFromHeader
	setDynamicBackendHost
	setDynamicBackendScheme
	setDynamicBackendUrl
)

type dynamicBackendFilter struct {
	typ   dynamicBackendFilterType
	input string
}

// verifies that the filter config has one string parameter
func dynamicBackendFilterConfig(config []interface{}) (string, error) {
	if len(config) != 1 {
		return "", filters.ErrInvalidFilterParameters
	}

	input, ok := config[0].(string)
	if !ok {
		return "", filters.ErrInvalidFilterParameters
	}

	return input, nil
}

// NewSetDynamicBackendHostFromHeader returns a filter specification that is used to set dynamic backend host from a header.
// Instances expect one parameters: a header name.
// Name: "setDynamicBackendHostFromHeader".
//
// If the header exists the value is put into the `StateBag`, additionally
// `SetOutgoingHost()` is used to set the host header
func NewSetDynamicBackendHostFromHeader() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendHostFromHeader}
}

// NewSetDynamicBackendSchemeFromHeader returns a filter specification that is used to set dynamic backend scheme from a header.
// Instances expect one parameters: a header name.
// Name: "setDynamicBackendSchemeFromHeader".
//
// If the header exists the value is put into the `StateBag`
func NewSetDynamicBackendSchemeFromHeader() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendSchemeFromHeader}
}

// NewSetDynamicBackendUrlFromHeader returns a filter specification that is used to set dynamic backend url from a header.
// Instances expect one parameters: a header name.
// Name: "setDynamicBackendUrlFromHeader".
//
// If the header exists the value is put into the `StateBag`, additionally
// `SetOutgoingHost()` is used to set the host header if the header is a valid url
func NewSetDynamicBackendUrlFromHeader() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendUrlFromHeader}
}

// NewSetDynamicBackendHost returns a filter specification that is used to set dynamic backend host.
// Instances expect one parameters: a host name.
// Name: "setDynamicBackendHost".
//
// The value is put into the `StateBag`, additionally
// `SetOutgoingHost()` is used to set the host header
func NewSetDynamicBackendHost() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendHost}
}

// NewSetDynamicBackendScheme returns a filter specification that is used to set dynamic backend scheme.
// Instances expect one parameters: a scheme name.
// Name: "setDynamicBackendScheme".
//
// The value is put into the `StateBag`
func NewSetDynamicBackendScheme() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendScheme}
}

// NewSetDynamicBackendUrl returns a filter specification that is used to set dynamic backend url.
// Instances expect one parameters: a url.
// Name: "setDynamicBackendUrl".
//
// The value is put into the `StateBag`, additionally `SetOutgoingHost()`
// is used to set the host header if the input provided is a valid url
func NewSetDynamicBackendUrl() filters.Spec {
	return &dynamicBackendFilter{typ: setDynamicBackendUrl}
}

func (spec *dynamicBackendFilter) Name() string {
	switch spec.typ {
	case setDynamicBackendHostFromHeader:
		return filters.SetDynamicBackendHostFromHeader
	case setDynamicBackendSchemeFromHeader:
		return filters.SetDynamicBackendSchemeFromHeader
	case setDynamicBackendUrlFromHeader:
		return filters.SetDynamicBackendUrlFromHeader
	case setDynamicBackendHost:
		return filters.SetDynamicBackendHost
	case setDynamicBackendScheme:
		return filters.SetDynamicBackendScheme
	case setDynamicBackendUrl:
		return filters.SetDynamicBackendUrl
	default:
		panic("invalid type")
	}
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *dynamicBackendFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
	input, err := dynamicBackendFilterConfig(config)
	return &dynamicBackendFilter{typ: spec.typ, input: input}, err
}

func (f *dynamicBackendFilter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case setDynamicBackendHostFromHeader:
		header := ctx.Request().Header.Get(f.input)
		if header != "" {
			ctx.StateBag()[filters.DynamicBackendHostKey] = header
			ctx.SetOutgoingHost(header)
		}
	case setDynamicBackendSchemeFromHeader:
		header := ctx.Request().Header.Get(f.input)
		if header != "" {
			ctx.StateBag()[filters.DynamicBackendSchemeKey] = header
		}
	case setDynamicBackendUrlFromHeader:
		header := ctx.Request().Header.Get(f.input)
		if header != "" {
			ctx.StateBag()[filters.DynamicBackendURLKey] = header
			bu, err := url.ParseRequestURI(header)
			if err == nil {
				ctx.SetOutgoingHost(bu.Host)
			}
		}
	case setDynamicBackendHost:
		ctx.StateBag()[filters.DynamicBackendHostKey] = f.input
		ctx.SetOutgoingHost(f.input)
	case setDynamicBackendScheme:
		ctx.StateBag()[filters.DynamicBackendSchemeKey] = f.input
	case setDynamicBackendUrl:
		ctx.StateBag()[filters.DynamicBackendURLKey] = f.input
		bu, err := url.ParseRequestURI(f.input)
		if err == nil {
			ctx.SetOutgoingHost(bu.Host)
		}
	}
}

func (f *dynamicBackendFilter) Response(ctx filters.FilterContext) {}
