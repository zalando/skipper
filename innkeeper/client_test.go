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
	if r.Header.Get(authHeaderName) != testAuthenticationToken {
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
		&routeData{1, "2015-09-28T16:58:56.955", "", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/"},
			nil, nil, nil,
			endpoint{endpointReverseProxy, "HTTPS", "example.org", 443, ""}}},
		&routeData{2, "", "2015-09-28T16:58:56.956", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/catalog"},
			&pathRewrite{Match: "", Replace: "/catalog"}, nil, nil,
			endpoint{endpointReverseProxy, "HTTPS", "example.org", 443, ""}}},
		&routeData{3, "2015-09-28T16:58:56.957", "", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/catalog"},
			&pathRewrite{Match: "", Replace: "/new-catalog"}, nil, nil,
			endpoint{endpointReverseProxy, "HTTPS", "catalog.example.org", 443, ""}}}}
}

func checkDoc(t *testing.T, rs []*eskip.Route, d []*routeData) {
	check, _, _ := convertData(d, nil, nil)
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

func TestParsingInnkeeperRoute(t *testing.T) {
	const testInnkeeperRoute = `{
        "id": 1,
        "createdAt": "2015-09-28T16:58:56.955",
        "deletedAt": "2015-09-28T16:58:56.956",
        "route": {
            "description": "The New Route",
            "match_methods": ["GET"],
            "match_headers": [
                {"name": "header0", "value": "value0"},
                {"name": "header1", "value": "value1"}
            ],
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "path_rewrite": {
                "match": "_",
                "replace": "-"
            },
            "request_headers": [
                {"name": "header2", "value": "value2"},
                {"name": "header3", "value": "value3"}
            ],
            "response_headers": [
                {"name": "header4", "value": "value4"},
                {"name": "header5", "value": "value5"}
            ],
            "endpoint": {
                "hostname": "www.example.org",
                "port": 443,
                "protocol": "HTTPS",
                "type": "REVERSE_PROXY"
            }
        }
    }`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Id != 1 || r.CreatedAt != "2015-09-28T16:58:56.955" || r.DeletedAt != "2015-09-28T16:58:56.956" {
		t.Error("failed to parse route data")
	}

	if len(r.Route.MatchMethods) != 1 || r.Route.MatchMethods[0] != "GET" {
		t.Error("failed to parse methods")
	}

	if len(r.Route.MatchHeaders) != 2 ||
		r.Route.MatchHeaders[0].Name != "header0" || r.Route.MatchHeaders[0].Value != "value0" ||
		r.Route.MatchHeaders[1].Name != "header1" || r.Route.MatchHeaders[1].Value != "value1" {
		t.Error("failed to parse methods")
	}

	if r.Route.MatchPath.Typ != "STRICT" || r.Route.MatchPath.Match != "/route" {
		t.Error("failed to parse path match", r.Route.MatchPath.Typ, r.Route.MatchPath.Match)
	}

	if r.Route.RewritePath == nil || r.Route.RewritePath.Match != "_" || r.Route.RewritePath.Replace != "-" {
		t.Error("failed to path rewrite")
	}

	if len(r.Route.RequestHeaders) != 2 ||
		r.Route.RequestHeaders[0].Name != "header2" || r.Route.RequestHeaders[0].Name != "header2" ||
		r.Route.RequestHeaders[1].Name != "header3" || r.Route.RequestHeaders[1].Name != "header3" {
		t.Error("failed to parse request headers")
	}

	if len(r.Route.ResponseHeaders) != 2 ||
		r.Route.ResponseHeaders[0].Name != "header4" || r.Route.ResponseHeaders[0].Name != "header4" ||
		r.Route.ResponseHeaders[1].Name != "header5" || r.Route.ResponseHeaders[1].Name != "header5" {
		t.Error("failed to parse request headers")
	}

	if r.Route.Endpoint.Hostname != "www.example.org" ||
		r.Route.Endpoint.Port != 443 ||
		r.Route.Endpoint.Protocol != "HTTPS" ||
		r.Route.Endpoint.Typ != "REVERSE_PROXY" {
		t.Error("failed to parse endpoint")
	}
}

func TestParsingInnkeeperRouteNoPathRewrite(t *testing.T) {
	const testInnkeeperRoute = `{
        "id": 1,
        "route": {}
    }`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Route.RewritePath != nil {
		t.Error("failed to path rewrite")
	}
}

func TestParsingMultipleInnkeeperRoutes(t *testing.T) {
	const testInnkeeperRoutes = `[{
        "id": 1,
        "route": {
            "description": "The New Route",
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "endpoint": {
                "hostname": "domain.eu"
            }
        }
    }, {
        "id": 2,
        "route": {
            "description": "The New Route",
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "endpoint": {
                "hostname": "domain.eu"
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
	const testInnkeeperRoutes = `[{"id": 1}, {"id": 2, "deletedAt": "2015-09-28T16:58:56.956"}]`

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

	rs, deleted, lastChange := convertData(testData(), nil, nil)

	test(len(rs), 2)
	if failed {
		return
	}

	test(rs[0].Id, "route1")
	test(rs[0].Path, "/")
	test(rs[0].Shunt, false)
	test(rs[0].Backend, "https://example.org:443")

	test(rs[1].Id, "route3")
	test(rs[1].Path, "/catalog")
	test(rs[1].Shunt, false)
	test(rs[1].Backend, "https://catalog.example.org:443")

	test(len(deleted), 1)
	test(lastChange, "2015-09-28T16:58:56.957")
}

func TestConvertRoutePathRegexp(t *testing.T) {
	d := &routeData{Route: routeDef{MatchPath: pathMatch{Typ: pathMatchRegexp, Match: "test-rx"}}}
	r := convertRoute("testRoute", d, nil, nil)
	if len(r.PathRegexps) != 1 || r.PathRegexps[0] != "test-rx" {
		t.Error("failed to convert path regexp")
	}
}

func TestConvertRouteMethods(t *testing.T) {
	d := &routeData{Id: 42, Route: routeDef{MatchMethods: []string{"GET", "HEAD"}}}
	rs, _, _ := convertData([]*routeData{d}, nil, nil)
	if len(rs) != 2 ||
		rs[0].Id != "route42GET" || rs[0].Method != "GET" ||
		rs[1].Method != "HEAD" || rs[1].Method != "HEAD" {
		t.Error("failed to convert methods")
	}
}

func TestConvertRouteHeaders(t *testing.T) {
	d := &routeData{Route: routeDef{MatchHeaders: []headerData{
		{Name: "header0", Value: "value0"},
		{Name: "header1", Value: "value1"}}}}
	rs := convertRoute("", d, nil, nil)
	if len(rs.Headers) != 2 || rs.Headers["header0"] != "value0" || rs.Headers["header1"] != "value1" {
		t.Error("failed to convert headers")
	}
}

func TestConvertFilters(t *testing.T) {
	d := &routeData{Route: routeDef{
		RewritePath:     &pathRewrite{Match: "test-rx", Replace: "replacement"},
		RequestHeaders:  []headerData{{Name: "header0", Value: "value0"}},
		ResponseHeaders: []headerData{{Name: "header1", Value: "value1"}}}}
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
	d := &routeData{Route: routeDef{Endpoint: endpoint{
		Typ:      endpointPermanentRedirect,
		Protocol: "HTTPS",
		Hostname: "www.example.org",
		Port:     443,
		Path:     "/some/path"}}}
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
	_, _, lastChange := convertData(d, nil, nil)
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
	newRoute := &routeData{4, "2015-09-28T16:58:56.959", "", routeDef{
		"", nil, nil,
		pathMatch{pathMatchStrict, "/catalog"},
		nil, nil, nil,
		endpoint{endpointReverseProxy, "HTTPS", "example.org", 443, "/even-newer-catalog"}}}
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
	newRoute := &routeData{4, "2015-09-28T16:58:56.959", "", routeDef{
		"", nil, nil,
		pathMatch{pathMatchStrict, "/catalog"},
		nil, nil, nil,
		endpoint{endpointReverseProxy, "HTTPS", "example.org", 443, "/even-newer-catalog"}}}
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
		di.Route.RewritePath = &pathRewrite{"", "replacement"}
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
			t.Error("failed to parse filters")
		}

		if r.Filters[0].Name != "filter1" ||
			len(r.Filters[0].Args) != 1 ||
			r.Filters[0].Args[0] != float64(3.14) {
			t.Error("failed to parse filters")
		}

		if r.Filters[1].Name != "filter2" ||
			len(r.Filters[1].Args) != 2 ||
			r.Filters[1].Args[0] != "key" ||
			r.Filters[1].Args[1] != float64(42) {
			t.Error("failed to parse filters")
		}

		if r.Filters[2].Name != builtin.ModPathName ||
			len(r.Filters[2].Args) != 2 ||
			r.Filters[2].Args[0] != ".*" ||
			r.Filters[2].Args[1] != "replacement" {
			t.Error("failed to parse filters")
		}

		if r.Filters[3].Name != "filter3" ||
			len(r.Filters[3].Args) != 1 ||
			r.Filters[3].Args[0] != "Hello, world!" {
			t.Error("failed to parse filters")
		}
	}
}
