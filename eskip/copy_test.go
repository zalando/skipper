package eskip

import (
	"reflect"
	"testing"
)

func TestCopy(t *testing.T) {
	checkFilter := func(t *testing.T, c, f *Filter) {
		if !reflect.DeepEqual(c, f) {
			t.Error("failed to copy filter")
		}

		f.Args[0] = "test-slice-identity"
		if c.Args[0] == f.Args[0] {
			t.Error("failed to copy args slice")
		}
	}

	checkPredicate := func(t *testing.T, c, p *Predicate) {
		if !reflect.DeepEqual(c, p) {
			t.Error("failed to copy predicate")
		}

		p.Args[0] = "test-slice-identity"
		if c.Args[0] == p.Args[0] {
			t.Error("failed to copy args slice")
		}
	}

	checkRoute := func(t *testing.T, c, r *Route) {
		r = Canonical(r)
		if !reflect.DeepEqual(c, r) {
			t.Error("failed to copy route")
		}

		for i := range r.Predicates {
			checkPredicate(t, c.Predicates[i], r.Predicates[i])
		}

		for i := range r.Filters {
			checkFilter(t, c.Filters[i], r.Filters[i])
		}

		r.LBEndpoints[0] = &LBEndpoint{Address: "test-slice-identity"}
		if c.LBEndpoints[0] == r.LBEndpoints[0] {
			t.Error("failed to copy LB endpoints")
		}
	}

	t.Run("filters", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			if CopyFilter(nil) != nil {
				t.Error("failed to copy nil filter")
			}
		})

		t.Run("single", func(t *testing.T) {
			f := &Filter{Name: "foo", Args: []interface{}{"hello", 42, 3.14}}
			c := CopyFilter(f)
			checkFilter(t, c, f)
		})

		t.Run("multiple", func(t *testing.T) {
			f := []*Filter{
				{Name: "foo", Args: []interface{}{"hello1", 42, 3.14}},
				{Name: "bar", Args: []interface{}{"hello2", 2 * 42, 2 * 3.14}},
				{Name: "baz", Args: []interface{}{"hello3", 3 * 42, 3 * 3.14}},
			}

			c := CopyFilters(f)
			if len(c) != len(f) {
				t.Fatal("failed to copy filters")
			}

			for i := range c {
				checkFilter(t, c[i], f[i])
			}
		})
	})

	t.Run("predicates", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			if CopyPredicate(nil) != nil {
				t.Error("failed to copy nil predicate")
			}
		})

		t.Run("single", func(t *testing.T) {
			p := &Predicate{Name: "foo", Args: []interface{}{"hello", 42, 3.14}}
			c := CopyPredicate(p)
			checkPredicate(t, c, p)
		})

		t.Run("multiple", func(t *testing.T) {
			p := []*Predicate{
				{Name: "foo", Args: []interface{}{"hello1", 42, 3.14}},
				{Name: "bar", Args: []interface{}{"hello2", 2 * 42, 2 * 3.14}},
				{Name: "baz", Args: []interface{}{"hello3", 3 * 42, 3 * 3.14}},
			}

			c := CopyPredicates(p)
			if len(c) != len(p) {
				t.Fatal("failed to copy predicates")
			}

			for i := range c {
				checkPredicate(t, c[i], p[i])
			}
		})
	})

	t.Run("routes", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			if Copy(nil) != nil {
				t.Error("failed to copy nil route")
			}
		})

		t.Run("single", func(t *testing.T) {
			r := &Route{
				Id: "route1",
				Predicates: []*Predicate{
					{Name: "pfoo", Args: []interface{}{"hello1", 42, 3.14}},
					{Name: "pbar", Args: []interface{}{"hello2", 2 * 42, 2 * 3.14}},
					{Name: "pbaz", Args: []interface{}{"hello3", 3 * 42, 3 * 3.14}},
				},
				Filters: []*Filter{
					{Name: "ffoo", Args: []interface{}{"hello1", 42, 3.14}},
					{Name: "fbar", Args: []interface{}{"hello2", 2 * 42, 2 * 3.14}},
					{Name: "fbaz", Args: []interface{}{"hello3", 3 * 42, 3 * 3.14}},
				},
				BackendType: LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []*LBEndpoint{
					{Address: "10.0.0.1:80"},
					{Address: "10.0.0.2:80"},
				},
			}

			c := Copy(r)
			checkRoute(t, c, r)
		})

		t.Run("multiple", func(t *testing.T) {
			r := []*Route{{
				Id: "route1",
				Predicates: []*Predicate{
					{Name: "p1foo", Args: []interface{}{"hello11", 42, 3.14}},
					{Name: "p1bar", Args: []interface{}{"hello12", 2 * 42, 2 * 3.14}},
					{Name: "p1baz", Args: []interface{}{"hello13", 3 * 42, 3 * 3.14}},
				},
				Filters: []*Filter{
					{Name: "f1foo", Args: []interface{}{"hello11", 42, 3.14}},
					{Name: "f1bar", Args: []interface{}{"hello12", 2 * 42, 2 * 3.14}},
					{Name: "f1baz", Args: []interface{}{"hello13", 3 * 42, 3 * 3.14}},
				},
				BackendType: LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []*LBEndpoint{
					{Address: "10.0.1.1:80"},
					{Address: "10.0.1.2:80"},
				},
			}, {
				Id: "route2",
				Predicates: []*Predicate{
					{Name: "p2foo", Args: []interface{}{"hello21", 42, 3.14}},
					{Name: "p2bar", Args: []interface{}{"hello22", 2 * 42, 2 * 3.14}},
					{Name: "p2baz", Args: []interface{}{"hello23", 3 * 42, 3 * 3.14}},
				},
				Filters: []*Filter{
					{Name: "f2foo", Args: []interface{}{"hello21", 42, 3.14}},
					{Name: "f2bar", Args: []interface{}{"hello22", 2 * 42, 2 * 3.14}},
					{Name: "f2baz", Args: []interface{}{"hello23", 3 * 42, 3 * 3.14}},
				},
				BackendType: LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []*LBEndpoint{
					{Address: "10.0.2.1:80"},
					{Address: "10.0.2.2:80"},
				},
			}, {
				Id: "route3",
				Predicates: []*Predicate{
					{Name: "p3foo", Args: []interface{}{"hello31", 42, 3.14}},
					{Name: "p3bar", Args: []interface{}{"hello32", 2 * 42, 2 * 3.14}},
					{Name: "p3baz", Args: []interface{}{"hello33", 3 * 42, 3 * 3.14}},
				},
				Filters: []*Filter{
					{Name: "f3foo", Args: []interface{}{"hello31", 42, 3.14}},
					{Name: "f3bar", Args: []interface{}{"hello32", 2 * 42, 2 * 3.14}},
					{Name: "f3baz", Args: []interface{}{"hello33", 3 * 42, 3 * 3.14}},
				},
				BackendType: LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []*LBEndpoint{
					{Address: "10.0.3.1:80"},
					{Address: "10.0.3.2:80"},
				},
			}}

			c := CopyRoutes(r)
			if len(c) != len(r) {
				t.Fatal("failed to copy routes")
			}

			for i := range c {
				checkRoute(t, c[i], r[i])
			}
		})
	})
}
