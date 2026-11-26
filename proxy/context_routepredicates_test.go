package proxy

import (
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

func TestContextRoutePredicates(t *testing.T) {
	tests := []struct {
		name     string
		route    *routing.Route
		expected []filters.RoutePredicate
	}{
		{
			name:     "no route",
			route:    nil,
			expected: nil,
		},
		{
			name: "route with no predicates",
			route: &routing.Route{
				Route: eskip.Route{},
			},
			expected: nil,
		},
		{
			name: "route with predicates",
			route: &routing.Route{
				Route: eskip.Route{
					Predicates: []*eskip.Predicate{
						{Name: "Foo", Args: []interface{}{"bar", float64(42)}},
						{Name: "Baz", Args: []interface{}{}},
					},
				},
			},
			expected: []filters.RoutePredicate{
				{Name: "Foo", Args: []interface{}{"bar", float64(42)}},
				{Name: "Baz", Args: []interface{}{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &context{route: tt.route}
			result := ctx.RoutePredicates()

			if len(result) != len(tt.expected) {
				t.Fatalf("RoutePredicates() returned %d predicates, want %d", len(result), len(tt.expected))
			}
			for i, p := range result {
				if p.Name != tt.expected[i].Name {
					t.Errorf("predicate[%d].Name = %q, want %q", i, p.Name, tt.expected[i].Name)
				}
				if len(p.Args) != len(tt.expected[i].Args) {
					t.Errorf("predicate[%d].Args length = %d, want %d", i, len(p.Args), len(tt.expected[i].Args))
				}
			}
		})
	}
}

func TestContextRoutePredicatesIsCopy(t *testing.T) {
	route := &routing.Route{
		Route: eskip.Route{
			Predicates: []*eskip.Predicate{
				{Name: "Foo", Args: []interface{}{"original"}},
			},
		},
	}
	ctx := &context{route: route}

	result := ctx.RoutePredicates()
	result[0].Name = "mutated"
	result[0].Args[0] = "mutated"

	if route.Route.Predicates[0].Name != "Foo" {
		t.Error("RoutePredicates() returned a reference; mutation affected the route")
	}
	if route.Route.Predicates[0].Args[0] != "original" {
		t.Error("RoutePredicates() Args are not copied; mutation affected the route")
	}
}

func TestContextRouteFilters(t *testing.T) {
	tests := []struct {
		name     string
		route    *routing.Route
		expected []filters.RouteFilter
	}{
		{
			name:     "no route",
			route:    nil,
			expected: nil,
		},
		{
			name: "route with no filters",
			route: &routing.Route{
				Route: eskip.Route{},
			},
			expected: nil,
		},
		{
			name: "route with filters",
			route: &routing.Route{
				Route: eskip.Route{
					Filters: []*eskip.Filter{
						{Name: "setResponseHeader", Args: []interface{}{"X-Foo", "bar"}},
						{Name: "status", Args: []interface{}{float64(200)}},
					},
				},
			},
			expected: []filters.RouteFilter{
				{Name: "setResponseHeader", Args: []interface{}{"X-Foo", "bar"}},
				{Name: "status", Args: []interface{}{float64(200)}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &context{route: tt.route}
			result := ctx.RouteFilters()

			if len(result) != len(tt.expected) {
				t.Fatalf("RouteFilters() returned %d filters, want %d", len(result), len(tt.expected))
			}
			for i, f := range result {
				if f.Name != tt.expected[i].Name {
					t.Errorf("filter[%d].Name = %q, want %q", i, f.Name, tt.expected[i].Name)
				}
				if len(f.Args) != len(tt.expected[i].Args) {
					t.Errorf("filter[%d].Args length = %d, want %d", i, len(f.Args), len(tt.expected[i].Args))
				}
			}
		})
	}
}

func TestContextRouteFiltersIsCopy(t *testing.T) {
	route := &routing.Route{
		Route: eskip.Route{
			Filters: []*eskip.Filter{
				{Name: "status", Args: []interface{}{float64(200)}},
			},
		},
	}
	ctx := &context{route: route}

	result := ctx.RouteFilters()
	result[0].Name = "mutated"
	result[0].Args[0] = float64(999)

	if route.Route.Filters[0].Name != "status" {
		t.Error("RouteFilters() returned a reference; mutation affected the route")
	}
	if route.Route.Filters[0].Args[0] != float64(200) {
		t.Error("RouteFilters() Args are not copied; mutation affected the route")
	}
}
