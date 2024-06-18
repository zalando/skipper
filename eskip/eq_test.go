package eskip

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEq(t *testing.T) {
	for _, test := range []struct {
		title  string
		routes []*Route
		expect bool
	}{{
		title:  "zero routes",
		expect: true,
	}, {
		title:  "single route",
		expect: true,
	}, {
		title:  "eq nil",
		routes: []*Route{nil, nil},
		expect: true,
	}, {
		title:  "one nil",
		routes: []*Route{nil, {}},
	}, {
		title: "eq non-canonical",
		routes: []*Route{{
			Shunt: true,
		}, {
			BackendType: ShuntBackend,
		}},
		expect: true,
	}, {
		title:  "non-eq id",
		routes: []*Route{{Id: "foo"}, {Id: "bar"}},
	}, {
		title:  "non-eq predicate count",
		routes: []*Route{{Predicates: []*Predicate{{}, {}}}, {Predicates: []*Predicate{{}}}},
	}, {
		title: "non-eq predicate name",
		routes: []*Route{
			{Predicates: []*Predicate{{Name: "Foo"}}},
			{Predicates: []*Predicate{{Name: "Bar"}}},
		},
	}, {
		title: "non-eq predicate arg count",
		routes: []*Route{
			{Predicates: []*Predicate{{Args: []interface{}{1, 2}}}},
			{Predicates: []*Predicate{{Args: []interface{}{1}}}},
		},
	}, {
		title: "non-eq predicate args",
		routes: []*Route{
			{Predicates: []*Predicate{{Args: []interface{}{1, 2}}}},
			{Predicates: []*Predicate{{Args: []interface{}{1, 3}}}},
		},
	}, {
		title:  "non-eq filter count",
		routes: []*Route{{Filters: []*Filter{{}, {}}}, {Filters: []*Filter{{}}}},
	}, {
		title: "non-eq filter name",
		routes: []*Route{
			{Filters: []*Filter{{Name: "Foo"}}},
			{Filters: []*Filter{{Name: "Bar"}}},
		},
	}, {
		title: "non-eq filter arg count",
		routes: []*Route{
			{Filters: []*Filter{{Args: []interface{}{1, 2}}}},
			{Filters: []*Filter{{Args: []interface{}{1}}}},
		},
	}, {
		title: "non-eq filter args",
		routes: []*Route{
			{Filters: []*Filter{{Args: []interface{}{1, 2}}}},
			{Filters: []*Filter{{Args: []interface{}{1, 3}}}},
		},
	}, {
		title: "non-eq backend type",
		routes: []*Route{
			{BackendType: ShuntBackend},
			{BackendType: LoopBackend},
		},
	}, {
		title: "non-eq backend address",
		routes: []*Route{
			{Backend: "https://one.example.org"},
			{Backend: "https://two.example.org"},
		},
	}, {
		title: "non-eq lb algorithm",
		routes: []*Route{
			{BackendType: LBBackend, LBAlgorithm: "roundRobin"},
			{BackendType: LBBackend, LBAlgorithm: "random"},
		},
	}, {
		title: "non-eq lb endpoint count",
		routes: []*Route{
			{BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}, {Address: "https://one.example.org"}}},
			{BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}}},
		},
	}, {
		title: "non-eq lb endpoints",
		routes: []*Route{
			{BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}}},
			{BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "https://two.example.org"}}},
		},
	}, {
		title: "all eq",
		routes: []*Route{{
			Id:          "foo",
			Predicates:  []*Predicate{{Name: "Foo", Args: []interface{}{1, 2}}},
			Filters:     []*Filter{{Name: "foo", Args: []interface{}{3, 4}}},
			BackendType: LBBackend,
			LBAlgorithm: "random",
			LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}, {Address: "https://two.example.org"}},
		}, {
			Id:          "foo",
			Predicates:  []*Predicate{{Name: "Foo", Args: []interface{}{1, 2}}},
			Filters:     []*Filter{{Name: "foo", Args: []interface{}{3, 4}}},
			BackendType: LBBackend,
			LBAlgorithm: "random",
			LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}, {Address: "https://two.example.org"}},
		}},
		expect: true,
	}, {
		title:  "one out of 3 non-eq",
		routes: []*Route{{Id: "foo"}, {Id: "foo"}, {Id: "bar"}},
	}, {
		title:  "3 eq",
		routes: []*Route{{Id: "foo"}, {Id: "foo"}, {Id: "foo"}},
		expect: true,
	}} {
		t.Run(test.title, func(t *testing.T) {
			if Eq(test.routes...) != test.expect {
				t.Error("failed to compare routes")
			}
		})
	}
}

func TestEqLists(t *testing.T) {
	for _, test := range []struct {
		title  string
		lists  [][]*Route
		expect bool
	}{{
		title:  "zero lists",
		expect: true,
	}, {
		title:  "one list",
		lists:  [][]*Route{{{Id: "foo"}, {Id: "bar"}}},
		expect: true,
	}, {
		title: "count non-eq",
		lists: [][]*Route{{{}, {}}, {{}}},
	}, {
		title: "has duplicate ID",
		lists: [][]*Route{
			{{Id: "foo"}, {Id: "bar"}, {Id: "foo"}},
			{{Id: "foo"}, {Id: "bar"}, {Id: "foo"}},
		},
	}, {
		title: "sorted",
		lists: [][]*Route{
			{{Id: "foo"}, {Id: "bar"}},
			{{Id: "bar"}, {Id: "foo"}},
		},
		expect: true,
	}, {
		title: "one out of 3 non-eq",
		lists: [][]*Route{
			{{Id: "foo"}, {Id: "bar"}},
			{{Id: "bar"}, {Id: "foo"}},
			{{Id: "foo"}, {Id: "baz"}},
		},
	}, {
		title: "3 eq",
		lists: [][]*Route{
			{{Id: "foo"}, {Id: "bar"}},
			{{Id: "bar"}, {Id: "foo"}},
			{{Id: "foo"}, {Id: "bar"}},
		},
		expect: true,
	}} {
		t.Run(test.title, func(t *testing.T) {
			if EqLists(test.lists...) != test.expect {
				t.Error("failed to compare lists of routes")
			}
		})
	}
}

func TestCanonical(t *testing.T) {
	for _, test := range []struct {
		title  string
		route  *Route
		expect *Route
	}{{
		title: "nil",
	}, {
		title:  "path",
		route:  &Route{Path: "/foo"},
		expect: &Route{Predicates: []*Predicate{{Name: "Path", Args: []interface{}{"/foo"}}}},
	}, {
		title: "path, from predicates",
		route: &Route{
			Path:       "/foo",
			Predicates: []*Predicate{{Name: "Path", Args: []interface{}{"/bar"}}},
		},
		expect: &Route{Predicates: []*Predicate{{Name: "Path", Args: []interface{}{"/bar"}}}},
	}, {
		title: "host regexps to predicates",
		route: &Route{
			HostRegexps: []string{"foo"},
			Predicates:  []*Predicate{{Name: "Host", Args: []interface{}{"bar"}}},
		},
		expect: &Route{
			Predicates: []*Predicate{
				{Name: "Host", Args: []interface{}{"bar"}},
				{Name: "Host", Args: []interface{}{"foo"}},
			},
		},
	}, {
		title: "path regexps to predicates",
		route: &Route{
			PathRegexps: []string{"foo"},
			Predicates:  []*Predicate{{Name: "PathRegexp", Args: []interface{}{"bar"}}},
		},
		expect: &Route{
			Predicates: []*Predicate{
				{Name: "PathRegexp", Args: []interface{}{"bar"}},
				{Name: "PathRegexp", Args: []interface{}{"foo"}},
			},
		},
	}, {
		title: "method to predicates",
		route: &Route{
			Method:     "GET",
			Predicates: []*Predicate{{Name: "Method", Args: []interface{}{"POST"}}},
		},
		expect: &Route{
			Predicates: []*Predicate{
				{Name: "Method", Args: []interface{}{"GET"}},
				{Name: "Method", Args: []interface{}{"POST"}},
			},
		},
	}, {
		title: "headers to predicates",
		route: &Route{
			Headers:    map[string]string{"X-Foo": "foo"},
			Predicates: []*Predicate{{Name: "Header", Args: []interface{}{"X-Bar", "bar"}}},
		},
		expect: &Route{
			Predicates: []*Predicate{
				{Name: "Header", Args: []interface{}{"X-Bar", "bar"}},
				{Name: "Header", Args: []interface{}{"X-Foo", "foo"}},
			},
		},
	}, {
		title: "header regexps to predicates",
		route: &Route{
			HeaderRegexps: map[string][]string{"X-Foo": {"foo"}},
			Predicates:    []*Predicate{{Name: "HeaderRegexp", Args: []interface{}{"X-Bar", "bar"}}},
		},
		expect: &Route{
			Predicates: []*Predicate{
				{Name: "HeaderRegexp", Args: []interface{}{"X-Bar", "bar"}},
				{Name: "HeaderRegexp", Args: []interface{}{"X-Foo", "foo"}},
			},
		},
	}, {
		title:  "legacy shunt",
		route:  &Route{Shunt: true},
		expect: &Route{BackendType: ShuntBackend},
	}, {
		title:  "network backend",
		route:  &Route{Backend: "https://www.example.org"},
		expect: &Route{BackendType: NetworkBackend, Backend: "https://www.example.org"},
	}, {
		title:  "clear LB when different type",
		route:  &Route{LBEndpoints: []*LBEndpoint{{Address: "https://one.example.org"}, {Address: "https://two.example.org"}}},
		expect: &Route{},
	}} {
		t.Run(test.title, func(t *testing.T) {
			c := Canonical(test.route)
			if !reflect.DeepEqual(c, test.expect) {
				t.Error("failed to canonicalize route")
				t.Log(cmp.Diff(c, test.expect))
			}
		})
	}
}

func TestCanonicalList(t *testing.T) {
	for _, test := range []struct {
		title  string
		list   []*Route
		expect []*Route
	}{{
		title: "zero routes",
	}, {
		title: "list",
		list:  []*Route{{Shunt: true}, {Method: "GET"}},
		expect: []*Route{
			{BackendType: ShuntBackend},
			{Predicates: []*Predicate{{Name: "Method", Args: []interface{}{"GET"}}}},
		},
	}} {
		t.Run(test.title, func(t *testing.T) {
			c := CanonicalList(test.list)
			if !reflect.DeepEqual(c, test.expect) {
				t.Error("failed to canonicalize list of routes")
				t.Log(cmp.Diff(c, test.expect))
			}
		})
	}
}
