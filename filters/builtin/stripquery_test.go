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
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCreateStripQueryFilter(t *testing.T) {
	sqs := NewStripQuery()
	if sqs.Name() != "stripQuery" {
		t.Error("wrong name")
	}

	f, err := sqs.CreateFilter(nil)
	if err != nil {
		t.Error("wrong id")
	}

	req := &http.Request{}
	c := &filtertest.Context{FRequest: req}
	f.Request(c)

	rsp := &http.Response{}
	c.FResponse = rsp
	f.Response(c)
}

func TestStripQuery(t *testing.T) {
	sqs := NewStripQuery()

	f, _ := sqs.CreateFilter(nil)

	url, _ := url.ParseRequestURI("http://example.org/foo?bar=baz")
	req := &http.Request{URL: url}

	c := &filtertest.Context{FRequest: req}
	f.Request(c)

	q := c.FRequest.URL.Query()
	if len(q) > 0 {
		t.Error("query not removed")
	}
}

var headerTests = []struct {
	uri    string
	header http.Header
}{
	{"http://example.org/foo?bar=baz", map[string][]string{"x-query-param-bar": {"baz"}}},
	{"http://example.org/foo?bar", map[string][]string{"x-query-param-bar": {""}}},
	{"http://example.org/foo?bar=baz&bar=qux", map[string][]string{"x-query-param-bar": {"baz"}}},
	{"http://example.org/foo?a-b=123", map[string][]string{"x-query-param-a-b": {"123"}}},
	{"http://example.org/foo?a%20b=123", map[string][]string{"x-query-param-ab": {"123"}}},
	{"http://example.org/foo?馬鹿=123", map[string][]string{"x-query-param-u99acu9e7f": {"123"}}},
}

func TestPreserveQuery(t *testing.T) {
	sqs := NewStripQuery()

	f, _ := sqs.CreateFilter([]interface{}{"true"})

	for _, tt := range headerTests {
		url, _ := url.ParseRequestURI(tt.uri)
		req := &http.Request{URL: url}

		c := &filtertest.Context{FRequest: req}
		f.Request(c)

		for k, h := range tt.header {

			if c.FRequest.Header.Get(k) != strings.Join(h, ",") {
				t.Errorf("Uri %q => %q, want %q (%v)", tt.uri, c.FRequest.Header.Get(k), h, c.FRequest.Header)
			}
		}
	}
}
