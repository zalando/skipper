package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
	skpnet "github.com/zalando/skipper/net"
)

// valkeyClient is the subset of skpnet.ValkeyRingClient methods used by ValkeyStorage.
type valkeyClient interface {
	Get(ctx context.Context, key string) (string, error)
	SetWithExpire(ctx context.Context, key string, value string, expire time.Duration) error
	Expire(ctx context.Context, key string, d time.Duration) (int64, error)
}

var _ valkeyClient = (*skpnet.ValkeyRingClient)(nil)

// ValkeyStorage implements Storage using a ValkeyRingClient (L2) with
// automatic fallback to LRUStorage (L1) on any Valkey error.
type ValkeyStorage struct {
	ring    valkeyClient
	l1      *LRUStorage
	metrics metrics.Metrics
	l1TTL   time.Duration // max TTL for write-through L1 warming; 0 = write-around
}

// NewValkeyStorage creates a ValkeyStorage backed by ring (L2) with l1 as the
// fallback in-memory cache. m is used to record per-operation counters:
//
//   - valkey_miss          — clean cache miss (key not found in Valkey)
//   - valkey_get_fallback  — Valkey error on Get; L1 was consulted instead
//   - valkey_set_fallback  — Valkey error on Set; L1 was written instead
//
// Pass metrics.Default when no test-scoped metrics collector is needed.
func NewValkeyStorage(ring *skpnet.ValkeyRingClient, l1 *LRUStorage, m metrics.Metrics, l1TTL time.Duration) *ValkeyStorage {
	return &ValkeyStorage{ring: ring, l1: l1, metrics: m, l1TTL: l1TTL}
}

func (s *ValkeyStorage) Get(ctx context.Context, key string) (*Entry, error) {
	data, err := s.ring.Get(ctx, key)
	if err != nil {
		if valkey.IsValkeyNil(err) {
			s.metrics.IncCounter("valkey_miss")
			return nil, nil
		}
		s.metrics.IncCounter("valkey_get_fallback")
		log.WithError(err).Warn("cache: valkey Get failed, falling back to L1")
		return s.l1.Get(ctx, key)
	}
	var e Entry
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return nil, fmt.Errorf("cache: decode valkey entry: %w", err)
	}
	return &e, nil
}

func (s *ValkeyStorage) Set(ctx context.Context, key string, entry *Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache: encode valkey entry: %w", err)
	}

	valkeyTTL := entry.TTL + max(entry.StaleIfError, entry.StaleWhileRevalidate)
	if valkeyTTL <= 0 {
		valkeyTTL = time.Minute
	}

	if err := s.ring.SetWithExpire(ctx, key, string(data), valkeyTTL); err != nil {
		s.metrics.IncCounter("valkey_set_fallback")
		log.WithError(err).Warn("cache: valkey Set failed, falling back to L1")
		return s.l1.Set(ctx, key, entry)
	}

	// Write-through: warm L1 with a bounded TTL so pods can serve subsequent
	// requests from local memory without a Valkey round-trip.
	// Skip warming for non-cacheable entries (TTL <= 0) to avoid polluting L1
	// with entries that should not be served.
	if s.l1TTL > 0 && entry.TTL > 0 {
		warmTTL := min(s.l1TTL, entry.TTL)
		warmed := *entry
		warmed.TTL = warmTTL
		warmed.CreatedAt = time.Now()
		_ = s.l1.Set(ctx, key, &warmed)
	}
	return nil
}

func (s *ValkeyStorage) Delete(ctx context.Context, key string) error {
	// ValkeyRingClient exposes no DEL; use EXPIRE key -1 (immediate deletion per Valkey docs).
	// -1*time.Second is required: time.Duration(-1) is -1ns, which truncates to EXPIRE key 0.
	// Valkey errors are best-effort — L1 delete always runs.
	if _, err := s.ring.Expire(ctx, key, -1*time.Second); err != nil {
		log.WithError(err).Warn("cache: valkey Delete failed")
	}
	return s.l1.Delete(ctx, key)
}
