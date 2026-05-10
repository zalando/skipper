package cache

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
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
	s := NewLRUStorage(1 << 20) // 1 MB
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
	s := NewLRUStorage(1 << 20)
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
	s := NewLRUStorage(1 << 20)
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
	s := NewLRUStorage(entrySize * shardCount)
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
	s := NewLRUStorage(1 << 20)
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

func TestLRUShard_CrossKeyEviction(t *testing.T) {
	// Test lruShard directly to make eviction deterministic — no hash routing.
	dataA := []byte("aaaa")
	dataB := []byte("bbbb")
	// maxBytes fits exactly one entry; writing a second must evict the first.
	shard := newLRUShard(int64(len(dataA)))

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
