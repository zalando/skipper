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

// L2Client provides the key-value operations required by L2Storage.
type L2Client interface {
	Get(ctx context.Context, key string) (string, error)
	SetWithExpire(ctx context.Context, key string, value string, expire time.Duration) error
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
	return s.get(ctx, key, true)
}

// GetSilent behaves like Get but suppresses the legacy l1_hit, l2_hit,
// l2_miss and l2_get_error lookup outcomes and their labeled counterparts.
// L2 operation and processing metrics are still recorded. Use it for internal
// plumbing lookups — e.g. the Vary-sentinel check in filter.go — that are
// not themselves a caching decision: every request pays for that lookup,
// so routing it through Get would have silently counted it as a real cache
// outcome on every single request.
func (s *L2Storage) GetSilent(ctx context.Context, key string) (*Entry, error) {
	return s.get(ctx, key, false)
}

func (s *L2Storage) get(ctx context.Context, key string, recordMetrics bool) (*Entry, error) {
	// L1-first: serve from local memory when the write-through warming populated it.
	// LRUStorage.Get returns entries within TTL + max(StaleIfError, StaleWhileRevalidate).
	// Only serve fresh L1 hits; stale entries fall through to L2 so a fresher copy
	// written by another instance is not bypassed.
	if e, err := s.l1.Get(ctx, key); err == nil && e != nil {
		if !e.IsStale(time.Now()) {
			if recordMetrics {
				s.metrics.IncCounter("cache.l1_hit")
				if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("l1_hit", statusCodeClass(e.StatusCode))
				}
			}
			return e, nil
		}
		// Stale L1 entry — fall through to L2.
	}

	l2Start := time.Now()
	data, err := s.l2client.Get(ctx, key)
	s.metrics.MeasureSince("cache.l2_get_duration", l2Start)
	cleanMiss := err != nil && s.isNoErr(err)
	outcome := "success"
	if cleanMiss {
		outcome = "miss"
	} else if err != nil {
		outcome = "error"
	}
	s.observeOperation(ctx, "get", outcome)
	if err != nil {
		if cleanMiss {
			if recordMetrics {
				s.metrics.IncCounter("cache.l2_miss")
				if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("l2_miss", "n/a")
				}
			}
			return nil, nil
		}
		if recordMetrics {
			s.metrics.IncCounter("cache.l2_get_error")
			if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
				p.IncCacheResult("l2_get_error", "n/a")
			}
		}
		log.WithError(err).Error("cache: L2 Get failed, treating as miss")
		return nil, nil
	}
	var e Entry
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		s.observeProcessingError(ctx, "get", "decode")
		if p, ok := s.metrics.(interface{ IncStorageError(string, string) }); ok {
			p.IncStorageError("l2", "get")
		}
		return nil, fmt.Errorf("cache: decode L2 entry: %w", err)
	}
	if recordMetrics {
		s.metrics.IncCounter("cache.l2_hit")
		if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
			p.IncCacheResult("l2_hit", statusCodeClass(e.StatusCode))
		}
	}
	// Write-through: warm L1 so subsequent requests on this process avoid L2 round-trips.
	// Use remaining freshness to avoid extending L1 beyond L2's actual expiry.
	// Unconditional (even for the silent path): this is a caching side effect of
	// having read the entry, not an outcome metric, and skipping it would make
	// sentinel lookups needlessly re-fetch from L2 every time.
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
		s.observeProcessingError(ctx, "set", "encode")
		if p, ok := s.metrics.(interface{ IncStorageError(string, string) }); ok {
			p.IncStorageError("l2", "set")
		}
		return fmt.Errorf("cache: encode L2 entry: %w", err)
	}

	l2TTL := entry.TTL + max(entry.StaleIfError, entry.StaleWhileRevalidate)
	if l2TTL <= 0 {
		l2TTL = time.Minute
	}

	l2Start := time.Now()
	err = s.l2client.SetWithExpire(ctx, key, string(data), l2TTL)
	s.metrics.MeasureSince("cache.l2_set_duration", l2Start)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.observeOperation(ctx, "set", outcome)
	if err != nil {
		s.metrics.IncCounter("cache.l2_set_fallback")
		if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
			p.IncCacheResult("l2_set_fallback", statusCodeClass(entry.StatusCode))
		}
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
	l2Start := time.Now()
	_, err := s.l2client.Del(ctx, key)
	s.metrics.MeasureSince("cache.l2_delete_duration", l2Start)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.observeOperation(ctx, "delete", outcome)
	if err != nil {
		log.WithError(err).Error("cache: L2 Delete failed")
		if p, ok := s.metrics.(interface{ IncStorageError(string, string) }); ok {
			p.IncStorageError("l2", "delete")
		}
	}
	return s.l1.Delete(ctx, key)
}

func (s *L2Storage) observeOperation(ctx context.Context, operation, outcome string) {
	if observer, ok := s.metrics.(metrics.CacheL2Observer); ok {
		observer.IncCacheL2Operation(operation, l2PurposeFromContext(ctx), outcome)
	}
}

func (s *L2Storage) observeProcessingError(ctx context.Context, operation, stage string) {
	if observer, ok := s.metrics.(metrics.CacheL2Observer); ok {
		observer.IncCacheL2ProcessingError(operation, l2PurposeFromContext(ctx), stage)
	}
}
