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
	requestHeader headerType = iota
	responseHeader
)

// common structure for requestHeader, responseHeader specifications and
// filters
type headerFilter struct {
	typ       headerType
	append    bool
	name, key string
	value     *filters.ParamTemplate
}

// verifies that the filter config has two string parameters
func headerFilterConfig(config []interface{}) (string, *filters.ParamTemplate, error) {
	if len(config) != 2 {
		return "", nil, filters.ErrInvalidFilterParameters
	}

	key, ok := config[0].(string)
	if !ok {
		return "", nil, filters.ErrInvalidFilterParameters
	}

	value, ok := config[1].(string)
	if !ok {
		return "", nil, filters.ErrInvalidFilterParameters
	}

	t, err := filters.NewParamTemplate(value)
	return key, t, err
}

// deprecated:
func NewRequestHeader() filters.Spec {
	s := NewAppendRequestHeader()
	s.(*headerFilter).name = RequestHeaderName
	return s
}

// deprecated:
func NewResponseHeader() filters.Spec {
	s := NewAppendResponseHeader()
	s.(*headerFilter).name = ResponseHeaderName
	return s
}

// Returns a filter specification that is used to set headers for requests.
// Instances expect two parameters: the header name and the header value.
// Name: "requestHeader".
//
// If the header name is 'Host', the filter uses the `SetOutgoingHost()`
// method to set the header in addition to the standard `Request.Header`
// map.
func NewSetRequestHeader() filters.Spec {
	return &headerFilter{
		typ:    requestHeader,
		append: false,
		name:   SetRequestHeaderName}
}

// Returns a filter specification that is used to append headers for requests.
// Instances expect two parameters: the header name and the header value.
// Name: "requestHeader".
//
// If the header name is 'Host', the filter uses the `SetOutgoingHost()`
// method to set the header in addition to the standard `Request.Header`
// map.
func NewAppendRequestHeader() filters.Spec {
	return &headerFilter{
		typ:    requestHeader,
		append: true,
		name:   AppendRequestHeaderName}
}

// Returns a filter specification that is used to set headers for responses.
// Instances expect two parameters: the header name and the header value.
// Name: "responseHeader".
func NewSetResponseHeader() filters.Spec {
	return &headerFilter{
		typ:    responseHeader,
		append: false,
		name:   SetResponseHeaderName}
}

// Returns a filter specification that is used to append headers for responses.
// Instances expect two parameters: the header name and the header value.
// Name: "responseHeader".
func NewAppendResponseHeader() filters.Spec {
	return &headerFilter{
		typ:    responseHeader,
		append: true,
		name:   AppendResponseHeaderName}
}

func (spec *headerFilter) Name() string { return spec.name }

func (spec *headerFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
	key, value, err := headerFilterConfig(config)
	return &headerFilter{typ: spec.typ, key: key, value: value}, err
}

func (f *headerFilter) Request(ctx filters.FilterContext) {
	if f.typ != requestHeader {
		return
	}

	v, ok := f.value.ExecuteLogged(ctx.PathParams())
	if !ok {
		return
	}

	sv := string(v)

	if f.append {
		ctx.Request().Header.Add(f.key, sv)
	} else {
		ctx.Request().Header.Set(f.key, sv)
	}

	if strings.ToLower(f.key) == "host" {
		ctx.SetOutgoingHost(sv)
	}
}

func (f *headerFilter) Response(ctx filters.FilterContext) {
	if f.typ != responseHeader {
		return
	}

	v, ok := f.value.ExecuteLogged(ctx.PathParams())
	if !ok {
		return
	}

	if f.append {
		ctx.Response().Header.Add(f.key, string(v))
	} else {
		ctx.Response().Header.Set(f.key, string(v))
	}
}
