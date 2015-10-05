package stripquery

import (
	"github.com/zalando/skipper/mock"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCreateStripQueryFilter(t *testing.T) {
	sqs := &StripQuery{}
	if sqs.Name() != "stripQuery" {
		t.Error("wrong name")
	}

	f, err := sqs.MakeFilter("id", nil)
	if err != nil || f.Id() != "id" {
		t.Error("wrong id")
	}

	req := &http.Request{}
	c := &mock.FilterContext{nil, req, nil, false, nil}
	f.Request(c)

	rsp := &http.Response{}
	c.FResponse = rsp
	f.Response(c)
}

func TestStripQuery(t *testing.T) {
	sqs := &StripQuery{}

	f, _ := sqs.MakeFilter("id", nil)

	url, _ := url.ParseRequestURI("http://example.org/foo?bar=baz")
	req := &http.Request{URL: url}

	c := &mock.FilterContext{nil, req, nil, false, nil}
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
	{"http://example.org/foo?bar=baz", map[string][]string{"x-query-param-bar": []string{"baz"}}},
	{"http://example.org/foo?bar", map[string][]string{"x-query-param-bar": []string{""}}},
	{"http://example.org/foo?bar=baz&bar=qux", map[string][]string{"x-query-param-bar": []string{"baz"}}},
	{"http://example.org/foo?a-b=123", map[string][]string{"x-query-param-a-b": []string{"123"}}},
	{"http://example.org/foo?a%20b=123", map[string][]string{"x-query-param-ab": []string{"123"}}},
	{"http://example.org/foo?馬鹿=123", map[string][]string{"x-query-param-u99acu9e7f": []string{"123"}}},
}

func TestPreserveQuery(t *testing.T) {
	sqs := &StripQuery{}

	f, _ := sqs.MakeFilter("id", []interface{}{"true"})

	for _, tt := range headerTests {
		url, _ := url.ParseRequestURI(tt.uri)
		req := &http.Request{URL: url}

		c := &mock.FilterContext{nil, req, nil, false, nil}
		f.Request(c)

		for k, h := range tt.header {

			if c.FRequest.Header.Get(k) != strings.Join(h, ",") {
				t.Errorf("Uri %q => %q, want %q (%v)", tt.uri, c.FRequest.Header.Get(k), h, c.FRequest.Header)
			}
		}
	}
}
