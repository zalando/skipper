package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
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
	m := &metricstest.MockMetrics{}
	s := NewLRUStorage(totalBytes, nil, m)

	ctx := context.Background()
	entry := makeEntry(string(make([]byte, 1000)), time.Minute) // 1000-byte payload ≫ 4-byte shard

	// Set must succeed (nil error) even though the entry is too large to store.
	if err := s.Set(ctx, "oversized", entry); err != nil {
		t.Fatalf("Set returned unexpected error: %v", err)
	}

	// The lru_oversized counter must have been incremented exactly once.
	if got, _ := m.Counter("cache.lru_oversized"); got != 1 {
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
	if _, ok := shard.get("a", true); !ok {
		t.Fatal("expected key a to be present after set")
	}

	shard.set("b", dataB)

	if _, ok := shard.get("a", true); ok {
		t.Fatal("expected key a to be evicted after key b filled the shard")
	}
	if _, ok := shard.get("b", true); !ok {
		t.Fatal("expected key b to be present after set")
	}
}

func TestShardedByteLRU_GetIsolation(t *testing.T) {
	for _, data := range [][]byte{nil, {}, []byte("original")} {
		t.Run(fmt.Sprintf("size-%d", len(data)), func(t *testing.T) {
			s := NewShardedByteLRU(1<<20, nil)
			s.Set("key", data)
			got, ok := s.Get("key")
			if !ok || got == nil || !bytes.Equal(got, data) {
				t.Fatalf("Get = %v, %v; want %v, true", got, ok, data)
			}
			if len(got) != 0 {
				got[0] = 'X'
			}
			again, ok := s.Get("key")
			if !ok || !bytes.Equal(again, data) {
				t.Fatalf("Get returned shared bytes: %q", again)
			}
		})
	}
}

func TestLRUStorage_GetIsolation(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()
	entry := makeEntry("original", time.Minute)
	entry.VaryHeaders = []string{"Accept-Language"}
	if err := s.Set(ctx, "key", entry); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := s.Get(ctx, "key")
		if err != nil || !cmp.Equal(got, entry) {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, entry)
		}
		got.Payload[0] = 'X'
		got.Header["Content-Type"][0] = "mutated"
		got.VaryHeaders[0] = "mutated"
	}
}

func TestLRUStorage_SnapshotLifetime(t *testing.T) {
	for _, operation := range []string{"overwrite", "delete", "evict", "expire"} {
		t.Run(operation, func(t *testing.T) {
			entry := makeEntry("original", time.Minute)
			if operation == "expire" {
				entry.CreatedAt = time.Now().Add(-2 * time.Minute)
			}
			data, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			s := NewLRUStorage(int64(len(data))*shardCount, nil, metrics.Default)
			ctx := context.Background()
			if err := s.Set(ctx, "key", entry); err != nil {
				t.Fatal(err)
			}
			snapshot, ok := s.lru.getShard("key").get("key", false)
			if !ok {
				t.Fatal("missing snapshot")
			}

			// Finish the writer after snapshot acquisition, before decoding it.
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch operation {
				case "overwrite":
					updated := *entry
					updated.Payload = []byte("replaced")
					if err := s.Set(ctx, "key", &updated); err != nil {
						t.Error(err)
					}
				case "delete":
					if err := s.Delete(ctx, "key"); err != nil {
						t.Error(err)
					}
				case "evict":
					for i := 0; ; i++ {
						other := fmt.Sprintf("other-%d", i)
						if s.lru.getShard(other) == s.lru.getShard("key") {
							if err := s.Set(ctx, other, entry); err != nil {
								t.Error(err)
							}
							break
						}
					}
				case "expire":
					if got, err := s.Get(ctx, "key"); err != nil || got != nil {
						t.Errorf("expired Get = %+v, %v", got, err)
					}
				}
			}()
			<-done

			var got Entry
			if err := json.Unmarshal(snapshot, &got); err != nil || !cmp.Equal(&got, entry) {
				t.Fatalf("snapshot changed after %s: %+v, %v", operation, got, err)
			}
			current, err := s.Get(ctx, "key")
			if err != nil {
				t.Fatal(err)
			}
			if operation == "overwrite" {
				if current == nil || string(current.Payload) != "replaced" {
					t.Fatalf("missing replacement: %+v", current)
				}
			} else if current != nil {
				t.Fatalf("entry still resident after %s", operation)
			}
		})
	}
}

func TestLRUStorage_ConcurrentReads(t *testing.T) {
	s := NewLRUStorage(1<<20, nil, metrics.Default)
	ctx := context.Background()
	entry := makeEntry(strings.Repeat("a", 1024), time.Hour)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			<-start
			for range 100 {
				got, err := s.Get(ctx, "key")
				if err != nil {
					t.Error(err)
					return
				}
				if got != nil {
					if !bytes.Equal(got.Payload, entry.Payload) || !cmp.Equal(got.Header, entry.Header) {
						t.Error("inconsistent entry")
						return
					}
					got.Payload[0] = 'X'
					got.Header.Set("Content-Type", "mutated")
				}
			}
		})
	}
	close(start)
	for range 100 {
		if err := s.Set(ctx, "key", entry); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, "key"); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
