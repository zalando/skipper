package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/valkeytest"
)

// stubValkeyClient is an in-memory valkeyClient stub for unit tests that
// should not depend on a running Valkey instance or Docker.
type stubValkeyClient struct {
	mu         sync.Mutex
	data       map[string]string
	broken     bool          // if true, all operations return an error
	lastExpire time.Duration // records the expire arg of the last SetWithExpire call
}

func newStubValkeyClient() *stubValkeyClient {
	return &stubValkeyClient{data: make(map[string]string)}
}

func newBrokenStubValkeyClient() *stubValkeyClient {
	return &stubValkeyClient{data: make(map[string]string), broken: true}
}

func (s *stubValkeyClient) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return "", errors.New("stub: broken")
	}
	v, ok := s.data[key]
	if !ok {
		return "", valkey.Nil
	}
	return v, nil
}

func (s *stubValkeyClient) SetWithExpire(_ context.Context, key, value string, expire time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return errors.New("stub: broken")
	}
	s.data[key] = value
	s.lastExpire = expire
	return nil
}

func (s *stubValkeyClient) Expire(_ context.Context, key string, d time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return 0, errors.New("stub: broken")
	}
	_, ok := s.data[key]
	if !ok {
		return 0, nil
	}
	if d < 0 {
		// negative duration → immediate deletion (mirrors Valkey EXPIRE key -1 semantics)
		delete(s.data, key)
	}
	// non-negative duration → TTL update; not modelled in stub (no expiry tracking)
	return 1, nil
}

func (s *stubValkeyClient) Del(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return 0, errors.New("stub: broken")
	}
	_, ok := s.data[key]
	if !ok {
		return 0, nil
	}
	delete(s.data, key)
	return 1, nil
}

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

	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(ring, lru, &metricstest.MockMetrics{}, 0, valkey.IsValkeyNil)

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

	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	m := &metricstest.MockMetrics{}
	s := NewL2Storage(ring, lru, m, 0, valkey.IsValkeyNil)

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
	if v, _ := m.Counter("cache.l1_hit"); v == 0 {
		t.Error("expected l1_hit to be incremented: Set fallback warmed L1, Get should serve from it")
	}

	l2Error, _ := m.Counter("cache.l2_get_error")
	if l2Error != 0 {
		t.Errorf("expected l2_get_error=0 (L1 served before Valkey was contacted), got %d", l2Error)
	}

	// Confirm the entry was physically written to L1 — not just returned via some
	// other path. A direct read from LRUStorage proves the write actually happened.
	l1Entry, err := lru.Get(ctx, key)
	if err != nil {
		t.Fatalf("L1 direct Get: %v", err)
	}
	if l1Entry == nil {
		t.Error("expected entry to be written to L1 on Valkey fallback, but L1 Get returned nil")
	}
}

func TestValkeyStorage_RecordsValkeyMiss(t *testing.T) {
	// Uses a stub client — no Docker or live Valkey needed.
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, m, 0, valkey.IsValkeyNil)

	got, err := s.Get(context.Background(), "nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on miss, got %+v", got)
	}
	l2Miss, _ := m.Counter("cache.l2_miss")
	if l2Miss != 1 {
		t.Errorf("expected l2_miss=1, got %d", l2Miss)
	}
	l2Error, _ := m.Counter("cache.l2_get_error")
	if l2Error != 0 {
		t.Errorf("expected l2_get_error=0 on clean miss, got %d", l2Error)
	}
}

func TestValkeyStorage_WriteThroughWarmsL1(t *testing.T) {
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, m, 60*time.Second, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "wt-key"
	entry := &Entry{
		StatusCode: 200,
		Payload:    []byte("warm"),
		TTL:        time.Minute,
		CreatedAt:  time.Now(),
	}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Break Valkey so any Get must come from L1.
	stub.broken = true

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get with broken Valkey: %v", err)
	}
	if got == nil {
		t.Fatal("expected L1 warm hit, got nil — write-through did not warm L1")
	}
	if string(got.Payload) != "warm" {
		t.Errorf("payload: got %q, want %q", string(got.Payload), "warm")
	}
	l1hit, _ := m.Counter("cache.l1_hit")
	if l1hit != 1 {
		t.Errorf("expected l1_hit=1, got %d", l1hit)
	}
}

func TestValkeyStorage_L1TTLBoundedToEntryTTL(t *testing.T) {
	stub := newStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, &metricstest.MockMetrics{}, 60*time.Second, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "bounded-key"
	entry := &Entry{
		StatusCode: 200,
		Payload:    []byte("short"),
		TTL:        10 * time.Second, // shorter than l1TTL
		CreatedAt:  time.Now(),
	}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Read directly from L1 to inspect the stored TTL.
	l1Entry, err := lru.Get(ctx, key)
	if err != nil {
		t.Fatalf("L1 Get: %v", err)
	}
	if l1Entry == nil {
		t.Fatal("expected L1 entry after write-through, got nil")
	}
	if l1Entry.TTL != 10*time.Second {
		t.Errorf("L1 TTL: got %v, want %v (should be min(l1TTL, entry.TTL))", l1Entry.TTL, 10*time.Second)
	}
}

func TestValkeyStorage_L1TTL_Zero_DisablesWarming(t *testing.T) {
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, m, 0, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "no-warm-key"
	entry := &Entry{
		StatusCode: 200,
		Payload:    []byte("bypass"),
		TTL:        time.Minute,
		CreatedAt:  time.Now(),
	}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Break Valkey — if L1 were warmed, Get would still return the entry.
	stub.broken = true

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get with broken Valkey: %v", err)
	}
	if got != nil {
		t.Error("expected nil (write-around: L1 should not be warmed when l1TTL=0)")
	}
}

func TestValkeyStorage_RecordsL2Hit(t *testing.T) {
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, m, 0, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "l2-hit-key"
	entry := &Entry{StatusCode: 200, Payload: []byte("v"), TTL: time.Minute, CreatedAt: time.Now()}

	if err := s.Set(ctx, key, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// L1 is not warmed (l1TTL=0), so Get must go to Valkey — a real L2 hit.
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected entry from Valkey, got nil")
	}
	l2Hit, _ := m.Counter("cache.l2_hit")
	if l2Hit != 1 {
		t.Errorf("expected l2_hit=1, got %d", l2Hit)
	}
	l1Hit, _ := m.Counter("cache.l1_hit")
	if l1Hit != 0 {
		t.Errorf("expected l1_hit=0 (write-around), got %d", l1Hit)
	}
}

func TestValkeyStorage_SplitFallbackCounters(t *testing.T) {
	// Uses a broken stub — no Docker or live Valkey needed.
	// Set triggers l2_set_fallback, which writes the entry to L1.
	// Get checks L1 first (L1-first reads) and finds the entry — incrementing l1_hit,
	// not l2_get_error.
	stub := newBrokenStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, m, 0, valkey.IsValkeyNil)

	ctx := context.Background()
	entry := &Entry{StatusCode: 200, Payload: []byte("x"), TTL: time.Minute, CreatedAt: time.Now()}

	_ = s.Set(ctx, "k", entry)
	l2SetFallback, _ := m.Counter("cache.l2_set_fallback")
	if l2SetFallback != 1 {
		t.Errorf("expected l2_set_fallback=1, got %d", l2SetFallback)
	}
	l2GetError, _ := m.Counter("cache.l2_get_error")
	if l2GetError != 0 {
		t.Errorf("expected l2_get_error=0 after Set, got %d", l2GetError)
	}

	// L1-first: the entry was written to L1 by the Set fallback path, so Get returns
	// it from L1 without ever touching (broken) Valkey.
	_, _ = s.Get(ctx, "k")
	l1Hit, _ := m.Counter("cache.l1_hit")
	if l1Hit != 1 {
		t.Errorf("expected l1_hit=1, got %d", l1Hit)
	}

	l2GetError, _ = m.Counter("cache.l2_get_error")
	if l2GetError != 0 {
		t.Errorf("expected l2_get_error=0 (L1 served before Valkey check), got %d", l2GetError)
	}
	l2SetFallback, _ = m.Counter("cache.l2_set_fallback")
	if l2SetFallback != 1 {
		t.Errorf("l2_set_fallback should still be 1, got %d", l2SetFallback)
	}
}

func TestValkeyStorage_DeleteCleansL1EvenOnValkeyError(t *testing.T) {
	// Valkey is broken, so Set falls back to L1. Delete must still clean L1
	// regardless of the Expire error from Valkey.
	stub := newBrokenStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := NewL2Storage(stub, lru, &metricstest.MockMetrics{}, 0, valkey.IsValkeyNil)

	ctx := context.Background()
	entry := &Entry{StatusCode: 200, Payload: []byte("body"), TTL: time.Minute, CreatedAt: time.Now()}

	_ = s.Set(ctx, "k", entry) // falls back to L1 (Valkey broken)

	got, err := lru.Get(ctx, "k")
	if err != nil || got == nil {
		t.Fatal("expected entry in L1 after Set fallback")
	}

	_ = s.Delete(ctx, "k") // Valkey Expire will error; L1 must still be cleaned

	got, _ = lru.Get(ctx, "k")
	if got != nil {
		t.Error("expected L1 to be empty after Delete, but entry still present")
	}
}

func TestL2Storage_NewL2Storage_NegativeL1TTL_ClampsToDefault(t *testing.T) {
	stub := newStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, &metricstest.MockMetrics{})
	s := NewL2Storage(stub, lru, &metricstest.MockMetrics{}, -time.Second, valkey.IsValkeyNil)
	if s.l1TTL != defaultMinTTL {
		t.Errorf("expected l1TTL clamped to %v, got %v", defaultMinTTL, s.l1TTL)
	}
}

func TestL2Storage_Get_CorruptJSON_ReturnsError(t *testing.T) {
	stub := newStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, &metricstest.MockMetrics{})
	s := NewL2Storage(stub, lru, &metricstest.MockMetrics{}, 0, valkey.IsValkeyNil)

	// Write corrupt JSON directly into the stub, bypassing Set.
	stub.mu.Lock()
	stub.data["bad-key"] = "not json {"
	stub.mu.Unlock()

	_, err := s.Get(context.Background(), "bad-key")
	if err == nil {
		t.Fatal("expected error for corrupt JSON in L2, got nil")
	}
}

func TestL2Storage_Get_L2Hit_WarmsL1(t *testing.T) {
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, m)
	s := NewL2Storage(stub, lru, m, 60*time.Second, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "warm-key"
	entry := &Entry{StatusCode: 200, Payload: []byte("v"), TTL: 2 * time.Minute, CreatedAt: time.Now()}

	// Write directly to L2 stub (bypass Set so L1 stays cold).
	data, _ := json.Marshal(entry)
	stub.mu.Lock()
	stub.data[key] = string(data)
	stub.mu.Unlock()

	// First Get: L2 hit, must warm L1.
	got, err := s.Get(ctx, key)
	if err != nil || got == nil {
		t.Fatalf("expected L2 hit: err=%v, got=%v", err, got)
	}
	l2Hit, _ := m.Counter("cache.l2_hit")
	if l2Hit != 1 {
		t.Errorf("expected l2_hit=1, got %d", l2Hit)
	}

	// Break L2 — second Get must come from L1 (write-through warmed it).
	stub.broken = true
	got2, err := s.Get(ctx, key)
	if err != nil || got2 == nil {
		t.Fatalf("expected L1 hit after warming: err=%v, got=%v", err, got2)
	}
	l1Hit, _ := m.Counter("cache.l1_hit")
	if l1Hit != 1 {
		t.Errorf("expected l1_hit=1 after L1 warming, got %d", l1Hit)
	}
}

func TestL2Storage_Get_L2Hit_ExpiredEntry_SkipsL1Warming(t *testing.T) {
	stub := newStubValkeyClient()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(64<<20, nil, m)
	s := NewL2Storage(stub, lru, m, 60*time.Second, valkey.IsValkeyNil)

	ctx := context.Background()
	key := "expired-key"
	// CreatedAt 10 min ago, TTL 1 min → remaining = -9 min → L1 warming must be skipped.
	entry := &Entry{StatusCode: 200, Payload: []byte("stale"), TTL: time.Minute, CreatedAt: time.Now().Add(-10 * time.Minute)}

	data, _ := json.Marshal(entry)
	stub.mu.Lock()
	stub.data[key] = string(data)
	stub.mu.Unlock()

	// L2 hit — but warming must be skipped because remaining <= 0.
	got, err := s.Get(ctx, key)
	if err != nil || got == nil {
		t.Fatalf("expected L2 hit for expired entry: err=%v, got=%v", err, got)
	}

	// Break L2 — if L1 was accidentally warmed, Get would still return the entry.
	stub.broken = true
	got2, _ := s.Get(ctx, key)
	if got2 != nil {
		t.Error("L1 must NOT be warmed when remaining TTL <= 0, but entry was found in L1")
	}
}

func TestL2Storage_Set_ZeroTTL_UsesDefaultMinTTL(t *testing.T) {
	stub := newStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, &metricstest.MockMetrics{})
	s := NewL2Storage(stub, lru, &metricstest.MockMetrics{}, 0, valkey.IsValkeyNil)

	ctx := context.Background()
	// TTL=0, StaleIfError=0, StaleWhileRevalidate=0 → l2TTL=0 → must use defaultMinTTL.
	entry := &Entry{StatusCode: 200, Payload: []byte("x"), TTL: 0, CreatedAt: time.Now()}
	if err := s.Set(ctx, "zero-ttl-key", entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	stub.mu.Lock()
	gotExpire := stub.lastExpire
	stub.mu.Unlock()

	if gotExpire != defaultMinTTL {
		t.Errorf("expected SetWithExpire called with %v, got %v", defaultMinTTL, gotExpire)
	}
}
