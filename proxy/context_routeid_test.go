package proxy

import (
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

func TestContextRouteId(t *testing.T) {
	tests := []struct {
		name     string
		route    *routing.Route
		expected string
	}{
		{
			name: "route with id",
			route: &routing.Route{
				Route: eskip.Route{
					Id: "test_route_123",
				},
			},
			expected: "test_route_123",
		},
		{
			name:     "no route",
			route:    nil,
			expected: "",
		},
		{
			name: "route with empty id",
			route: &routing.Route{
				Route: eskip.Route{
					Id: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &context{
				route: tt.route,
			}

			result := ctx.RouteId()
			if result != tt.expected {
				t.Errorf("RouteId() = %q, want %q", result, tt.expected)
			}
		})
	}
}
