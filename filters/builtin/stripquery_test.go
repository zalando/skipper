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
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
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

	f, _ := sqs.CreateFilter([]any{"true"})

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

func BenchmarkSanitize(b *testing.B) {
	piece := "query=cXVlcnkgUGRwKCRjb25maWdTa3U6IElEIS&variables=%7B%0A%20%20%20%20%22beautyColorImageWidth%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20200,%0A%20%20%20%20%22portraitGalleryWidth%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20828,%0A%20%20%20%20%22segmentedBannerHeaderLogoWidth%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%2084,%0A%20%20%20%20%22shouldIncludeCtaTrackingKey%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20false,%0A%20%20%20%20%22shouldIncludeFlagInfo%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20false,%0A%20%20%20%20%22shouldIncludeHistogramValues%22:%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20%20false,%0A%20%20%20%20%22shouldIncludeOfferSelectionValues%22:%20%20%20%20%20%20%20%20%20%20%20true,%0A%20%20%20%20%22shouldIncludeOmnibusConfigModeChanges%22:%20%20%20%20%20%20%20false,%0A%20%20%20%20%22shouldIncludeOmnibusPriceLabelChanges%22:%20%20%20%20%20%20%20false,&apiEndpoint=https%253A%252F%252Fmodified%252Fsecret%252Fgraphql%252Fsecret&frontendType=secret&zalandoFeature=secret"
	q := strings.Repeat(piece, 2)
	v, e := url.ParseQuery(q)

	if e != nil {
		b.Error(e)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k := range v {
			sanitize(k)
		}
	}
}
