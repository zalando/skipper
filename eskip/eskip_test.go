package eskip

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

var benchmarkRoutes10k = strings.Repeat(`xxxx_xx__xxxxx__xxx_xxxxxxxx_xxxxxxxxxx_xxxxxxx_xxxxxxx_xxxxxxx_xxxxx__xxx__40_0:
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

func TestParse(t *testing.T) {
	for _, ti := range []struct {
		msg        string
		expression string
		check      []*Route
		err        string
	}{{
		"loadbalancer endpoints same protocol",
		`* -> <roundRobin, "http://localhost:80", "fastcgi://localhost:80">`,
		nil,
		"loadbalancer endpoints cannot have mixed protocols",
	}, {
		"path predicate",
		`Path("/some/path") -> "https://www.example.org"`,
		[]*Route{{Path: "/some/path", Backend: "https://www.example.org"}},
		"",
	}, {
		"path regexp",
		`PathRegexp("^/some") && PathRegexp(/\/\w+Id$/) -> "https://www.example.org"`,
		[]*Route{{
			PathRegexps: []string{"^/some", "/\\w+Id$"},
			Backend:     "https://www.example.org"}},
		"",
	}, {
		"weight predicate",
		`Weight(50) -> "https://www.example.org"`,
		[]*Route{{
			Predicates: []*Predicate{
				{"Weight", []interface{}{float64(50)}},
			},
			Backend: "https://www.example.org",
		}},
		"",
	}, {
		"method predicate",
		`Method("HEAD") -> "https://www.example.org"`,
		[]*Route{{Method: "HEAD", Backend: "https://www.example.org"}},
		"",
	}, {
		"invalid method predicate",
		`Path("/endpoint") && Method("GET", "POST") -> "https://www.example.org"`,
		nil,
		`invalid route "": Method predicate expects 1 string argument`,
	}, {
		"invalid header predicate",
		`foo: Path("/endpoint") && Header("Foo") -> "https://www.example.org";`,
		nil,
		`invalid route "foo": Header predicate expects 2 string arguments`,
	}, {
		"host regexps",
		`Host(/^www[.]/) && Host(/[.]org$/) -> "https://www.example.org"`,
		[]*Route{{HostRegexps: []string{"^www[.]", "[.]org$"}, Backend: "https://www.example.org"}},
		"",
	}, {
		"headers",
		`Header("Header-0", "value-0") &&
		Header("Header-1", "value-1") ->
		"https://www.example.org"`,
		[]*Route{{
			Headers: map[string]string{"Header-0": "value-0", "Header-1": "value-1"},
			Backend: "https://www.example.org"}},
		"",
	}, {
		"header regexps",
		`HeaderRegexp("Header-0", "value-0") &&
		HeaderRegexp("Header-0", "value-1") &&
		HeaderRegexp("Header-1", "value-2") &&
		HeaderRegexp("Header-1", "value-3") ->
		"https://www.example.org"`,
		[]*Route{{
			HeaderRegexps: map[string][]string{
				"Header-0": {"value-0", "value-1"},
				"Header-1": {"value-2", "value-3"}},
			Backend: "https://www.example.org"}},
		"",
	}, {
		"comment as last token",
		"route: Any() -> <shunt>; // some comment",
		[]*Route{{Id: "route", BackendType: ShuntBackend, Shunt: true}},
		"",
	}, {
		"catch all",
		`* -> "https://www.example.org"`,
		[]*Route{{Backend: "https://www.example.org"}},
		"",
	}, {
		"custom predicate",
		`Custom1(3.14, "test value") && Custom2() -> "https://www.example.org"`,
		[]*Route{{
			Predicates: []*Predicate{
				{"Custom1", []interface{}{float64(3.14), "test value"}},
				{"Custom2", nil}},
			Backend: "https://www.example.org"}},
		"",
	}, {
		"double path predicates",
		`Path("/one") && Path("/two") -> "https://www.example.org"`,
		nil,
		// TODO: should it be "duplicate path predicate"?
		"duplicate path tree predicate",
	}, {
		"double method predicates",
		`Method("HEAD") && Method("GET") -> "https://www.example.org"`,
		nil,
		"duplicate method predicate",
	}, {
		"shunt",
		`* -> setRequestHeader("X-Foo", "bar") -> <shunt>`,
		[]*Route{{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: ShuntBackend,
			Shunt:       true,
		}},
		"",
	}, {
		"loopback",
		`* -> setRequestHeader("X-Foo", "bar") -> <loopback>`,
		[]*Route{{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: LoopBackend,
		}},
		"",
	}, {
		"dynamic",
		`* -> setRequestHeader("X-Foo", "bar") -> <dynamic>`,
		[]*Route{{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: DynamicBackend,
		}},
		"",
	}, {
		"forward",
		`* -> setRequestHeader("X-Foo", "bar") -> <forward>`,
		[]*Route{{
			Filters: []*Filter{
				{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}},
			},
			BackendType: ForwardBackend,
		}},
		"",
	}, {
		"multiple routes",
		`r1: Path("/foo") -> <shunt>; r2: Path("/bar") -> "https://www.example.org"`,
		[]*Route{
			{Id: "r1", Path: "/foo", BackendType: ShuntBackend, Shunt: true},
			{Id: "r2", Path: "/bar", Backend: "https://www.example.org"},
		},
		"",
	}, {
		"syntax error with id",
		`fooId: * -> #`,
		nil,
		"parse failed after token ->, last route id: fooId, position 12: syntax error",
	}, {
		"syntax error multiple routes",
		`r1: Path("/foo") -> <shunt>; r2: Path("/bar") -> #`,
		nil,
		"parse failed after token ->, last route id: r2, position 49: syntax error",
	}, {
		"syntax without id",
		`* -> #`,
		nil,
		"parse failed after token ->, position 5: syntax error",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			routes, err := Parse(ti.expression)

			if ti.err != "" {
				assert.EqualError(t, err, ti.err)
				assert.Nil(t, routes)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, ti.check, routes)
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
		err        string
	}{{
		"empty",
		" \t",
		nil,
		"",
	}, {
		"error",
		"trallala",
		nil,
		"parse failed after token trallala, position 8: syntax error",
	}, {
		"error 2",
		"foo-bar",
		nil,
		"parse failed after token foo, position 3: syntax error",
	}, {
		"success",
		`filter1(3.14) -> filter2("key", 42)`,
		[]*Filter{{Name: "filter1", Args: []interface{}{3.14}}, {Name: "filter2", Args: []interface{}{"key", float64(42)}}},
		"",
	}, {
		"a comment produces nil filters without error",
		"// a comment",
		nil,
		"",
	}, {
		"a trailing comment is ignored",
		`filter1(3.14) -> filter2("key", 42) // a trailing comment`,
		[]*Filter{{Name: "filter1", Args: []interface{}{3.14}}, {Name: "filter2", Args: []interface{}{"key", float64(42)}}},
		"",
	}, {
		"a comment before is ignored",
		`// a comment on a separate line
		filter1(3.14) -> filter2("key", 42)`,
		[]*Filter{{Name: "filter1", Args: []interface{}{3.14}}, {Name: "filter2", Args: []interface{}{"key", float64(42)}}},
		"",
	}, {
		"a comment after is ignored",
		`filter1(3.14) -> filter2("key", 42)
		// a comment on a separate line`,
		[]*Filter{{Name: "filter1", Args: []interface{}{3.14}}, {Name: "filter2", Args: []interface{}{"key", float64(42)}}},
		"",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			fs, err := ParseFilters(ti.expression)

			if ti.err != "" {
				assert.EqualError(t, err, ti.err)
				assert.Nil(t, fs)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, ti.check, fs)
			}
		})
	}
}

func TestParsePredicates(t *testing.T) {
	for _, test := range []struct {
		title    string
		input    string
		expected []*Predicate
		err      string
	}{{
		title: "empty",
	}, {
		title: "invalid",
		input: `not predicates`,
		err:   "parse failed after token predicates, position 14: syntax error",
	}, {
		title: "invalid",
		input: `Header#`,
		err:   "parse failed after token Header, position 6: syntax error",
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
	}, {
		"a comment produces nil predicates without error",
		"// a comment",
		nil,
		"",
	}, {
		title: "a trailing comment is ignored",
		input: `Foo("bar") && Baz("qux") // a trailing comment`,
		expected: []*Predicate{
			{Name: "Foo", Args: []interface{}{"bar"}},
			{Name: "Baz", Args: []interface{}{"qux"}},
		},
	}, {
		title: "a comment before is ignored",
		input: `// a comment on a separate line
		Foo("bar") && Baz("qux")`,
		expected: []*Predicate{
			{Name: "Foo", Args: []interface{}{"bar"}},
			{Name: "Baz", Args: []interface{}{"qux"}},
		},
	}, {
		title: "a comment after is ignored",
		input: `Foo("bar") && Baz("qux")
		// a comment on a separate line`,
		expected: []*Predicate{
			{Name: "Foo", Args: []interface{}{"bar"}},
			{Name: "Baz", Args: []interface{}{"qux"}},
		},
	}} {
		t.Run(test.title, func(t *testing.T) {
			ps, err := ParsePredicates(test.input)

			if test.err != "" {
				assert.EqualError(t, err, test.err)
				assert.Nil(t, ps)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, ps)
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
	_, err := Parse(benchmarkRoutes10k)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(len(benchmarkRoutes10k)), "bytes/op")

	for i := 0; i < b.N; i++ {
		_, _ = Parse(benchmarkRoutes10k)
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
