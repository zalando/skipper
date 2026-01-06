package valkeytest

import (
	"context"
	"testing"
	"time"
)

func TestValkeytest(t *testing.T) {
	r, done := NewTestValkey(t)
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := ping(ctx, r, "", ""); err != nil {
		t.Fatalf("Failed to ping valkey: %v", err)
	}
}
