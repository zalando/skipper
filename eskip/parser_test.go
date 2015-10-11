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

package eskip

import (
	"testing"
)

const (
	singleRouteExample = `
        PathRegexp(/\.html$/) && Header("Accept", "text/html") ->
        pathRewrite(/\.html$/, ".jsx") ->
        requestHeader("X-Type", "page") ->
        "https://render.example.com"`

	singleRouteDefExample = "testroute:" + singleRouteExample

	routingDocumentExample = `
        route0: ` + singleRouteExample + `;
        
        route1: Path("/some/path") -> "https://backend-0.example.com";
        route2: Path("/some/other/path") -> fixPath() -> "https://backend-1.example.com";
        
        route3:
            Method("POST") && Path("/api") ->
            requestHeader("X-Type", "ajax-post") ->
            "https://api.example.com"`
)

func checkSingleRouteExample(r *parsedRoute, t *testing.T) {
	if len(r.matchers) != 2 ||
		r.matchers[0].name != "PathRegexp" || len(r.matchers[0].args) != 1 ||
		r.matchers[0].args[0] != "\\.html$" ||
		r.matchers[1].name != "Header" || len(r.matchers[1].args) != 2 ||
		r.matchers[1].args[0] != "Accept" || r.matchers[1].args[1] != "text/html" {
		t.Error("failed to parse match expression")
	}

	if len(r.filters) != 2 {
		t.Error("failed to parse filters", len(r.filters))
	}

	if r.filters[0].Name != "pathRewrite" || r.filters[1].Name != "requestHeader" {
		t.Error("failed to parse filter name", r.filters[0].Name, r.filters[1].Name)
	}

	if len(r.filters[0].Args) != 2 || len(r.filters[1].Args) != 2 {
		t.Error("failed to parse filter args", len(r.filters[0].Args) != 2, len(r.filters[1].Args))
	}

	if r.filters[0].Args[0].(string) != `\.html$` ||
		r.filters[0].Args[1].(string) != ".jsx" ||
		r.filters[1].Args[0].(string) != "X-Type" ||
		r.filters[1].Args[1].(string) != "page" {
		t.Error("failed to parse filter args",
			r.filters[0].Args[0].(string),
			r.filters[0].Args[1].(string),
			r.filters[1].Args[0].(string),
			r.filters[1].Args[1].(string))
	}

	if r.shunt || r.backend != "https://render.example.com" {
		t.Error("failed to parse filter backend", r.shunt, r.backend)
	}
}

func TestReturnsLexerErrors(t *testing.T) {
	_, err := parse("invalid code")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestParseSingleRoute(t *testing.T) {
	r, err := parse(singleRouteExample)

	if err != nil {
		t.Error("failed to parse", err)
	}

	if len(r) != 1 {
		t.Error("failed to parse, no route returned")
	}

	checkSingleRouteExample(r[0], t)
}

func TestParseSingleRouteDef(t *testing.T) {
	r, err := parse(singleRouteDefExample)

	if err != nil {
		t.Error("failed to parse", err)
	}

	if len(r) != 1 {
		t.Error("failed to parse, no route returned")
	}

	checkSingleRouteExample(r[0], t)

	if r[0].id != "testroute" {
		t.Error("failed to parse route definition id", r[0].id)
	}
}

func TestParseInvalidDocument(t *testing.T) {
	missingSemicolon := `
        route0: Method("GET") -> "https://backend-0.example.com"
        route1: Method("POST") -> "https://backend-1.example.com"`

	_, err := parse(missingSemicolon)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestParseDocument(t *testing.T) {
	r, err := parse(routingDocumentExample)

	if err != nil {
		t.Error("failed to parse document", err)
	}

	if len(r) != 4 {
		t.Error("failed to parse document", len(r))
	}

	some := func(r []*parsedRoute, f func(*parsedRoute) bool) bool {
		for _, ri := range r {
			if f(ri) {
				return true
			}
		}

		return false
	}

	mkidcheck := func(n string) func(*parsedRoute) bool {
		return func(r *parsedRoute) bool {
			return r.id == n
		}
	}

	if !some(r, mkidcheck("route0")) ||
		!some(r, mkidcheck("route1")) ||
		!some(r, mkidcheck("route2")) ||
		!some(r, mkidcheck("route3")) {
		t.Error("failed to parse route definition ids")
	}
}
