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

package innkeeper

import (
	"encoding/json"
	"errors"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
)

const testAuthenticationToken = "test token"

type autoAuth bool

func (aa autoAuth) GetToken() (string, error) {
	if aa {
		return testAuthenticationToken, nil
	}

	return "", errors.New(string(authErrorAuthentication))
}

type innkeeperHandler struct{ data []*routeData }

func (h *innkeeperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(authHeaderName) != "Bearer "+testAuthenticationToken {
		w.WriteHeader(http.StatusUnauthorized)
		enc := json.NewEncoder(w)

		// ignoring error
		enc.Encode(&apiError{ErrorType: string(authErrorPermission)})

		return
	}

	var responseData []*routeData
	if r.URL.Path == "/routes" {
		for _, di := range h.data {
			if di.DeletedAt == "" {
				responseData = append(responseData, di)
			}
		}
	} else {
		lastMod := path.Base(r.URL.Path)
		if lastMod == updatePathRoot {
			lastMod = ""
		}

		for _, di := range h.data {
			if di.CreatedAt > lastMod || di.DeletedAt > lastMod {
				responseData = append(responseData, di)
			}
		}
	}

	if b, err := json.Marshal(responseData); err == nil {
		w.Write(b)
	} else {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func innkeeperServer(data []*routeData) *httptest.Server {
	return httptest.NewServer(&innkeeperHandler{data})
}

func testData() []*routeData {
	return []*routeData{
		&routeData{
			Id:         1,
			Name:       "",
			ActivateAt: "2015-09-28T16:58:56.955",
			CreatedAt:  "2015-09-28T16:58:56.955",
			DeletedAt:  "",
			Route: routeDef{
				Matcher: matcher{
					HostMatcher:    "",
					PathMatcher:    &pathMatcher{matchStrict, "/"},
					MethodMatcher:  "GET",
					HeaderMatchers: nil},
				Filters:  nil,
				Endpoint: "https://example.org:443"},
		}, &routeData{
			Id:         2,
			Name:       "",
			ActivateAt: "2015-09-28T16:58:56.955",
			CreatedAt:  "2015-09-28T16:58:56.955",
			DeletedAt:  "2015-09-28T16:58:56.956",
			Route: routeDef{
				Matcher: matcher{
					HostMatcher:    "",
					PathMatcher:    &pathMatcher{matchStrict, "/"},
					MethodMatcher:  "GET",
					HeaderMatchers: nil},
				Filters:  nil,
				Endpoint: "https://example.org:443"},
		}, &routeData{
			Id:         3,
			Name:       "",
			ActivateAt: "2015-09-28T16:58:56.955",
			CreatedAt:  "2015-09-28T16:58:56.955",
			DeletedAt:  "2015-09-28T16:58:56.956",
			Route: routeDef{
				Matcher: matcher{
					HostMatcher:    "",
					PathMatcher:    &pathMatcher{matchStrict, "/"},
					MethodMatcher:  "GET",
					HeaderMatchers: nil},
				Filters: []filter{
					filter{Name: "pathRewrite", Args: []interface{}{"", "/catalog"}}},
				Endpoint: "https://example.org:443/"},
		}, &routeData{
			Id:         4,
			Name:       "",
			ActivateAt: "2015-09-28T16:58:56.957",
			CreatedAt:  "2015-09-28T16:58:56.957",
			DeletedAt:  "",
			Route: routeDef{
				Matcher: matcher{
					HostMatcher:    "",
					PathMatcher:    &pathMatcher{matchStrict, "/catalog"},
					MethodMatcher:  "GET",
					HeaderMatchers: nil},
				Filters: []filter{
					filter{Name: "pathRewrite", Args: []interface{}{"", "/new-catalog"}}},
				Endpoint: "https://catalog.example.org:443"}}}
}

func checkDoc(t *testing.T, rs []*eskip.Route, d []*routeData) {
	check, _, _ := convertJsonToEskip(d, nil, nil)
	if len(rs) != len(check) {
		t.Error("doc lengths do not match")
		return
	}

	for i, r := range rs {
		if r.Id != check[i].Id {
			t.Error("doc id does not match")
			return
		}

		if r.Path != check[i].Path {
			t.Error("doc path does not match")
			return
		}

		if len(r.PathRegexps) != len(check[i].PathRegexps) {
			t.Error("doc path regexp lengths do not match")
			return
		}

		for j, rx := range r.PathRegexps {
			if rx != check[i].PathRegexps[j] {
				t.Error("doc path regexp does not match")
				return
			}
		}

		if r.Method != check[i].Method {
			t.Error("doc method does not match")
			return
		}

		if len(r.Headers) != len(check[i].Headers) {
			t.Error("doc header lengths do not match")
			return
		}

		for k, h := range r.Headers {
			if h != check[i].Headers[k] {
				t.Error("doc header does not match")
				return
			}
		}

		if len(r.Filters) != len(check[i].Filters) {
			t.Error("doc filter lengths do not match")
			return
		}

		for j, f := range r.Filters {
			if f.Name != check[i].Filters[j].Name {
				t.Error("doc filter does not match")
				return
			}

			if len(f.Args) != len(check[i].Filters[j].Args) {
				t.Error("doc filter arg lengths do not match")
				return
			}

			for k, a := range f.Args {
				if a != check[i].Filters[j].Args[k] {
					t.Error("doc filter arg does not match")
					return
				}
			}
		}

		if r.Shunt != check[i].Shunt {
			t.Error("doc shunt does not match")
			return
		}

		if r.Backend != check[i].Backend {
			t.Error("doc backend does not match")
			return
		}
	}
}

func TestParsingInnkeeperSimpleRoute(t *testing.T) {
	const testInnkeeperRoute = `{
			"name": "THE_ROUTE",
			"description": "this is a route",
			"activate_at": "2015-09-28T16:58:56.957",
			"id": 1,
			"created_at": "2015-09-28T16:58:56.955",
			"deleted_at": "2015-09-28T16:58:56.956",
			"route": {
				"matcher": {
					"path_matcher": {
						"match": "/hello-*",
						"type": "REGEX"
					}
				}
			}
		}`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Name != "THE_ROUTE" {
		t.Error("failed to parse the name")
	}

	if r.Id != 1 || r.CreatedAt != "2015-09-28T16:58:56.955" || r.DeletedAt != "2015-09-28T16:58:56.956" ||
		r.ActivateAt != "2015-09-28T16:58:56.957" {
		t.Error("failed to parse route data")
	}

	if r.Route.Matcher.PathMatcher.Match != "/hello-*" || r.Route.Matcher.PathMatcher.Typ != "REGEX" {
		t.Error("failed to parse path matcher")
	}

	if r.Route.Endpoint != "" {
		t.Error("failed to parse the endpoint")
	}
}

func TestParsingInnkeeperComplexRoute(t *testing.T) {
	const testInnkeeperRoute = `{
			"name": "THE_ROUTE",
			"description": "this is a route",
			"activate_at": "2015-09-28T16:58:56.957",
			"id": 1,
			"created_at": "2015-09-28T16:58:56.955",
			"deleted_at": "2015-09-28T16:58:56.956",
			"route": {
				"matcher": {
					"host_matcher": "example.com",
					"path_matcher": {
						"match": "/hello-*",
						"type": "REGEX"
					},
					"method_matcher": "POST",
					"header_matchers": [{
						"name": "X-Host",
						"value": "www.*",
						"type": "REGEX"
					}, {
						"name": "X-Port",
						"value": "8080",
						"type": "STRICT"
					}]
				},
				"filters": [{
					"name": "someFilter",
					"args": ["Hello", 123]
				}, {
					"name": "someOtherFilter",
					"args": ["Hello", 123, "World"]
				}],
				"endpoint": "https://www.endpoint.com:8080/endpoint"
			}
		}`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Name != "THE_ROUTE" {
		t.Error("failed to parse the name")
	}

	if r.Id != 1 || r.CreatedAt != "2015-09-28T16:58:56.955" || r.DeletedAt != "2015-09-28T16:58:56.956" ||
		r.ActivateAt != "2015-09-28T16:58:56.957" {
		t.Error("failed to parse route data")
	}

	if r.Route.Matcher.HostMatcher != "example.com" {
		t.Error("failed to parse the host matcher")
	}

	if r.Route.Matcher.MethodMatcher != "POST" {
		t.Error("failed to parse the method matcher")
	}

	if r.Route.Matcher.PathMatcher.Match != "/hello-*" || r.Route.Matcher.PathMatcher.Typ != "REGEX" {
		t.Error("failed to parse path matcher")
	}

	if len(r.Route.Matcher.HeaderMatchers) != 2 || r.Route.Matcher.HeaderMatchers[0].Name != "X-Host" ||
		r.Route.Matcher.HeaderMatchers[0].Typ != "REGEX" ||
		r.Route.Matcher.HeaderMatchers[0].Value != "www.*" {
		t.Error("failed to parse header matchers")
	}

	if len(r.Route.Filters) != 2 {
		t.Error("failed to parse the filters")
	}

	if r.Route.Filters[0].Name != "someFilter" {
		t.Error("failed to parse the filter name")
	}

	args := r.Route.Filters[0].Args

	if len(args) != 2 && args[0] != "Hello" && args[1] != 123 {
		t.Error("failed to parse the filter args")
	}

	if r.Route.Endpoint != "https://www.endpoint.com:8080/endpoint" {
		t.Error("failed to parse the endpoint")
	}
}

func TestParsingMultipleInnkeeperRoutes(t *testing.T) {
	const testInnkeeperRoutes = `[{
			"name": "THE_ROUTE",
			"description": "this is a route",
			"activate_at": "2015-09-28T16:58:56.957",
			"id": 1,
			"created_at": "2015-09-28T16:58:56.955",
			"deleted_at": "2015-09-28T16:58:56.956",
			"route": {
				"matcher": {
					"path_matcher": {
						"match": "/hello-*",
						"type": "REGEX"
					}
				}
			}
		}, {
			"name": "THE_ROUTE",
			"description": "this is a route",
			"activate_at": "2015-09-28T16:58:56.957",
			"id": 2,
			"created_at": "2015-09-28T16:58:56.955",
			"deleted_at": "2015-09-28T16:58:56.956",
			"route": {
				"matcher": {
					"path_matcher": {
						"match": "/hello-*",
						"type": "REGEX"
					}
				}
			}
		}]`

	rs := []*routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoutes), &rs)
	if err != nil {
		t.Error(err)
	}

	if len(rs) != 2 || rs[0].Id != 1 || rs[1].Id != 2 {
		t.Error("failed to parse routes")
	}
}

func TestParsingMultipleInnkeeperRoutesWithDelete(t *testing.T) {
	const testInnkeeperRoutes = `[{"id": 1}, {"id": 2, "deleted_at": "2015-09-28T16:58:56.956"}]`

	rs := []*routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoutes), &rs)
	if err != nil {
		t.Error(err)
	}

	if len(rs) != 2 || rs[0].Id != 1 || rs[1].Id != 2 || rs[0].DeletedAt != "" || rs[1].DeletedAt != "2015-09-28T16:58:56.956" {
		t.Error("failed to parse routes")
	}
}

func TestConvertDoc(t *testing.T) {
	failed := false

	test := func(left, right interface{}, msg ...interface{}) {
		if failed || left == right {
			return
		}

		failed = true
		t.Error(append([]interface{}{"failed to convert data", left, right}, msg...)...)
	}

	rs, deleted, lastChange := convertJsonToEskip(testData(), nil, nil)

	test(len(rs), 2)
	if failed {
		return
	}

	test(rs[0].Id, "route1")
	test(rs[0].Path, "/")
	test(rs[0].Shunt, false)
	test(rs[0].Backend, "https://example.org:443")

	test(rs[1].Id, "route4")
	test(rs[1].Path, "/catalog")
	test(rs[1].Shunt, false)
	test(rs[1].Backend, "https://catalog.example.org:443")

	test(len(deleted), 2)
	test(lastChange, "2015-09-28T16:58:56.957")
}

func TestConvertRoutePathRegexp(t *testing.T) {
	d := &routeData{Route: routeDef{Matcher: matcher{PathMatcher: &pathMatcher{Typ: matchRegex, Match: "test-rx"}}}}
	r := convertRoute("testRoute", d, nil, nil)
	if len(r.PathRegexps) != 1 || r.PathRegexps[0] != "test-rx" {
		t.Error("failed to convert path regexp")
	}
}

func TestConvertRouteMethods(t *testing.T) {
	d := &routeData{Id: 42, Route: routeDef{Matcher: matcher{MethodMatcher: "GET"}}}
	rs, _, _ := convertJsonToEskip([]*routeData{d}, nil, nil)
	if len(rs) != 1 ||
		rs[0].Id != "route42" || rs[0].Method != "GET" {
		t.Error("failed to convert methods")
	}
}

func TestConvertRouteHeaders(t *testing.T) {
	d := &routeData{Route: routeDef{Matcher: matcher{HeaderMatchers: []headerMatcher{
		{Name: "header0", Value: "value0", Typ: matchStrict},
		{Name: "header1", Value: "value1", Typ: matchStrict}}}}}
	rs := convertRoute("", d, nil, nil)

	if len(rs.Headers) != 2 || rs.Headers["header0"] != "value0" ||
		rs.Headers["header1"] != "value1" {
		t.Error("failed to convert headers")
	}
}

func TestConvertFilters(t *testing.T) {
	d := &routeData{Route: routeDef{
		Filters: []filter{
			filter{Name: builtin.ModPathName, Args: []interface{}{"test-rx", "replacement"}},
			filter{Name: builtin.RequestHeaderName, Args: []interface{}{"header0", "value0"}},
			filter{Name: builtin.ResponseHeaderName, Args: []interface{}{"header1", "value1"}},
		}}}

	rs := convertRoute("", d, nil, nil)
	if len(rs.Filters) != 3 ||
		rs.Filters[0].Name != builtin.ModPathName || len(rs.Filters[0].Args) != 2 ||
		rs.Filters[0].Args[0] != "test-rx" || rs.Filters[0].Args[1] != "replacement" ||
		rs.Filters[1].Name != builtin.RequestHeaderName || len(rs.Filters[1].Args) != 2 ||
		rs.Filters[1].Args[0] != "header0" || rs.Filters[1].Args[1] != "value0" ||
		rs.Filters[2].Name != builtin.ResponseHeaderName || len(rs.Filters[2].Args) != 2 ||
		rs.Filters[2].Args[0] != "header1" || rs.Filters[2].Args[1] != "value1" {
		t.Error("failed to convert filters")
	}
}

func TestConvertShunt(t *testing.T) {
	d := &routeData{Route: routeDef{Filters: []filter{filter{Name: builtin.RedirectName,
		Args: []interface{}{fixedRedirectStatus, "https://www.example.org:443/some/path"}}}}}
	rs := convertRoute("", d, nil, nil)

	if !rs.Shunt || len(rs.Filters) != 1 ||
		rs.Filters[0].Name != builtin.RedirectName ||
		len(rs.Filters[0].Args) != 2 ||
		rs.Filters[0].Args[0] != fixedRedirectStatus ||
		rs.Filters[0].Args[1] != "https://www.example.org:443/some/path" {
		t.Error("failed to convert shunt backend")
	}
}

func TestConvertDeletedChangeLatest(t *testing.T) {
	d := testData()
	d[1].DeletedAt = "2015-09-28T16:58:56.958"
	_, _, lastChange := convertJsonToEskip(d, nil, nil)
	if lastChange != "2015-09-28T16:58:56.958" {
		t.Error("failed to detect deleted last change")
	}
}

func TestReceivesEmpty(t *testing.T) {
	s := innkeeperServer(nil)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil || len(rs) != 0 {
		t.Error(err, "failed to receive empty")
	}
}

func TestReceivesInitial(t *testing.T) {
	d := testData()
	s := innkeeperServer(d)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	checkDoc(t, rs, d)
}

func TestFailingAuthOnReceive(t *testing.T) {
	d := testData()
	s := innkeeperServer(d)
	a := autoAuth(false)

	c, err := New(Options{Address: s.URL, Authentication: a})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestReceivesUpdates(t *testing.T) {
	d := testData()
	h := &innkeeperHandler{d}
	s := httptest.NewServer(h)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	d = testData()
	d[2].DeletedAt = "2015-09-28T16:58:56.958"

	newRoute := &routeData{
		Id:         4,
		Name:       "",
		ActivateAt: "2015-09-28T16:58:56.959",
		CreatedAt:  "2015-09-28T16:58:56.959",
		DeletedAt:  "",
		Route: routeDef{
			Matcher: matcher{
				HostMatcher:    "",
				PathMatcher:    &pathMatcher{matchStrict, "/"},
				MethodMatcher:  "GET",
				HeaderMatchers: nil},
			Filters:  nil,
			Endpoint: "https://example.org:443/even-newer-catalog"},
	}

	d = append(d, newRoute)
	h.data = d

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	checkDoc(t, rs, []*routeData{newRoute})
	if len(ds) != 1 || ds[0] != "route3" {
		t.Error("unexpected delete")
	}
}

func TestFailingAuthOnUpdate(t *testing.T) {
	d := testData()
	h := &innkeeperHandler{d}
	s := httptest.NewServer(h)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	c.authToken = ""
	c.opts.Authentication = autoAuth(false)
	d = testData()
	d[2].DeletedAt = "2015-09-28T16:58:56.958"

	newRoute := &routeData{
		Id:         4,
		Name:       "",
		ActivateAt: "2015-09-28T16:58:56.959",
		CreatedAt:  "2015-09-28T16:58:56.959",
		DeletedAt:  "",
		Route: routeDef{
			Matcher: matcher{
				HostMatcher:    "",
				PathMatcher:    &pathMatcher{matchStrict, "/"},
				MethodMatcher:  "GET",
				HeaderMatchers: nil},
			Filters:  nil,
			Endpoint: "https://example.org:443/even-newer-catalog"},
	}

	d = append(d, newRoute)
	h.data = d

	_, _, err = c.LoadUpdate()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestUsesPreAndPostRouteFilters(t *testing.T) {
	d := testData()
	for _, di := range d {
		di.Route.Filters = []filter{filter{Name: builtin.ModPathName, Args: []interface{}{".*", "replacement"}}}
	}

	s := innkeeperServer(d)

	c, err := New(Options{
		Address:          s.URL,
		Authentication:   autoAuth(true),
		PreRouteFilters:  `filter1(3.14) -> filter2("key", 42)`,
		PostRouteFilters: `filter3("Hello, world!")`})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	for _, r := range rs {
		if len(r.Filters) != 4 {
			t.Error("failed to parse filters 1")
		}

		if r.Filters[0].Name != "filter1" ||
			len(r.Filters[0].Args) != 1 ||
			r.Filters[0].Args[0] != float64(3.14) {
			t.Error("failed to parse filters 2")
		}

		if r.Filters[1].Name != "filter2" ||
			len(r.Filters[1].Args) != 2 ||
			r.Filters[1].Args[0] != "key" ||
			r.Filters[1].Args[1] != float64(42) {
			t.Error("failed to parse filters 3")
		}

		if r.Filters[2].Name != builtin.ModPathName ||
			len(r.Filters[2].Args) != 2 ||
			r.Filters[2].Args[0] != ".*" ||
			r.Filters[2].Args[1] != "replacement" {
			t.Error("failed to parse filters 4")
		}

		if r.Filters[3].Name != "filter3" ||
			len(r.Filters[3].Args) != 1 ||
			r.Filters[3].Args[0] != "Hello, world!" {
			t.Error("failed to parse filters 5")
		}
	}
}
