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
	if err := ping(ctx, r, ""); err != nil {
		t.Fatalf("Failed to ping redis: %v", err)
	}
}
