package redistest

import (
	"context"
	"testing"
	"time"
)

func TestRedistest(t *testing.T) {
	r, done := NewTestRedis(t)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", ""); err != nil {
		t.Fatalf("Failed to ping redis: %v", err)
	}
}

func TestRedistestRedis6(t *testing.T) {
	r, done := newTestRedisWithOptions(t, options{image: "redis:6-alpine"})
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", ""); err != nil {
		t.Fatalf("Failed to ping redis: %v", err)
	}
}
