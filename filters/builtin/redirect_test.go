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
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRedirect(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		code           int
		filterLocation string
		checkLocation  string
	}{{
		"schema only",
		http.StatusFound,
		"http:",
		"http://incoming.example.org/some/path?foo=1&bar=2",
	}, {
		"schema and host",
		http.StatusFound,
		"http://redirect.example.org",
		"http://redirect.example.org/some/path?foo=1&bar=2",
	}, {
		"schema, host and path",
		http.StatusFound,
		"http://redirect.example.org/some/other/path",
		"http://redirect.example.org/some/other/path?foo=1&bar=2",
	}, {
		"schema, host, path and query",
		http.StatusFound,
		"http://redirect.example.org/some/other/path?newquery=3",
		"http://redirect.example.org/some/other/path?newquery=3",
	}, {
		"host only",
		http.StatusFound,
		"//redirect.example.org",
		"https://redirect.example.org/some/path?foo=1&bar=2",
	}, {
		"host and path",
		http.StatusFound,
		"//redirect.example.org/some/other/path",
		"https://redirect.example.org/some/other/path?foo=1&bar=2",
	}, {
		"host, path and query",
		http.StatusFound,
		"//redirect.example.org/some/other/path?newquery=3",
		"https://redirect.example.org/some/other/path?newquery=3",
	}, {
		"path only",
		http.StatusFound,
		"/some/other/path",
		"https://incoming.example.org/some/other/path?foo=1&bar=2",
	}, {
		"path and query",
		http.StatusFound,
		"/some/other/path?newquery=3",
		"https://incoming.example.org/some/other/path?newquery=3",
	}, {
		"query only",
		http.StatusFound,
		"?newquery=3",
		"https://incoming.example.org/some/path?newquery=3",
	}, {
		"schema and path",
		http.StatusFound,
		"http:///some/other/path",
		"http://incoming.example.org/some/other/path?foo=1&bar=2",
	}, {
		"schema, path and query",
		http.StatusFound,
		"http:///some/other/path?newquery=3",
		"http://incoming.example.org/some/other/path?newquery=3",
	}, {
		"schema and query",
		http.StatusFound,
		"http://?newquery=3",
		"http://incoming.example.org/some/path?newquery=3",
	}, {
		"different code",
		http.StatusMovedPermanently,
		"/some/path",
		"https://incoming.example.org/some/path?foo=1&bar=2",
	}} {
		for _, tii := range []struct {
			msg  string
			name string
		}{{
			"deprecated",
			RedirectName,
		}, {
			"not deprecated",
			RedirectToName,
		}} {
			dc := testdataclient.New([]*eskip.Route{{
				Shunt: true,
				Filters: []*eskip.Filter{{
					Name: tii.name,
					Args: []interface{}{float64(ti.code), ti.filterLocation}}}}})
			rt := routing.New(routing.Options{
				FilterRegistry: MakeRegistry(),
				DataClients:    []routing.DataClient{dc}})

			// pick up routing
			time.Sleep(30 * time.Millisecond)

			p := proxy.New(rt, proxy.OptionsNone)
			req := &http.Request{
				URL:  &url.URL{Path: "/some/path", RawQuery: "foo=1&bar=2"},
				Host: "incoming.example.org"}
			w := httptest.NewRecorder()
			p.ServeHTTP(w, req)

			if w.Code != ti.code {
				t.Error(ti.msg, tii.msg, "invalid status code", w.Code)
			}

			if w.Header().Get("Location") != ti.checkLocation {
				t.Error(ti.msg, tii.msg, "invalid location", w.Header().Get("Location"))
			}
		}
	}
}
