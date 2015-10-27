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
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRedirect(t *testing.T) {
	spec := NewRedirect()
	f, err := spec.CreateFilter([]interface{}{float64(http.StatusFound), "https://example.org"})
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FResponseWriter: httptest.NewRecorder(), FRequest: &http.Request{URL: &url.URL{}}}
	f.Response(ctx)

	if ctx.FResponseWriter.(*httptest.ResponseRecorder).Code != http.StatusFound {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != "https://example.org" {
		t.Error("invalid location")
	}
}

func TestRedirectRelative(t *testing.T) {
	spec := NewRedirect()
	f, err := spec.CreateFilter([]interface{}{float64(http.StatusFound), "/relative/url"})
	if err != nil {
		t.Error(err)
	}

	request, _ := http.NewRequest("GET", "https://example.org/some/url", nil)

	ctx := &filtertest.Context{
		FResponseWriter: httptest.NewRecorder(),
		FRequest:        request}
	f.Response(ctx)

	if ctx.FResponseWriter.(*httptest.ResponseRecorder).Code != http.StatusFound {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != "https://example.org/relative/url" {
		t.Error("invalid location")
	}
}

func testLocation(t *testing.T, filterLocation, checkLocation string) {
	spec := NewRedirect()
	f, err := spec.CreateFilter([]interface{}{float64(http.StatusFound), filterLocation})
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{
		FResponseWriter: httptest.NewRecorder(),
		FRequest: &http.Request{
			URL:  &url.URL{Path: "/some/path", RawQuery: "foo=1&bar=2"},
			Host: "incoming.example.org"}}
	f.Response(ctx)

	if ctx.ResponseWriter().(*httptest.ResponseRecorder).Code != http.StatusFound {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != checkLocation {
		t.Error("invalid location", ctx.FResponseWriter.Header().Get("Location"))
	}
}

func TestSchemeOnly(t *testing.T) {
	testLocation(t,
		"http:",
		"http://incoming.example.org/some/path?foo=1&bar=2")

}

func TestSchemeAndHost(t *testing.T) {
	testLocation(t,
		"http://redirect.example.org",
		"http://redirect.example.org/some/path?foo=1&bar=2")
}

func TestSchemeAndHostAndPath(t *testing.T) {
	testLocation(t,
		"http://redirect.example.org/some/other/path",
		"http://redirect.example.org/some/other/path?foo=1&bar=2")
}

func TestSchemeAndHostAndPathAndQuery(t *testing.T) {
	testLocation(t,
		"http://redirect.example.org/some/other/path?newquery=3",
		"http://redirect.example.org/some/other/path?newquery=3")
}

func TestHostOnly(t *testing.T) {
	testLocation(t,
		"//redirect.example.org",
		"https://redirect.example.org/some/path?foo=1&bar=2")
}

func TestHostAndPath(t *testing.T) {
	testLocation(t,
		"//redirect.example.org/some/other/path",
		"https://redirect.example.org/some/other/path?foo=1&bar=2")
}

func TestHostAndPathAndQuery(t *testing.T) {
	testLocation(t,
		"//redirect.example.org/some/other/path?newquery=3",
		"https://redirect.example.org/some/other/path?newquery=3")
}

func TestPathOnly(t *testing.T) {
	testLocation(t,
		"/some/other/path",
		"https://incoming.example.org/some/other/path?foo=1&bar=2")
}

func TestPathAndQuery(t *testing.T) {
	testLocation(t,
		"/some/other/path?newquery=3",
		"https://incoming.example.org/some/other/path?newquery=3")
}

func TestQueryOnly(t *testing.T) {
	testLocation(t,
		"?newquery=3",
		"https://incoming.example.org/some/path?newquery=3")
}

func TestSchemeAndPath(t *testing.T) {
	testLocation(t,
		"http:///some/other/path",
		"http://incoming.example.org/some/other/path?foo=1&bar=2")
}

func TestSchemeAndPathAndQuery(t *testing.T) {
	testLocation(t,
		"http:///some/other/path?newquery=3",
		"http://incoming.example.org/some/other/path?newquery=3")
}

func TestSchemeAndQuery(t *testing.T) {
	testLocation(t,
		"http://?newquery=3",
		"http://incoming.example.org/some/path?newquery=3")
}
