package eskip

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func sortNameArgList(o interface{}) {
	l, ok := o.([]interface{})
	if !ok {
		return
	}

	sort.Slice(l, func(i, j int) bool {
		ii := l[i].(map[string]interface{})
		ij := l[j].(map[string]interface{})
		ni, oki := ii["name"].(string)
		nj, okj := ij["name"].(string)
		if !oki || !okj {
			return oki
		}

		if ni != nj {
			return ni < nj
		}

		ai, _ := ii["args"].([]interface{})
		aj, _ := ii["args"].([]interface{})
		return len(ai) < len(aj)
	})
}

func sortFiltersPredicates(j interface{}) {
	if l, ok := j.([]interface{}); ok {
		// multiple routes
		for _, li := range l {
			sortFiltersPredicates(li)
		}

		return
	}

	r := j.(map[string]interface{})
	sortNameArgList(r["predicates"])
	sortNameArgList(r["filters"])
}

func jsonCmp(a, b []byte) (string, bool) {
	var aj interface{}
	if err := json.Unmarshal(a, &aj); err != nil {
		return err.Error(), false
	}

	var bj interface{}
	if err := json.Unmarshal(b, &bj); err != nil {
		return err.Error(), false
	}

	sortFiltersPredicates(aj)
	sortFiltersPredicates(bj)
	d := cmp.Diff(aj, bj)
	return d, d == ""
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes interface{} // one or more
		json   string
	}{
		{
			"empty fields",
			[]*Route{{BackendType: LoopBackend}},
			`[{"backend":{"type":"loopback"}}]`,
		},
		{
			"network backend",
			[]*Route{{Id: "foo", BackendType: NetworkBackend, Backend: "127.0.0.1"}},
			`[{"id":"foo","backend":{"type":"network","address":"127.0.0.1"}}]`,
		},
		{
			"loop backend",
			[]*Route{{Id: "bar", BackendType: LoopBackend}},
			`[{"id":"bar","backend":{"type":"loopback"}}]`,
		},
		{
			"lb backend",
			[]*Route{{Id: "beef", BackendType: LBBackend, LBAlgorithm: "yolo", LBEndpoints: []*LBEndpoint{{Address: "localhost"}}}},
			`[{"id":"beef","backend":{"type":"lb","algorithm":"yolo","endpoints":["localhost"]}}]`,
		},
		{
			"shunt backend",
			[]*Route{{Id: "shunty", BackendType: ShuntBackend}},
			`[{"id":"shunty","backend":{"type":"shunt"}}]`,
		},
		{
			"predicates and filters",
			[]*Route{
				{
					Id: "predfilter",
					Predicates: []*Predicate{
						{Name: "Method", Args: []interface{}{"GET"}},
					},
					Filters: []*Filter{
						{Name: "setPath", Args: []interface{}{"/foo"}},
					},
					Backend: "127.0.0.1",
				},
			},
			`[
				{
					"id":"predfilter",
					"backend":{"type":"network","address":"127.0.0.1"},
					"predicates":[{"name":"Method","args":["GET"]}],
					"filters":[{"name":"setPath","args":["/foo"]}]
				}
			]`,
		},
		{
			"shunt, field",
			[]*Route{{Id: "sh", Shunt: true}},
			`[{"id":"sh","backend":{"type":"shunt"}}]`,
		},
		{
			"network backend",
			[]*Route{{Id: "network", Backend: "127.0.0.1"}},
			`[{"id":"network","backend":{"type":"network","address":"127.0.0.1"}}]`,
		},
		{
			"predicates",
			[]*Route{{Id: "oldschool", Path: "/foo", Headers: map[string]string{"Bar": "baz"}, Backend: "127.0.0.1"}},
			`[
				{
					"id":"oldschool",
					"backend":{
						"type":"network",
						"address":"127.0.0.1"
					},
					"predicates":[
						{"name":"Header","args":["Bar", "baz"]},
						{"name":"Path","args":["/foo"]}
					]
				}
			]`,
		},
		{
			"empty",
			&Route{},
			`{}`,
		},
		{
			"basic, no backend",
			&Route{
				Filters:    []*Filter{{"xsrf", nil}},
				Predicates: []*Predicate{{"Test", nil}},
			},
			`{
				"predicates": [{
					"name": "Test"
				}],
				"filters": [{
					"name": "xsrf"
				}]
			}`,
		},
		{
			"basic, with backend",
			&Route{Method: "GET", Backend: "https://www.example.org"},
			`{
				"backend": {"type": "network", "address": "https://www.example.org"},
				"predicates": [{
					"name": "Method",
					"args": ["GET"]
				}]
			}`,
		},
		{
			"shunt",
			&Route{Method: "GET", Shunt: true},
			`{
				"backend": {"type": "shunt"},
				"predicates": [{
					"name": "Method",
					"args": ["GET"]
				}]
			}`,
		},
		{
			"shunt, via backend type and legacy",
			&Route{Method: "GET", Shunt: true, BackendType: ShuntBackend},
			`{
				"backend": {"type": "shunt"},
				"predicates": [{
					"name": "Method",
					"args":["GET"]
				}]
			}`,
		},
		{
			"shunt, via backend type",
			&Route{Method: "GET", BackendType: ShuntBackend},
			`{
				"backend": {"type": "shunt"},
				"predicates": [{
					"name": "Method",
					"args": ["GET"]
				}]
			}`,
		},
		{
			"loop backend",
			&Route{Method: "GET", BackendType: LoopBackend},
			`{
				"backend": {"type": "loopback"},
				"predicates": [{
					"name": "Method",
					"args": ["GET"]
				}]
			}`,
		},
		{
			"dynamic backend",
			&Route{Method: "GET", BackendType: DynamicBackend},
			`{
				"backend": {"type": "dynamic"},
				"predicates": [{
					"name": "Method",
					"args": ["GET"]
				}]
			}`,
		},
		{
			"complex, legacy fields",
			&Route{
				Method:      "PUT",
				Path:        `/some/"/path`,
				HostRegexps: []string{"h-expression", "slash/h-expression"},
				PathRegexps: []string{"p-expression", "slash/p-expression"},
				Headers: map[string]string{
					`ap"key`: `ap"value`},
				HeaderRegexps: map[string][]string{
					`ap"key`: {"slash/value0", "slash/value1"}},
				Predicates: []*Predicate{{"Test", []interface{}{3.14, "hello"}}},
				Filters: []*Filter{
					{"filter0", []interface{}{float64(3.1415), "argvalue"}},
					{"filter1", []interface{}{float64(-42), `ap"argvalue`}}},
				Shunt:   false,
				Backend: "https://www.example.org"},
			`{
				"backend": {"type": "network", "address": "https://www.example.org"},
				"predicates": [
					{"name": "Method", "args": ["PUT"]},
					{"name": "Path", "args": ["/some/\"/path"]},
					{"name": "Host", "args": ["h-expression"]},
					{"name": "Host", "args": ["slash/h-expression"]},
					{"name": "PathRegexp", "args": ["p-expression"]},
					{"name": "PathRegexp", "args": ["slash/p-expression"]},
					{"name": "Header", "args": ["ap\"key","ap\"value"]},
					{"name": "HeaderRegexp", "args": ["ap\"key","slash/value0"]},
					{"name": "HeaderRegexp", "args": ["ap\"key","slash/value1"]},
					{"name": "Test", "args": [3.14,"hello"]}
				],
				"filters":[
					{"name": "filter0", "args": [3.1415,"argvalue"]},
					{"name": "filter1", "args": [-42,"ap\"argvalue"]}
				]
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotJSON, err := json.Marshal(tc.routes)
			if err != nil {
				t.Fatalf("cannot marshal %v, got error: %v", tc.routes, err)
			}

			if diff, eq := jsonCmp([]byte(tc.json), gotJSON); !eq {
				t.Fatalf("Wrong output:\n  %s", diff)
			}

			if _, ok := tc.routes.(*Route); ok {
				var r *Route
				if err := json.Unmarshal(gotJSON, &r); err != nil {
					t.Fatalf("cannot unmarshal %s, got error: %v", gotJSON, err)
				}

				if !Eq(r, tc.routes.(*Route)) {
					t.Fatalf("unmarshal result incorrect, %v != %v", r, tc.routes)
				}
			} else if _, ok := tc.routes.([]*Route); ok {
				var r []*Route
				if err := json.Unmarshal(gotJSON, &r); err != nil {
					t.Fatalf("cannot unmarshal %s, got error: %v", gotJSON, err)
				}

				if !EqLists(r, tc.routes.([]*Route)) {
					t.Fatalf("unmarshal result incorrect, %v != %v", r, tc.routes)
				}
			} else {
				t.Fatal("invalid input routes")
			}

		})
	}
}

func TestInvalidJSON(t *testing.T) {
	for name, input := range map[string]string{
		"invalid":              "{\\",
		"invalid backend type": `{"backend": {"type": "foo"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var r *Route
			if err := json.Unmarshal([]byte(input), &r); err == nil {
				t.Fatalf("Failed to fail: %s.", name)
			}
		})
	}
}

type testRouteContainer struct {
	Routes []*Route `json:"routes"`
}

func BenchmarkJsonUnmarshal(b *testing.B) {
	content, err := json.Marshal(testRouteContainer{Routes: MustParse(benchmarkRoutes10k)})
	if err != nil {
		b.Fatal(err)
	}

	out := testRouteContainer{}
	if err := json.Unmarshal(content, &out); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = json.Unmarshal(content, &out)
	}
}
