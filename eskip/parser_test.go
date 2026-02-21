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
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	singleRouteExample = `
        PathRegexp(/\.html$/) && Header("Accept", "text/html") ->
        modPath(/\.html$/, ".jsx") ->
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
            "https://api.example.com";

        catchAll: * -> "https://www.example.org";
        catchAllWithCustom: * && Custom() -> "https://www.example.org"`
)

func checkSingleRouteExample(r *parsedRoute, t *testing.T) {
	if len(r.predicates) != 2 ||
		r.predicates[0].Name != "PathRegexp" || len(r.predicates[0].Args) != 1 ||
		r.predicates[0].Args[0] != "\\.html$" ||
		r.predicates[1].Name != "Header" || len(r.predicates[1].Args) != 2 ||
		r.predicates[1].Args[0] != "Accept" || r.predicates[1].Args[1] != "text/html" {
		t.Error("failed to parse match expression")
	}

	if len(r.filters) != 2 {
		t.Error("failed to parse filters", len(r.filters))
	}

	if r.filters[0].Name != "modPath" || r.filters[1].Name != "requestHeader" {
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
	_, err := parseDocument("invalid code")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestParseSingleRoute(t *testing.T) {
	r, err := parseDocument(singleRouteExample)

	if err != nil {
		t.Error("failed to parse", err)
	}

	if len(r) != 1 {
		t.Error("failed to parse, no route returned")
	}

	checkSingleRouteExample(r[0], t)
}

func TestParsingSpecialChars(t *testing.T) {
	r, err := parseDocument(`Path("newlines") -> inlineContent("Line \ \1\nLine 2\r\nLine 3\a\b\f\n\r\t\v") -> <shunt>`)

	if err != nil {
		t.Error("failed to parse", err)
	}

	if len(r) != 1 {
		t.Error("failed to parse, no route returned")
	}

	expected := "Line \\ \\1\nLine 2\r\nLine 3\a\b\f\n\r\t\v"
	actual := r[0].filters[0].Args[0]

	if actual != expected {
		t.Error("wrong arguments")
	}
}

func TestParseSingleRouteDef(t *testing.T) {
	r, err := parseDocument(singleRouteDefExample)

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

	_, err := parseDocument(missingSemicolon)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestParseDocument(t *testing.T) {
	r, err := parseDocument(routingDocumentExample)

	if err != nil {
		t.Error("failed to parse document", err)
	}

	if len(r) != 6 {
		t.Error("failed to parse document", len(r))
	}

	some := func(r []*parsedRoute, f func(*parsedRoute) bool) bool {
		return slices.ContainsFunc(r, f)
	}

	mkidcheck := func(n string) func(*parsedRoute) bool {
		return func(r *parsedRoute) bool {
			return r.id == n
		}
	}

	if !some(r, mkidcheck("route0")) ||
		!some(r, mkidcheck("route1")) ||
		!some(r, mkidcheck("route2")) ||
		!some(r, mkidcheck("route3")) ||
		!some(r, mkidcheck("catchAll")) ||
		!some(r, mkidcheck("catchAllWithCustom")) {
		t.Error("failed to parse route definition ids")
	}
}

func TestNumberNotClosedWithDecimalSign(t *testing.T) {
	_, err := parseDocument(`* -> number(3.) -> <shunt>`)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestNumberStartingWithDecimal(t *testing.T) {
	_, err := parseDocument(`* -> number(.3) -> <shunt>`)
	if err != nil {
		t.Error("failed to parse number", err)
	}
}

func TestNumber(t *testing.T) {
	_, err := parseDocument(`* -> number(3.14) -> <shunt>`)
	if err != nil {
		t.Error("failed to parse number", err)
	}
}

func TestRegExp(t *testing.T) {
	testRegExpOnce(t, `PathRegexp(/[/]/)-> <shunt>`, `[/]`)
	testRegExpOnce(t, `PathRegexp(/[\[]/)-> <shunt>`, `[\[]`)
	testRegExpOnce(t, `PathRegexp(/[\]]/)-> <shunt>`, `[\]]`)
	testRegExpOnce(t, `PathRegexp(/[\\]/)-> <shunt>`, `[\]`)
	testRegExpOnce(t, `PathRegexp(/[\/]/)-> <shunt>`, `[/]`)
	testRegExpOnce(t, `PathRegexp(/["]/)-> <shunt>`, `["]`)
	testRegExpOnce(t, `PathRegexp(/[\"]/)-> <shunt>`, `[\"]`)
	testRegExpOnce(t, `PathRegexp(/\//)-> <shunt>`, `/`)
	testRegExpOnce(t, `PathRegexp(/[[:upper:]]/)-> <shunt>`, `[[:upper:]]`)
}

func testRegExpOnce(t *testing.T, regexpStr string, expectedRegExp string) {
	routes, err := parseDocument(regexpStr)
	if err != nil {
		t.Error("failed to parse PathRegexp:"+regexpStr, err)
		return
	}

	if expectedRegExp != routes[0].predicates[0].Args[0] {
		t.Error("failed to parse PathRegexp:"+regexpStr+", expected regexp to be "+expectedRegExp, err)
	}
}

func TestLBBackend(t *testing.T) {
	for _, test := range []struct {
		title          string
		code           string
		expectedResult []*Route
		fail           bool
	}{{
		title: "empty",
		code:  "* -> <>",
		fail:  true,
	}, {
		title: "empty with whitespace",
		code:  "* -> <   >",
		fail:  true,
	}, {
		title: "algorithm only",
		code:  "* -> <roundRobin>",
		fail:  true,
	}, {
		title: "single endpoint, default algorithm",
		code:  `* -> <"https://example.org">`,
		expectedResult: []*Route{{
			BackendType: LBBackend,
			LBEndpoints: []string{"https://example.org"},
		}},
	}, {
		title: "multiple endpoints, default algorithm",
		code: `* -> <"https://example1.org",
		             "https://example2.org",
		             "https://example3.org">`,
		expectedResult: []*Route{{
			BackendType: LBBackend,
			LBEndpoints: []string{
				"https://example1.org",
				"https://example2.org",
				"https://example3.org",
			},
		}},
	}, {
		title: "single endpoint, with algorithm",
		code:  `* -> <algFoo, "https://example.org">`,
		expectedResult: []*Route{{
			BackendType: LBBackend,
			LBAlgorithm: "algFoo",
			LBEndpoints: []string{"https://example.org"},
		}},
	}, {
		title: "multiple endpoints, default algorithm",
		code: `* -> <algFoo,
		             "https://example1.org",
		             "https://example2.org",
		             "https://example3.org">`,
		expectedResult: []*Route{{
			BackendType: LBBackend,
			LBAlgorithm: "algFoo",
			LBEndpoints: []string{
				"https://example1.org",
				"https://example2.org",
				"https://example3.org",
			},
		}},
	}, {
		title: "multiple endpoints, default algorithm, with filters",
		code: `* -> foo() -> <algFoo,
		             "https://example1.org",
		             "https://example2.org",
		             "https://example3.org">`,
		expectedResult: []*Route{{
			Filters:     []*Filter{{Name: "foo"}},
			BackendType: LBBackend,
			LBAlgorithm: "algFoo",
			LBEndpoints: []string{
				"https://example1.org",
				"https://example2.org",
				"https://example3.org",
			},
		}},
	}} {
		t.Run(test.title, func(t *testing.T) {
			r, err := Parse(test.code)
			if test.fail && err == nil {
				t.Fatal("failed to fail")
			}

			if err != nil && !test.fail {
				t.Fatal(err)
			}

			if test.fail {
				return
			}

			if d := cmp.Diff(r, test.expectedResult); d != "" {
				t.Log("failed to parse routes")
				t.Log(d)
				t.Fatal()
			}
		})
	}
}
