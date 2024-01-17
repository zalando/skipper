package eskip

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func checkItems(t *testing.T, message string, l, lenExpected int, checkItem func(int) bool) bool {
	t.Helper()
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
	t.Helper()
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
		"loadbalancer endpoints same protocol",
		`* -> <roundRobin, "http://localhost:80", "fastcgi://localhost:80">`,
		nil,
		true,
	}, {
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
		"weight predicate",
		`Weight(50) -> "https://www.example.org"`,
		&Route{
			Predicates: []*Predicate{
				{"Weight", []interface{}{float64(50)}},
			},
			Backend: "https://www.example.org",
		},
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
		&Route{Id: "route", BackendType: ShuntBackend, Shunt: true},
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
				{"Custom1", []interface{}{float64(3.14), "test value"}},
				{"Custom2", nil}},
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
	}, {
		"shunt",
		`* -> setRequestHeader("X-Foo", "bar") -> <shunt>`,
		&Route{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: ShuntBackend,
			Shunt:       true,
		},
		false,
	}, {
		"loopback",
		`* -> setRequestHeader("X-Foo", "bar") -> <loopback>`,
		&Route{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: LoopBackend,
		},
		false,
	}, {
		"dynamic",
		`* -> setRequestHeader("X-Foo", "bar") -> <dynamic>`,
		&Route{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: DynamicBackend,
		},
		false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			stringMapKeys := func(m map[string]string) []string {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}

				return keys
			}

			stringsMapKeys := func(m map[string][]string) []string {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}

				return keys
			}

			checkItemsT := func(submessage string, l, lExp int, checkItem func(i int) bool) bool {
				return checkItems(t, submessage, l, lExp, checkItem)
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
				t.Error("failure case", err, ti.err)
				return
			}

			if ti.err {
				return
			}

			r := routes[0]

			if r.Id != ti.check.Id {
				t.Error("id", r.Id, ti.check.Id)
				return
			}

			if r.Path != ti.check.Path {
				t.Error("path", r.Path, ti.check.Path)
				return
			}

			if !checkStrings("host", r.HostRegexps, ti.check.HostRegexps) {
				return
			}

			if !checkStrings("path regexp", r.PathRegexps, ti.check.PathRegexps) {
				return
			}

			if r.Method != ti.check.Method {
				t.Error("method", r.Method, ti.check.Method)
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

			if !checkFilters(t, "", r.Filters, ti.check.Filters) {
				return
			}

			if r.BackendType != ti.check.BackendType {
				t.Error("invalid backend type", r.BackendType, ti.check.BackendType)
			}

			if r.Shunt != ti.check.Shunt {
				t.Error("shunt", r.Shunt, ti.check.Shunt)
			}

			if r.Shunt && r.BackendType != ShuntBackend || !r.Shunt && r.BackendType == ShuntBackend {
				t.Error("shunt, deprecated and new form are not sync")
			}

			if r.BackendType == LoopBackend && r.Shunt {
				t.Error("shunt set for loopback route")
			}

			if r.Backend != ti.check.Backend {
				t.Error("backend", r.Backend, ti.check.Backend)
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	const (
		valid   = "* -> <shunt>"
		invalid = "this is an invalid route definition"
	)

	expected, err := Parse(valid)
	if err != nil {
		t.Fatal(err)
	}

	r := MustParse(valid)
	if !cmp.Equal(r, expected) {
		t.Errorf("Invalid parse result: %s", cmp.Diff(r, expected))
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic parsing %q", invalid)
		}
	}()

	_ = MustParse(invalid)
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
		t.Run(ti.msg, func(t *testing.T) {
			fs, err := ParseFilters(ti.expression)
			if err == nil && ti.err || err != nil && !ti.err {
				t.Error(ti.msg, "failure case", err, ti.err)
				return
			}

			checkFilters(t, ti.msg, fs, ti.check)
		})
	}
}

func TestPredicateParsing(t *testing.T) {
	for _, test := range []struct {
		title    string
		input    string
		expected []*Predicate
		fail     bool
	}{{
		title: "empty",
	}, {
		title: "invalid",
		input: "not predicates",
		fail:  true,
	}, {
		title:    "single predicate",
		input:    `Foo("bar")`,
		expected: []*Predicate{{Name: "Foo", Args: []interface{}{"bar"}}},
	}, {
		title: "multiple predicates",
		input: `Foo("bar") && Baz("qux") && Quux("quuz")`,
		expected: []*Predicate{
			{Name: "Foo", Args: []interface{}{"bar"}},
			{Name: "Baz", Args: []interface{}{"qux"}},
			{Name: "Quux", Args: []interface{}{"quuz"}},
		},
	}, {
		title: "star notation",
		input: `*`,
	}, {
		title:    "comment fuzz 1",
		input:    `///`,
		expected: nil,
	}, {
		title:    "comment fuzz 2", // "\x2f\x2f..." == "//..."
		input:    "\x2f\x2f\x00\x00\x00\xe6\xfe\x00\x00\x2f\x00\x00\x00\x00\x00\x00\x00\xe6\xfe\x00\x00\x2f\x00\x00\x00\x00",
		expected: nil,
	}} {
		t.Run(test.title, func(t *testing.T) {
			p, err := ParsePredicates(test.input)

			if err == nil && test.fail {
				t.Fatalf("failed to fail: %#v", p)
			} else if err != nil && !test.fail {
				t.Fatal(err)
			}

			if !cmp.Equal(p, test.expected) {
				t.Errorf("invalid parse result:\n%s", cmp.Diff(p, test.expected))
			}
		})
	}
}

func TestClone(t *testing.T) {
	r := &Route{
		Id:            "foo",
		Path:          "/bar",
		HostRegexps:   []string{"[.]example[.]org$", "^www[.]"},
		PathRegexps:   []string{"^/", "bar$"},
		Method:        "GET",
		Headers:       map[string]string{"X-Foo": "bar"},
		HeaderRegexps: map[string][]string{"X-Bar": {"baz", "qux"}},
		Predicates:    []*Predicate{{Name: "Foo", Args: []interface{}{"bar", "baz"}}},
		Filters:       []*Filter{{Name: "foo", Args: []interface{}{42, 84}}},
		Backend:       "https://www2.example.org",
	}

	c := r.Copy()
	if c == r {
		t.Error("routes are of the same instance")
	}

	if !reflect.DeepEqual(c, r) {
		t.Error("failed to clone all the fields")
	}
}

func TestDefaultFiltersDo(t *testing.T) {
	input, err := Parse(`r1: Host("example.org") -> inlineContent("OK") -> "http://127.0.0.1:9001"`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	filter, err := ParseFilters("status(418)")
	if err != nil {
		t.Errorf("Failed to parse filter: %v", err)
	}
	filter2, err := ParseFilters("status(419)")
	if err != nil {
		t.Errorf("Failed to parse filter: %v", err)
	}

	outputPrepend, err := Parse(`r1: Host("example.org") -> status(418) -> inlineContent("OK") -> "http://127.0.0.1:9001"`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	outputAppend, err := Parse(`r1: Host("example.org")  -> inlineContent("OK") -> status(418) -> "http://127.0.0.1:9001"`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	outputPrependAppend, err := Parse(`r1: Host("example.org") -> status(419) -> inlineContent("OK") -> status(418) -> "http://127.0.0.1:9001"`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	outputPrependAppend2, err := Parse(`r1: Host("example.org") -> status(419) -> status(418) -> inlineContent("OK") -> status(418) -> status(419) -> "http://127.0.0.1:9001"`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	for _, tt := range []struct {
		name   string
		df     *DefaultFilters
		routes []*Route
		want   []*Route
	}{
		{
			name:   "test no default filters should not change anything",
			df:     &DefaultFilters{},
			routes: input,
			want:   input,
		}, {
			name: "test default filters, that are nil should not change anything",
			df: &DefaultFilters{
				Append:  nil,
				Prepend: nil,
			},
			routes: input,
			want:   input,
		}, {
			name: "test default filters, that prepend should prepend a filter",
			df: &DefaultFilters{
				Append:  nil,
				Prepend: filter,
			},
			routes: input,
			want:   outputPrepend,
		}, {
			name: "test default filters, that append should append a filter",
			df: &DefaultFilters{
				Append:  filter,
				Prepend: nil,
			},
			routes: input,
			want:   outputAppend,
		}, {
			name: "test default filters, that append and prepend should append and prepend a filter",
			df: &DefaultFilters{
				Append:  filter,
				Prepend: filter2,
			},
			routes: input,
			want:   outputPrependAppend,
		}, {
			name: "test default filters, that append and prepend should append and prepend a filters",
			df: &DefaultFilters{
				Append:  append(filter, filter2...),
				Prepend: append(filter2, filter...),
			},
			routes: input,
			want:   outputPrependAppend2,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.df.Do(tt.routes); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Want %v, got %v", tt.want, got)
			}
		})
	}

}

func TestDefaultFiltersDoCorrectPrependFilters(t *testing.T) {
	filters, err := ParseFilters("status(1) -> status(2) -> status(3)")
	if err != nil {
		t.Errorf("Failed to parse filter: %v", err)
	}

	routes, err := Parse(`
r1: Method("GET") -> inlineContent("r1") -> <shunt>;
r2: Method("POST") -> inlineContent("r2") -> <shunt>;
`)
	if err != nil {
		t.Errorf("Failed to parse route: %v", err)
	}

	df := &DefaultFilters{Prepend: filters}

	finalRoutes := df.Do(routes)
	for _, route := range finalRoutes {
		if route.Id != route.Filters[len(route.Filters)-1].Args[0].(string) {
			t.Errorf("Route %v has incorrect filters: %v", route.Id, route.Filters[3])
		}
	}
}

func TestEditorPreProcessor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		rep    *Editor
		routes string
		want   string
	}{
		{
			name:   "test empty Editor should not change the routes",
			rep:    &Editor{},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>`,
			want:   `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>`,
		},
		{
			name: "test no match should not change the routes",
			rep: &Editor{
				reg:  regexp.MustCompile("SourceFromLast[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>`,
			want:   `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>`,
		},
		{
			name: "test match should change the routes",
			rep: &Editor{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r1_filter: Source("1.2.3.4/26") -> status(201) -> <shunt>`,
			want:   `r1_filter: ClientIP("1.2.3.4/26") -> status(201) -> <shunt>`,
		},
		{
			name: "test multiple routes match should change the routes",
			rep: &Editor{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			r1_filter: Source("1.2.3.4/26") -> status(201) -> <shunt>;`,
			want: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			r1_filter: ClientIP("1.2.3.4/26") -> status(201) -> <shunt>`,
		},
		{
			name: "test match should change the routes with multiple params",
			rep: &Editor{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `rn_filter: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>`,
			want:   `rn_filter: ClientIP("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>`,
		},
		{
			name: "test multiple routes match should change the routes with multiple params",
			rep: &Editor{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			rn_filter: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
			want: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			rn_filter: ClientIP("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
		},
		{
			name: "test match should change the filter of a route",
			rep: &Editor{
				reg:  regexp.MustCompile("uniformRequestLatency[(](.*)[)]"),
				repl: "normalRequestLatency($1)",
			},
			routes: `r1_filter: Source("1.2.3.4/26") -> uniformRequestLatency("100ms", "10ms") -> status(201) -> <shunt>`,
			want:   `r1_filter: Source("1.2.3.4/26") -> normalRequestLatency("100ms", "10ms") -> status(201) -> <shunt>`,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := Parse(tt.routes)
			if err != nil {
				t.Errorf("Failed to parse route: %v", err)
			}
			want, err := Parse(tt.want)
			if err != nil {
				t.Errorf("Failed to parse route: %v", err)
			}
			got := tt.rep.Do(routes)

			if !EqLists(got, want) {
				t.Errorf("Failed to get routes %d == %d: \nwant: %v, \ngot: %v\n%s", len(want), len(got), want, got, cmp.Diff(want, got))
			}
		})
	}

}
func TestClonePreProcessor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		rep    *Clone
		routes string
		want   string
	}{
		{
			name:   "test empty Clone should not change the routes",
			rep:    &Clone{},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;`,
			want:   `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;`,
		},
		{
			name: "test match on builtin predicates",
			rep: &Clone{
				reg:  regexp.MustCompile("Host[(](.*)[)]"),
				repl: "HostAny($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;`,
			want: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			clone_r0: HostAny("www[.]example[.]org") -> status(201) -> <shunt>;`,
		},
		{
			name: "test no match should not change the routes",
			rep: &Clone{
				reg:  regexp.MustCompile("SourceFromLast[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;`,
			want:   `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;`,
		},
		{
			name: "test match should change the routes",
			rep: &Clone{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r1: Source("1.2.3.4/26") -> status(201) -> <shunt>;`,
			want: `r1: Source("1.2.3.4/26") -> status(201) -> <shunt>;
			clone_r1: ClientIP("1.2.3.4/26") -> status(201) -> <shunt>;`,
		},
		{
			name: "test multiple routes match should change the routes",
			rep: &Clone{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			r1: Source("1.2.3.4/26") -> status(201) -> <shunt>;`,

			want: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			r1: Source("1.2.3.4/26") -> status(201) -> <shunt>;
			clone_r1: ClientIP("1.2.3.4/26") -> status(201) -> <shunt>;`,
		},
		{
			name: "test match should change the routes with multiple params",
			rep: &Clone{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `rn: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
			want: `rn: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;
			clone_rn: ClientIP("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
		},
		{
			name: "test multiple routes match should change the routes with multiple params",
			rep: &Clone{
				reg:  regexp.MustCompile("Source[(](.*)[)]"),
				repl: "ClientIP($1)",
			},
			routes: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			rn: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
			want: `r0: Host("www[.]example[.]org") -> status(201) -> <shunt>;
			rn: Source("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;
			clone_rn: ClientIP("1.2.3.4/26", "10.5.5.0/24") -> status(201) -> <shunt>;`,
		},
		{
			name: "test match should change the filter of a route",
			rep: &Clone{
				reg:  regexp.MustCompile("uniformRequestLatency[(](.*)[)]"),
				repl: "normalRequestLatency($1)",
			},
			routes: `r1_filter: Source("1.2.3.4/26") -> uniformRequestLatency("100ms", "10ms") -> status(201) -> <shunt>;`,
			want: `r1_filter: Source("1.2.3.4/26") -> uniformRequestLatency("100ms", "10ms") -> status(201) -> <shunt>;
			clone_r1_filter: Source("1.2.3.4/26") -> normalRequestLatency("100ms", "10ms") -> status(201) -> <shunt>;`,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := Parse(tt.routes)
			if err != nil {
				t.Errorf("Failed to parse route: %v", err)
			}
			want, err := Parse(tt.want)
			if err != nil {
				t.Errorf("Failed to parse route: %v", err)
			}
			got := tt.rep.Do(routes)

			if !EqLists(got, want) {
				t.Errorf("Failed to get routes %d == %d: \nwant: %v, \ngot: %v\n%s", len(want), len(got), want, got, cmp.Diff(want, got))
			}
		})
	}

}

func TestPredicateString(t *testing.T) {
	for _, tt := range []struct {
		name      string
		predicate *Predicate
		want      string
	}{
		{
			name: "test one parameter",
			predicate: &Predicate{
				Name: "ClientIP",
				Args: []interface{}{
					"1.2.3.4/26",
				},
			},
			want: `ClientIP("1.2.3.4/26")`,
		},
		{
			name: "test two parameters",
			predicate: &Predicate{
				Name: "ClientIP",
				Args: []interface{}{
					"1.2.3.4/26",
					"10.2.3.4/22",
				},
			},
			want: `ClientIP("1.2.3.4/26", "10.2.3.4/22")`,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.predicate.String()
			if got != tt.want {
				t.Errorf("Failed to String(): Want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestFilterString(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter *Filter
		want   string
	}{
		{
			name: "test one parameter",
			filter: &Filter{
				Name: "setPath",
				Args: []interface{}{
					"/foo",
				},
			},
			want: `setPath("/foo")`,
		},
		{
			name: "test two parameters",
			filter: &Filter{
				Name: "uniformRequestLatency",
				Args: []interface{}{
					"100ms",
					"10ms",
				},
			},
			want: `uniformRequestLatency("100ms", "10ms")`,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.String()
			if got != tt.want {
				t.Errorf("Failed to String(): Want %v, got %v", tt.want, got)
			}
		})
	}
}

func BenchmarkParsePredicates(b *testing.B) {
	doc := `FooBarBazKeyValues(
		"https://example.org/foo0", "foobarbaz0",
		"https://example.org/foo1", "foobarbaz1",
		"https://example.org/foo2", "foobarbaz2",
		"https://example.org/foo3", "foobarbaz3",
		"https://example.org/foo4", "foobarbaz4",
		"https://example.org/foo5", "foobarbaz5",
		"https://example.org/foo6", "foobarbaz6",
		"https://example.org/foo7", "foobarbaz7",
		"https://example.org/foo8", "foobarbaz8",
		"https://example.org/foo9", "foobarbaz9")`

	_, err := ParsePredicates(doc)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParsePredicates(doc)
	}
}

func BenchmarkParse(b *testing.B) {
	doc := strings.Repeat(`xxxx_xx__xxxxx__xxx_xxxxxxxx_xxxxxxxxxx_xxxxxxx_xxxxxxx_xxxxxxx_xxxxx__xxx__40_0:
		Path("/xxxxxxxxx/:xxxxxxxx_xx/xxxxxxxx-xxxxxxxxxx-xxxxxxxxx")
		&& Host("^(xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx-xxxx-18[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?|xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx-xxxx-19[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?|xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx-xxxx-20[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?|xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx-xxxx-21[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?|xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?|xxxxxxxxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx[.]xxxxxxxxxxx[.]xxx[.]?(:[0-9]+)?)$")
		&& Host("^(xxx-xxxxxxxx-xxxxxxxxxx-xxxxxxx-xxxxxxx-xxxx-21[.]xxx-xxxx[.]xxxxx[.]xx[.]?(:[0-9]+)?)$")
		&& Weight(4)
		&& Method("GET")
		&& JWTPayloadAllKV("xxxxx://xxxxxxxx.xxxxxxx.xxx/xxxxx", "xxxxx")
		&& Header("X-Xxxxxxxxx-Xxxxx", "xxxxx")
		-> disableAccessLog(2, 3, 40, 500)
		-> fifo(1000, 100, "10s")
		-> apiUsageMonitoring("{\"xxx_xx\":\"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx\",\"xxxxxxxxxxx_xx\":\"xxx-xxxxxxxx-xxxxxxxxxx\",\"xxxx_xxxxxxxxx\":[\"/xxxxxxxxx/{xxxxxxxx_xx}/xxxxxxxx-xxxxxxxxxx\",\"/xxxxxxxxx/{xxxxxxxx_xx}/xxxxxxxx-xxxxxxxxxx-xxxxxxx\",\"/xxxxxxxxx/{xxxxxxxx_xx}/xxxxxxxx-xxxxxxxxxx-xxxxxxxxx\"]}")
		-> oauthTokeninfoAnyKV("xxxxx", "/xxxxxxxxx")
		-> unverifiedAuditLog("xxxxx://xxxxxxxx.xxxxxxx.xxx/xxxxxxx-xx")
		-> oauthTokeninfoAllScope("xxx")
		-> flowId("reuse")
		-> forwardToken("X-XxxxxXxxx-Xxxxxxx", "xxx", "xxxxx", "xxxxx")
		-> stateBagToTag("xxxx-xxxx", "xxxxxx.xxx")
		-> <powerOfRandomNChoices, "http://1.2.1.1:8080", "http://1.2.1.2:8080", "http://1.2.1.3:8080", "http://1.2.1.4:8080", "http://1.2.1.5:8080">;
	`, 10_000)

	_, err := Parse(doc)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Parse(doc)
	}
}

var stringSink string

func BenchmarkRouteString(b *testing.B) {
	doc := `
		Method("GET") &&
		Path("/foo") &&
		Host(/foo/) &&
		Host(/bar/) &&
		Host(/baz/) &&
		PathRegexp(/C/) &&
		PathRegexp(/B/) &&
		PathRegexp(/A/) &&
		Header("Foo", "Bar") &&
		Header("Bar", "Baz") &&
		Header("Qux", "Bar") &&
		HeaderRegexp("B", /3/) &&
		HeaderRegexp("B", /2/) &&
		HeaderRegexp("A", /1/) &&
		Foo("bar", "baz") &&
		True() -> <shunt>`

	rr, err := Parse(doc)
	if err != nil {
		b.Fatal(err)
	}
	r := rr[0]
	var s string

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s = r.String()
	}
	stringSink = s
}

func BenchmarkRouteStringNoRepeatedPredicates(b *testing.B) {
	doc := `
		Method("GET") &&
		Path("/foo") &&
		Host(/foo/) &&
		PathRegexp(/A/) &&
		Header("Foo", "Bar") &&
		HeaderRegexp("A", /1/) &&
		Foo("bar", "baz") &&
		True() -> <shunt>`

	rr, err := Parse(doc)
	if err != nil {
		b.Fatal(err)
	}
	r := rr[0]
	var s string

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s = r.String()
	}
	stringSink = s
}
