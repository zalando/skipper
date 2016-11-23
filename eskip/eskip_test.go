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

import "testing"

func checkItems(t *testing.T, message string, l, lenExpected int, checkItem func(int) bool) bool {
	if l != lenExpected {
		t.Error(message, "length", l, lenExpected)
		return false
	}

	for i := 0; i < l; i++ {
		if !checkItem(i) {
			t.Error(message, "item", i)
			return false
		}
	}

	return true
}

func checkFilters(t *testing.T, message string, fs, fsExp []*Filter) bool {
	return checkItems(t, "filters "+message,
		len(fs),
		len(fsExp),
		func(i int) bool {
			return fs[i].Name == fsExp[i].Name &&
				checkItems(t, "filter args",
					len(fs[i].Args),
					len(fsExp[i].Args),
					func(j int) bool {
						return fs[i].Args[j] == fsExp[i].Args[j]
					})
		})
}

func TestParseRouteExpression(t *testing.T) {
	for _, ti := range []struct {
		msg        string
		expression string
		check      *Route
		err        bool
	}{{
		"path predicate",
		`Path("/some/path") -> "https://www.example.org"`,
		&Route{Path: "/some/path", Backend: "https://www.example.org"},
		false,
	}, {
		"path regexp",
		`PathRegexp("^/some") && PathRegexp(/\/\w+Id$/) -> "https://www.example.org"`,
		&Route{
			PathRegexps: []string{"^/some", "/\\w+Id$"},
			Backend:     "https://www.example.org"},
		false,
	}, {
		"method predicate",
		`Method("HEAD") -> "https://www.example.org"`,
		&Route{Method: "HEAD", Backend: "https://www.example.org"},
		false,
	}, {
		"invalid method predicate",
		`Path("/endpoint") && Method("GET", "POST") -> "https://www.example.org"`,
		nil,
		true,
	}, {
		"host regexps",
		`Host(/^www[.]/) && Host(/[.]org$/) -> "https://www.example.org"`,
		&Route{HostRegexps: []string{"^www[.]", "[.]org$"}, Backend: "https://www.example.org"},
		false,
	}, {
		"headers",
		`Header("Header-0", "value-0") &&
		Header("Header-1", "value-1") ->
		"https://www.example.org"`,
		&Route{
			Headers: map[string]string{"Header-0": "value-0", "Header-1": "value-1"},
			Backend: "https://www.example.org"},
		false,
	}, {
		"header regexps",
		`HeaderRegexp("Header-0", "value-0") &&
		HeaderRegexp("Header-0", "value-1") &&
		HeaderRegexp("Header-1", "value-2") &&
		HeaderRegexp("Header-1", "value-3") ->
		"https://www.example.org"`,
		&Route{
			HeaderRegexps: map[string][]string{
				"Header-0": {"value-0", "value-1"},
				"Header-1": {"value-2", "value-3"}},
			Backend: "https://www.example.org"},
		false,
	}, {
		"comment as last token",
		"route: Any() -> <shunt>; // some comment",
		&Route{Id: "route", Shunt: true},
		false,
	}, {
		"catch all",
		`* -> "https://www.example.org"`,
		&Route{Backend: "https://www.example.org"},
		false,
	}, {
		"custom predicate",
		`Custom1(3.14, "test value") && Custom2() -> "https://www.example.org"`,
		&Route{
			Predicates: []*Predicate{
				&Predicate{"Custom1", []interface{}{float64(3.14), "test value"}},
				&Predicate{"Custom2", nil}},
			Backend: "https://www.example.org"},
		false,
	}, {
		"double path predicates",
		`Path("/one") && Path("/two") -> "https://www.example.org"`,
		nil,
		true,
	}, {
		"double method predicates",
		`Method("HEAD") && Method("GET") -> "https://www.example.org"`,
		nil,
		true,
	}} {
		stringMapKeys := func(m map[string]string) []string {
			keys := make([]string, 0, len(m))
			for k, _ := range m {
				keys = append(keys, k)
			}

			return keys
		}

		stringsMapKeys := func(m map[string][]string) []string {
			keys := make([]string, 0, len(m))
			for k, _ := range m {
				keys = append(keys, k)
			}

			return keys
		}

		checkItemsT := func(submessage string, l, lExp int, checkItem func(i int) bool) bool {
			return checkItems(t, ti.msg+" "+submessage, l, lExp, checkItem)
		}

		checkStrings := func(submessage string, s, sExp []string) bool {
			return checkItemsT(submessage, len(s), len(sExp), func(i int) bool {
				return s[i] == sExp[i]
			})
		}

		checkStringMap := func(submessage string, m, mExp map[string]string) bool {
			keys := stringMapKeys(m)
			return checkItemsT(submessage, len(m), len(mExp), func(i int) bool {
				return m[keys[i]] == mExp[keys[i]]
			})
		}

		checkStringsMap := func(submessage string, m, mExp map[string][]string) bool {
			keys := stringsMapKeys(m)
			return checkItemsT(submessage, len(m), len(mExp), func(i int) bool {
				return checkItemsT(submessage, len(m[keys[i]]), len(mExp[keys[i]]), func(j int) bool {
					return m[keys[i]][j] == mExp[keys[i]][j]
				})
			})
		}

		routes, err := Parse(ti.expression)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
			return
		}

		if ti.err {
			return
		}

		r := routes[0]

		if r.Id != ti.check.Id {
			t.Error(ti.msg, "id", r.Id, ti.check.Id)
			return
		}

		if r.Path != ti.check.Path {
			t.Error(ti.msg, "path", r.Path, ti.check.Path)
			return
		}

		if !checkStrings("host", r.HostRegexps, ti.check.HostRegexps) {
			return
		}

		if !checkStrings("path regexp", r.PathRegexps, ti.check.PathRegexps) {
			return
		}

		if r.Method != ti.check.Method {
			t.Error(ti.msg, "method", r.Method, ti.check.Method)
			return
		}

		if !checkStringMap("headers", r.Headers, ti.check.Headers) {
			return
		}

		if !checkStringsMap("header regexps", r.HeaderRegexps, ti.check.HeaderRegexps) {
			return
		}

		if !checkItemsT("custom predicates",
			len(r.Predicates),
			len(ti.check.Predicates),
			func(i int) bool {
				return r.Predicates[i].Name == ti.check.Predicates[i].Name &&
					checkItemsT("custom predicate args",
						len(r.Predicates[i].Args),
						len(ti.check.Predicates[i].Args),
						func(j int) bool {
							return r.Predicates[i].Args[j] == ti.check.Predicates[i].Args[j]
						})
			}) {
			return
		}

		if !checkFilters(t, ti.msg, r.Filters, ti.check.Filters) {
			return
		}

		if r.Shunt != ti.check.Shunt {
			t.Error(ti.msg, "shunt", r.Shunt, ti.check.Shunt)
		}

		if r.Backend != ti.check.Backend {
			t.Error(ti.msg, "backend", r.Backend, ti.check.Backend)
		}
	}
}

func TestParseFilters(t *testing.T) {
	for _, ti := range []struct {
		msg        string
		expression string
		check      []*Filter
		err        bool
	}{{
		"empty",
		" \t",
		nil,
		false,
	}, {
		"error",
		"trallala",
		nil,
		true,
	}, {
		"success",
		`filter1(3.14) -> filter2("key", 42)`,
		[]*Filter{{Name: "filter1", Args: []interface{}{3.14}}, {Name: "filter2", Args: []interface{}{"key", float64(42)}}},
		false,
	}} {
		fs, err := ParseFilters(ti.expression)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
			return
		}

		checkFilters(t, ti.msg, fs, ti.check)
	}
}

func TestRouteJSON(t *testing.T) {
	for _, item := range []struct {
		route  *Route
		string string
	}{{
		&Route{},
		`{"id":"","backend":"","predicates":[],"filters":[]}` + "\n",
	}, {
		&Route{
			Filters:    []*Filter{{"xsrf", nil}},
			Predicates: []*Predicate{{"Test", nil}},
		},
		`{"id":"","backend":"","predicates":[{"name":"Test","args":[]}],"filters":[{"name":"xsrf","args":[]}]}` + "\n",
	}, {
		&Route{Method: "GET", Backend: "https://www.example.org"},
		`{"id":"","backend":"https://www.example.org","predicates":[{"name":"Method","args":["GET"]}],"filters":[]}` + "\n",
	}, {
		&Route{Method: "GET", Shunt: true},
		`{"id":"","backend":"<shunt>","predicates":[{"name":"Method","args":["GET"]}],"filters":[]}` + "\n",
	}, {
		&Route{
			Method:      "PUT",
			Path:        `/some/"/path`,
			HostRegexps: []string{"h-expression", "slash/h-expression"},
			PathRegexps: []string{"p-expression", "slash/p-expression"},
			Headers: map[string]string{
				`ap"key`: `ap"value`},
			HeaderRegexps: map[string][]string{
				`ap"key`: []string{"slash/value0", "slash/value1"}},
			Predicates: []*Predicate{{"Test", []interface{}{3.14, "hello"}}},
			Filters: []*Filter{
				{"filter0", []interface{}{float64(3.1415), "argvalue"}},
				{"filter1", []interface{}{float64(-42), `ap"argvalue`}}},
			Shunt:   false,
			Backend: "https://www.example.org"},
		`{` +
			`"id":"",` +
			`"backend":"https://www.example.org",` +
			`"predicates":[` +
			`{"name":"Method","args":["PUT"]}` +
			`,{"name":"Path","args":["/some/\"/path"]}` +
			`,{"name":"HostRegexp","args":["h-expression"]}` +
			`,{"name":"HostRegexp","args":["slash/h-expression"]}` +
			`,{"name":"PathRegexp","args":["p-expression"]}` +
			`,{"name":"PathRegexp","args":["slash/p-expression"]}` +
			`,{"name":"Header","args":["ap\"key","ap\"value"]}` +
			`,{"name":"HeaderRegexp","args":["ap\"key","slash/value0"]}` +
			`,{"name":"HeaderRegexp","args":["ap\"key","slash/value1"]}` +
			`,{"name":"Test","args":[3.14,"hello"]}` +
			`],` +
			`"filters":[` +
			`{"name":"filter0","args":[3.1415,"argvalue"]}` +
			`,{"name":"filter1","args":[-42,"ap\"argvalue"]}` +
			`]` +
			`}` + "\n",
	}} {
		bytes, err := item.route.MarshalJSON()
		if err != nil {
			t.Error(err)
		}
		rstring := string(bytes[:])
		if rstring != item.string {
			t.Errorf("Wrong output:\n  %s\nexpected:\n  %s", rstring, item.string)
		}
	}
}
