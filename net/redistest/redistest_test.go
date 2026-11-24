package redistest

import (
	"context"
	"testing"
	"time"

	"github.com/zalando/skipper/net/nettest"
)

func TestRedistest(t *testing.T) {
	r, done := NewTestRedis(t)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", "", nil); err != nil {
		t.Fatalf("Failed to ping redis: %v", err)
	}
}

func TestRedistestRedis6(t *testing.T) {
	r, done := newTestRedisWithOptions(t, options{image: "redis:6-alpine"})
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", "", nil); err != nil {
		t.Fatalf("Failed to ping redis: %v", err)
	}
}

func TestRedistestSwarmMTLS(t *testing.T) {
	ca := nettest.NewCA(t)
	r, done := NewTestRedisSwarmMTLS(t, ca)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", "", clientTLSConfig(t, ca)); err != nil {
		t.Fatalf("Failed to ping redis with mutual TLS: %v", err)
	}
}
