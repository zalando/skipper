package routing

import (
	"context"
	"sync"
)

type contextKey struct{}

var routingContextKey contextKey

// NewContext returns a new context with associated routing context.
// It does nothing and returns ctx if it already has associated routing context.
func NewContext(ctx context.Context) context.Context {
	if _, ok := ctx.Value(routingContextKey).(*sync.Map); ok {
		return ctx
	}
	return context.WithValue(ctx, routingContextKey, &sync.Map{})
}

// FromContext returns value from the routing context stored in ctx.
// It returns value associated with the key or stores result of the defaultValue call.
// defaultValue may be called multiple times but only one result will be used as a default value.
func FromContext[K comparable, V any](ctx context.Context, key K, defaultValue func() V) V {
	m, _ := ctx.Value(routingContextKey).(*sync.Map)

	// https://github.com/golang/go/issues/44159#issuecomment-780774977
	val, ok := m.Load(key)
	if !ok {
		val = defaultValue()
		val, _ = m.LoadOrStore(key, val)
	}
	return val.(V)
}
