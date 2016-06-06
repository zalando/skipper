// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtin

import (
	"github.com/zalando/skipper/filters"
	"strings"
)

type headerType int

const (
	setRequestHeader headerType = iota
	appendRequestHeader
	dropRequestHeader
	setResponseHeader
	appendResponseHeader
	dropResponseHeader

	depRequestHeader
	depResponseHeader
)

// common structure for requestHeader, responseHeader specifications and
// filters
type headerFilter struct {
	typ              headerType
	name, key, value string
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
	default:
		panic("invalid header type")
	}
}

func (spec *headerFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
	key, value, err := headerFilterConfig(spec.typ, config)
	return &headerFilter{typ: spec.typ, key: key, value: value}, err
}

func (f *headerFilter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case setRequestHeader:
		ctx.Request().Header.Set(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case appendRequestHeader, depRequestHeader:
		ctx.Request().Header.Add(f.key, f.value)
		if strings.ToLower(f.key) == "host" {
			ctx.SetOutgoingHost(f.value)
		}
	case dropRequestHeader:
		ctx.Request().Header.Del(f.key)
	}
}

func (f *headerFilter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case setResponseHeader:
		ctx.Response().Header.Set(f.key, f.value)
	case appendResponseHeader, depResponseHeader:
		ctx.Response().Header.Add(f.key, f.value)
	case dropResponseHeader:
		ctx.Response().Header.Del(f.key)
	}
}
