package cache

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/valkeytest"
)

// stubValkeyClient is an in-memory valkeyClient stub for unit tests that
// should not depend on a running Valkey instance or Docker.
type stubValkeyClient struct {
	mu     sync.Mutex
	data   map[string]string
	broken bool // if true, all operations return an error
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

func (s *stubValkeyClient) SetWithExpire(_ context.Context, key, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return errors.New("stub: broken")
	}
	s.data[key] = value
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

// testMetrics is a minimal metrics.Metrics stub for testing.
// Only IncCounter does real work; all other methods are no-ops.
type testMetrics struct {
	mu       sync.Mutex
	counters map[string]int
}

var _ metrics.Metrics = (*testMetrics)(nil)

func (m *testMetrics) IncCounter(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counters == nil {
		m.counters = make(map[string]int)
	}
	m.counters[key]++
}

func (m *testMetrics) counter(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[key]
}

// metrics.Metrics no-op implementations
func (m *testMetrics) MeasureSince(key string, start time.Time)                                 {}
func (m *testMetrics) IncCounterBy(key string, value int64)                                     {}
func (m *testMetrics) IncFloatCounterBy(key string, value float64)                              {}
func (m *testMetrics) MeasureRouteLookup(start time.Time)                                       {}
func (m *testMetrics) MeasureFilterCreate(filterName string, start time.Time)                   {}
func (m *testMetrics) MeasureFilterRequest(filterName string, start time.Time)                  {}
func (m *testMetrics) MeasureAllFiltersRequest(routeId string, start time.Time)                 {}
func (m *testMetrics) MeasureBackendRequestHeader(host string, size int)                        {}
func (m *testMetrics) MeasureBackend(routeId string, start time.Time)                           {}
func (m *testMetrics) MeasureBackendHost(routeBackendHost string, start time.Time)              {}
func (m *testMetrics) MeasureBackendZone(zone string, start time.Time)                          {}
func (m *testMetrics) MeasureFilterResponse(filterName string, start time.Time)                 {}
func (m *testMetrics) MeasureAllFiltersResponse(routeId string, start time.Time)                {}
func (m *testMetrics) MeasureResponse(code int, method string, routeId string, start time.Time) {}
func (m *testMetrics) MeasureResponseSize(host string, size int64)                              {}
func (m *testMetrics) MeasureProxy(requestDuration, responseDuration time.Duration)             {}
func (m *testMetrics) MeasureServe(routeId, host, method string, code int, start time.Time)     {}
func (m *testMetrics) IncRoutingFailures()                                                      {}
func (m *testMetrics) IncErrorsBackend(routeId string)                                          {}
func (m *testMetrics) MeasureBackend5xx(t time.Time)                                            {}
func (m *testMetrics) IncErrorsStreaming(routeId string)                                        {}
func (m *testMetrics) RegisterHandler(path string, handler *http.ServeMux)                      {}
func (m *testMetrics) UpdateGauge(key string, value float64)                                    {}
func (m *testMetrics) SetInvalidRoute(routeId, reason string)                                   {}
func (m *testMetrics) Close()                                                                   {}
func (m *testMetrics) String() string                                                           { return "testMetrics" }

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
	s := NewValkeyStorage(ring, lru, &testMetrics{}, 0)

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
	m := &testMetrics{}
	s := NewValkeyStorage(ring, lru, m, 0)

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
	if m.counter("l1_hit") == 0 {
		t.Error("expected l1_hit to be incremented: Set fallback warmed L1, Get should serve from it")
	}
	if m.counter("valkey_get_fallback") != 0 {
		t.Errorf("expected valkey_get_fallback=0 (L1 served before Valkey was contacted), got %d", m.counter("valkey_get_fallback"))
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
	m := &testMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: m, l1TTL: 0}

	got, err := s.Get(context.Background(), "nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on miss, got %+v", got)
	}
	if m.counter("valkey_miss") != 1 {
		t.Errorf("expected valkey_miss=1, got %d", m.counter("valkey_miss"))
	}
	if m.counter("valkey_get_fallback") != 0 {
		t.Errorf("expected valkey_get_fallback=0 on clean miss, got %d", m.counter("valkey_get_fallback"))
	}
}

func TestValkeyStorage_WriteThroughWarmsL1(t *testing.T) {
	stub := newStubValkeyClient()
	m := &testMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: m, l1TTL: 60 * time.Second}

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
	if m.counter("l1_hit") != 1 {
		t.Errorf("expected l1_hit=1, got %d", m.counter("l1_hit"))
	}
}

func TestValkeyStorage_L1TTLBoundedToEntryTTL(t *testing.T) {
	stub := newStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: &testMetrics{}, l1TTL: 60 * time.Second}

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
	m := &testMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: m, l1TTL: 0} // write-around

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

func TestValkeyStorage_SplitFallbackCounters(t *testing.T) {
	// Uses a broken stub — no Docker or live Valkey needed.
	// Set triggers valkey_set_fallback, which writes the entry to L1.
	// Get checks L1 first (L1-first reads) and finds the entry — incrementing l1_hit,
	// not valkey_get_fallback.
	stub := newBrokenStubValkeyClient()
	m := &testMetrics{}
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: m, l1TTL: 0}

	ctx := context.Background()
	entry := &Entry{StatusCode: 200, Payload: []byte("x"), TTL: time.Minute, CreatedAt: time.Now()}

	_ = s.Set(ctx, "k", entry)
	if m.counter("valkey_set_fallback") != 1 {
		t.Errorf("expected valkey_set_fallback=1, got %d", m.counter("valkey_set_fallback"))
	}
	if m.counter("valkey_get_fallback") != 0 {
		t.Errorf("expected valkey_get_fallback=0 after Set, got %d", m.counter("valkey_get_fallback"))
	}

	// L1-first: the entry was written to L1 by the Set fallback path, so Get returns
	// it from L1 without ever touching (broken) Valkey.
	_, _ = s.Get(ctx, "k")
	if m.counter("l1_hit") != 1 {
		t.Errorf("expected l1_hit=1, got %d", m.counter("l1_hit"))
	}
	if m.counter("valkey_get_fallback") != 0 {
		t.Errorf("expected valkey_get_fallback=0 (L1 served before Valkey check), got %d", m.counter("valkey_get_fallback"))
	}
	if m.counter("valkey_set_fallback") != 1 {
		t.Errorf("valkey_set_fallback should still be 1, got %d", m.counter("valkey_set_fallback"))
	}
}

func TestValkeyStorage_DeleteCleansL1EvenOnValkeyError(t *testing.T) {
	// Valkey is broken, so Set falls back to L1. Delete must still clean L1
	// regardless of the Expire error from Valkey.
	stub := newBrokenStubValkeyClient()
	lru := NewLRUStorage(64<<20, nil, metrics.Default)
	s := &ValkeyStorage{ring: stub, l1: lru, metrics: &testMetrics{}, l1TTL: 0}

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
