package cache

import (
	"context"
	"testing"
	"time"

	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/valkeytest"
)

func TestValkeyStorage_GetSetDelete(t *testing.T) {
	addr, done := valkeytest.NewTestValkey(t)
	defer done()

	ring, err := skpnet.NewValkeyRingClient(&skpnet.ValkeyOptions{
		Addrs: []string{addr},
	})
	if err != nil {
		t.Fatalf("NewValkeyRingClient: %v", err)
	}
	defer ring.Close()

	lru := NewLRUStorage(64<<20, nil)
	s := NewValkeyStorage(ring, lru)

	ctx := context.Background()
	key := "test-key"
	entry := &Entry{
		StatusCode: 200,
		Payload:    []byte("hello"),
		TTL:        time.Minute,
		CreatedAt:  time.Now(),
	}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected entry, got nil")
	}
	if got.StatusCode != entry.StatusCode {
		t.Errorf("StatusCode: got %d, want %d", got.StatusCode, entry.StatusCode)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err = s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestValkeyStorage_FallsBackToL1OnValkeyUnavailable(t *testing.T) {
	addr, done := valkeytest.NewTestValkey(t)

	ring, err := skpnet.NewValkeyRingClient(&skpnet.ValkeyOptions{
		Addrs:            []string{addr},
		ConnWriteTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewValkeyRingClient: %v", err)
	}
	defer ring.Close()

	lru := NewLRUStorage(64<<20, nil)
	s := NewValkeyStorage(ring, lru)

	// Stop valkey before exercising fallback paths.
	done()

	ctx := context.Background()
	key := "fallback-key"
	entry := &Entry{
		StatusCode: 200,
		Payload:    []byte("from-l1"),
		TTL:        time.Minute,
		CreatedAt:  time.Now(),
	}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set with valkey down: %v", err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get with valkey down: %v", err)
	}
	if got == nil {
		t.Fatal("expected L1 fallback hit, got nil")
	}
}