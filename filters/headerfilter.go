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

package filters

import (
	"errors"
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
	typ              headerType
	name, key, value string
}

// verifies that the filter config has two string arguments
func headerFilterConfig(config []interface{}) (string, string, error) {
	if len(config) != 2 {
		return "", "", errors.New("invalid number of args, expecting 2")
	}

	key, ok := config[0].(string)
	if !ok {
		return "", "", errors.New("invalid header key, expecting string")
	}

	value, ok := config[1].(string)
	if !ok {
		return "", "", errors.New("invalid header value, expecting string")
	}

	return key, value, nil
}

// Creates a filter specification that is used to set headers for requests.
// Instances expect two arguments: the header name and the header value.
// Name: "requestHeader"
func NewRequestHeaderSpec() Spec {
	return &headerFilter{typ: requestHeader, name: RequestHeaderName}
}

// Creates a filter specification that is used to set headers for responses.
// Instances expect two arguments: the header name and the header value.
// Name: "responseHeader"
func NewResponseHeaderSpec() Spec {
	return &headerFilter{typ: responseHeader, name: ResponseHeaderName}
}

func (spec *headerFilter) Name() string { return spec.name }

func (spec *headerFilter) CreateFilter(config []interface{}) (Filter, error) {
	key, value, err := headerFilterConfig(config)
	return &headerFilter{typ: spec.typ, key: key, value: value}, err
}

func (f *headerFilter) Request(ctx FilterContext) {
	if f.typ == requestHeader {
		req := ctx.Request()
		if strings.ToLower(f.key) == "host" {
			req.Host = f.value
		}

		req.Header.Add(f.key, f.value)
	}
}

func (f *headerFilter) Response(ctx FilterContext) {
	if f.typ == responseHeader {
		ctx.Response().Header.Add(f.key, f.value)
	}
}
