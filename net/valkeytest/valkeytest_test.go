package valkeytest

import (
	"context"
	"testing"
	"time"

	"github.com/zalando/skipper/net/nettest"
)

func TestValkeytest(t *testing.T) {
	r, done := NewTestValkey(t)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", "", nil); err != nil {
		t.Fatalf("Failed to ping valkey: %v", err)
	}
}

func TestValkeytestSwarmMTLS(t *testing.T) {
	ca := nettest.NewCA(t)
	r, done := NewTestValkeySwarmMTLS(t, ca)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", "", clientTLSConfig(t, ca)); err != nil {
		t.Fatalf("Failed to ping valkey with mutual TLS: %v", err)
	}
}
