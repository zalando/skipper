package routing

import (
	"context"
	"sync"
	"testing"
)

func TestContext(t *testing.T) {
	m := &sync.Map{}
	m.Store("foo", "bar")

	ctx := NewContext(context.Background())
	nctx := context.WithValue(ctx, routingContextKey, m)

	defaultVal := func() string { return "default" }
	val := FromContext(nctx, "foo", defaultVal)
	if val != "bar" {
		t.Fatalf("Failed to get value from context: %q", val)
	}
}

func TestContextDefault(t *testing.T) {
	ctx := context.Background()
	nctx := NewContext(ctx)

	defaultVal := func() string { return "default" }
	if v := FromContext(nctx, "foo", defaultVal); v != "default" {
		t.Fatalf("Failed to get \"default\", got %q", v)
	}
}
