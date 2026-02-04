package filtertest

import (
	"testing"
)

func TestContextRouteId(t *testing.T) {
	tests := []struct {
		name     string
		routeId  string
		expected string
	}{
		{
			name:     "route with id",
			routeId:  "test_route_456",
			expected: "test_route_456",
		},
		{
			name:     "empty route id",
			routeId:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				FRouteId: tt.routeId,
			}

			result := ctx.RouteId()
			if result != tt.expected {
				t.Errorf("RouteId() = %q, want %q", result, tt.expected)
			}
		})
	}
}
