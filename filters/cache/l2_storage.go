package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
)

const defaultMinTTL = time.Minute

type L2Client interface {
	Get(ctx context.Context, key string) (string, error)
	SetWithExpire(ctx context.Context, key string, value string, expire time.Duration) error
	Expire(ctx context.Context, key string, d time.Duration) (int64, error)
	Del(ctx context.Context, key string) (int64, error)
}

// L2Storage implements Storage using a L2Client with
// write-through warming of LRUStorage (L1). On L2Client Set errors, L1
// is used as a fallback. On L2Client Get errors, the request is treated
// as a miss and fetched from origin.
type L2Storage struct {
	l2client L2Client
	l1       *LRUStorage
	metrics  metrics.Metrics
	l1TTL    time.Duration // max TTL for write-through L1 warming; 0 = write-around

	isNoErr func(error) bool
}

// NewL2Storage creates a *L2Storage backed by ring (L2) with l1 as the
// fallback in-memory cache. m is used to record per-operation counters:
//
//   - l1_hit           — L1 returned a warm entry; L2 not consulted
//   - l2_miss          — clean cache miss (key not found in L2)
//   - l2_get_error     — L2 error on Get; treated as a cache miss
//   - l2_set_fallback  — L2 error on Set; entry written to L1 only
//   - l2_hit           — successful L2 Get (entry returned from L2)
//
// Pass metrics.Default when no test-scoped metrics collector is needed.
func NewL2Storage(l2client L2Client, l1 *LRUStorage, m metrics.Metrics, l1TTL time.Duration, isNoErr func(error) bool) *L2Storage {
	if l1TTL < 0 {
		log.Warnf("cache: NewL2Storage: l1TTL must be >= 0: %v", l1TTL)
		l1TTL = defaultMinTTL
	}
	return &L2Storage{
		l2client: l2client,
		l1:       l1,
		metrics:  m,
		l1TTL:    l1TTL,
		isNoErr:  isNoErr,
	}
}

func (s *L2Storage) Get(ctx context.Context, key string) (*Entry, error) {
	// L1-first: serve from local memory when the write-through warming populated it.
	// LRUStorage.Get returns entries within TTL + max(StaleIfError, StaleWhileRevalidate).
	// Only serve fresh L1 hits; stale entries fall through to L2 so a fresher copy
	// written by another instance is not bypassed.
	if e, err := s.l1.Get(ctx, key); err == nil && e != nil {
		if !e.IsStale(time.Now()) {
			s.metrics.IncCounter("cache.l1_hit")
			return e, nil
		}
		// Stale L1 entry — fall through to L2.
	}

	data, err := s.l2client.Get(ctx, key)
	if err != nil {
		if s.isNoErr(err) {
			s.metrics.IncCounter("cache.l2_miss")
			return nil, nil
		}
		s.metrics.IncCounter("cache.l2_get_error")
		log.WithError(err).Error("cache: L2 Get failed, treating as miss")
		return nil, nil
	}
	var e Entry
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return nil, fmt.Errorf("cache: decode L2 entry: %w", err)
	}
	s.metrics.IncCounter("cache.l2_hit")
	// Write-through: warm L1 so subsequent requests on this process avoid L2 round-trips.
	// Use remaining freshness to avoid extending L1 beyond L2's actual expiry.
	if s.l1TTL > 0 && e.TTL > 0 {
		if remaining := e.TTL - time.Since(e.CreatedAt); remaining > 0 {
			warmed := e
			warmed.TTL = min(s.l1TTL, remaining)
			warmed.CreatedAt = time.Now()
			_ = s.l1.Set(ctx, key, &warmed)
		}
	}
	return &e, nil
}

func (s *L2Storage) Set(ctx context.Context, key string, entry *Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache: encode L2 entry: %w", err)
	}

	l2TTL := entry.TTL + max(entry.StaleIfError, entry.StaleWhileRevalidate)
	if l2TTL <= 0 {
		l2TTL = time.Minute
	}

	if err := s.l2client.SetWithExpire(ctx, key, string(data), l2TTL); err != nil {
		s.metrics.IncCounter("cache.l2_set_fallback")
		log.WithError(err).Error("cache: L2 Set failed, falling back to L1")
		return s.l1.Set(ctx, key, entry)
	}

	// Write-through: warm L1 with a bounded TTL so pods can serve subsequent
	// requests from local memory without a L2 round-trip.
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

func (s *L2Storage) Delete(ctx context.Context, key string) error {
	// L2 errors are best-effort — L1 delete always runs.
	// Note: only the local process's L1 is cleared. Other Skipper processes in the
	// fleet retain their own L1 copies until --cache-l1-ttl expires naturally.
	if _, err := s.l2client.Del(ctx, key); err != nil {
		log.WithError(err).Error("cache: L2 Delete failed")
		s.metrics.IncCounter("cache.storage_error")
	}
	return s.l1.Delete(ctx, key)
}
