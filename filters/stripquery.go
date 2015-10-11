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

// filter to strip query parameters from the request and optionally transpose them to request headers
package filters

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

const StripQueryName = "stripQuery"

type StripQuery struct {
	// preserves the query parameter in the form of x-query-param-<queryParamName>: <queryParamValue> headers
	// ?foo=bar becomes x-query-param-foo: bar
	preserveAsHeader bool
}

func (spec *StripQuery) Name() string { return StripQueryName }

// copied from textproto/reader
func validHeaderFieldByte(b byte) bool {
	return ('A' <= b && b <= 'Z') ||
		('a' <= b && b <= 'z') ||
		('0' <= b && b <= '9') ||
		b == '-'
}

// make sure we don't generate invalid headers
func sanitize(input string) string {
	toAscii := strconv.QuoteToASCII(input)
	var b bytes.Buffer
	for _, i := range toAscii {
		if validHeaderFieldByte(byte(i)) {
			b.WriteRune(i)
		}
	}
	return b.String()
}

func (f *StripQuery) Request(ctx FilterContext) {
	r := ctx.Request()
	if r == nil {
		return
	}

	url := r.URL
	if url == nil {
		return
	}

	if !f.preserveAsHeader {
		url.RawQuery = ""
		return
	}

	q := url.Query()
	for k, vv := range q {
		for _, v := range vv {
			if r.Header == nil {
				r.Header = http.Header{}
			}
			r.Header.Add(fmt.Sprintf("X-Query-Param-%s", sanitize(k)), v)
		}
	}

	url.RawQuery = ""
}

func (f *StripQuery) Response(ctx FilterContext) {}

func (mw *StripQuery) CreateFilter(config []interface{}) (Filter, error) {
	var preserveAsHeader = false
	if len(config) == 1 {
		preserveAsHeaderString, ok := config[0].(string)
		if !ok {
			return nil, errors.New("invalid config type, expecting string")
		}
		if preserveAsHeaderString == "true" {
			preserveAsHeader = true
		}
	}
	return &StripQuery{preserveAsHeader}, nil
}
