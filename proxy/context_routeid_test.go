package proxy

import (
	"testing"

	log "github.com/sirupsen/logrus"
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

func TestContextLoggerHasApplicationField(t *testing.T) {
	ctx := &context{application: "my-service"}

	logger := ctx.Logger()

	entry, ok := logger.(*log.Entry)
	if !ok {
		t.Fatalf("Logger() returned %T, want *log.Entry", logger)
	}

	got, ok := entry.Data["application"]
	if !ok {
		t.Fatal("Logger() entry missing 'application' field")
	}
	if got != "my-service" {
		t.Errorf("application field = %q, want %q", got, "my-service")
	}
}

func TestContextLoggerNoApplicationFieldWhenEmpty(t *testing.T) {
	ctx := &context{}

	logger := ctx.Logger()

	if entry, ok := logger.(*log.Entry); ok {
		if _, hasApp := entry.Data["application"]; hasApp {
			t.Error("Logger() should not have 'application' field when application is empty")
		}
	}
}
