package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/metrics"
)

func makeEntry(payload string, ttl time.Duration) *Entry {
	return &Entry{
		StatusCode: http.StatusOK,
		Payload:    []byte(payload),
		Header:     http.Header{"Content-Type": {"application/json"}},
		CreatedAt:  time.Now(),
		TTL:        ttl,
	}
}

func TestLRUStorage_HitAndMiss(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()

	got, err := s.Get(ctx, "missing")
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) for missing key, got (%v, %v)", got, err)
	}

	want := makeEntry(`{"id":1}`, time.Minute)
	if err := s.Set(ctx, "k1", want); err != nil {
		t.Fatal(err)
	}

	got, err = s.Get(ctx, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected entry, got nil")
	}
	if string(got.Payload) != string(want.Payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got.Payload, want.Payload)
	}
	if got.StatusCode != want.StatusCode {
		t.Fatalf("status mismatch: got %d, want %d", got.StatusCode, want.StatusCode)
	}
}

func TestLRUStorage_HardExpiry(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()

	entry := makeEntry("stale", time.Millisecond)
	// Back-date CreatedAt so the entry is already expired.
	entry.CreatedAt = time.Now().Add(-time.Second)

	if err := s.Set(ctx, "expired", entry); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "expired")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for expired entry, got %+v", got)
	}

	// Confirm the key was evicted and a second Get is also nil.
	got, err = s.Get(ctx, "expired")
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) after eviction, got (%v, %v)", got, err)
	}
}

func TestLRUStorage_Delete(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()

	if err := s.Set(ctx, "del", makeEntry("x", time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "del"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "del")
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) after delete, got (%v, %v)", got, err)
	}
}

func TestLRUStorage_InPlaceUpdate(t *testing.T) {
	sample, _ := json.Marshal(makeEntry("v1", time.Minute))
	entrySize := int64(len(sample)) + 20
	s := NewLRUStorage(entrySize*shardCount, nil, metrics.Default)
	ctx := context.Background()

	// Overwrite an existing key — Get must return the new payload.
	if err := s.Set(ctx, "k", makeEntry("v1", time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, "k", makeEntry("v2", time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "k")
	if err != nil || got == nil {
		t.Fatalf("expected entry after overwrite, got (%v, %v)", got, err)
	}
	if string(got.Payload) != "v2" {
		t.Fatalf("expected updated payload %q, got %q", "v2", got.Payload)
	}
}

func TestLRUStorage_ImmutabilityAfterSet(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()

	entry := makeEntry("original", time.Minute)
	if err := s.Set(ctx, "imm", entry); err != nil {
		t.Fatal(err)
	}

	// Mutate the original after storing; cached copy must be unaffected.
	entry.Payload = []byte("mutated")

	got, err := s.Get(ctx, "imm")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if string(got.Payload) != "original" {
		t.Fatalf("cache was mutated: got %q", got.Payload)
	}
}

func TestLRUStorage_EvictionCallbackDoesNotDeadlock(t *testing.T) {
	// Regression: onEvict called Bytes() which re-acquired the shard mutex
	// already held by set(), deadlocking the goroutine.
	var lru *LRUStorage
	lru = NewLRUStorage(1<<20, func() {
		// This mirrors the onEvict in NewCacheFilter.
		_ = lru.lru.Bytes()
	}, metrics.Default)
	ctx := context.Background()

	// Fill one shard past capacity to force eviction. Each entry is ~100 bytes;
	// writing shardCount+1 unique keys guarantees at least one shard overflows.
	sample, _ := json.Marshal(makeEntry("x", time.Minute))
	entrySize := int64(len(sample)) + 20
	// Use a tiny budget so the first two writes to the same shard evict.
	lru = NewLRUStorage(entrySize*int64(shardCount), func() {
		_ = lru.lru.Bytes()
	}, metrics.Default)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Write shardCount+1 distinct keys — guarantees eviction on at least one shard.
		for i := range shardCount + 1 {
			key := fmt.Sprintf("key-%d", i)
			_ = lru.Set(ctx, key, makeEntry("payload", time.Minute))
		}
	}()

	select {
	case <-done:
		// passed
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: Set() did not return within 5 seconds")
	}
}

func TestLRUStorage_OversizedEntry(t *testing.T) {
	// With 256 shards and 1 KB total capacity, each shard holds 4 bytes.
	// A payload larger than 4 bytes exceeds every shard's maxBytes.
	const totalBytes = 1024 // 1 KB → 4 bytes per shard
	m := &testMetrics{}
	s := NewLRUStorage(totalBytes, nil, m)

	ctx := context.Background()
	entry := makeEntry(string(make([]byte, 1000)), time.Minute) // 1000-byte payload ≫ 4-byte shard

	// Set must succeed (nil error) even though the entry is too large to store.
	if err := s.Set(ctx, "oversized", entry); err != nil {
		t.Fatalf("Set returned unexpected error: %v", err)
	}

	// The lru_oversized counter must have been incremented exactly once.
	if got := m.counter("lru_oversized"); got != 1 {
		t.Errorf("lru_oversized counter: got %d, want 1", got)
	}

	// The entry must not have been stored — Get must return nil.
	got, err := s.Get(ctx, "oversized")
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected Get to return nil for oversized entry, got %+v", got)
	}
}

func TestLRUShard_CrossKeyEviction(t *testing.T) {
	// Test lruShard directly to make eviction deterministic — no hash routing.
	dataA := []byte("aaaa")
	dataB := []byte("bbbb")
	// maxBytes fits exactly one entry; writing a second must evict the first.
	shard := newLRUShard(int64(len(dataA)), nil)

	shard.set("a", dataA)
	if _, ok := shard.get("a"); !ok {
		t.Fatal("expected key a to be present after set")
	}

	shard.set("b", dataB)

	if _, ok := shard.get("a"); ok {
		t.Fatal("expected key a to be evicted after key b filled the shard")
	}
	if _, ok := shard.get("b"); !ok {
		t.Fatal("expected key b to be present after set")
	}
}
