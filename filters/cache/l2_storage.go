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
// is used as a fallback. L2Client Get errors are returned with any eligible
// local stale-if-error candidate, which must not be served without an origin failure.
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
//   - l2_get_error     — L2 read error; origin required before local SIE fallback
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
	entry, _, err := s.getWithLookup(ctx, key, true)
	return entry, err
}

// GetSilent behaves like Get but suppresses the legacy l1_hit, l2_hit,
// l2_miss and l2_get_error lookup outcomes and their labeled counterparts.
// L2 operation and processing metrics are still recorded. Use it for internal
// plumbing lookups — e.g. the Vary-sentinel check in filter.go — that are
// not themselves a caching decision: every request pays for that lookup,
// so routing it through Get would have silently counted it as a real cache
// outcome on every single request.
func (s *L2Storage) GetSilent(ctx context.Context, key string) (*Entry, error) {
	entry, _, err := s.getWithLookup(ctx, key, false)
	return entry, err
}

func (s *L2Storage) getWithLookup(ctx context.Context, key string, recordMetrics bool) (*Entry, cacheLookup, error) {
	lookup := cacheLookup{tier: "none"}
	// L1-first: serve from local memory when the write-through warming populated it.
	// LRUStorage.Get returns entries within TTL + max(StaleIfError, StaleWhileRevalidate).
	// Only serve fresh, warm L1 hits; stale entries fall through to L2 so a fresher copy
	// written by another instance is not bypassed. Preserve TTL==0 entries'
	// conditional-revalidation behavior.
	local, l1Err := s.l1.Get(ctx, key)
	lookup.failed = l1Err != nil
	if l1Err == nil && local != nil {
		now := time.Now()
		if !local.IsStale(now) && (local.TTL == 0 || now.Before(local.CreatedAt.Add(local.TTL))) &&
			(local.L1FreshUntil.IsZero() || now.Before(local.L1FreshUntil)) {
			if recordMetrics {
				s.metrics.IncCounter("cache.l1_hit")
				if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("l1_hit", statusCodeClass(local.StatusCode))
				}
			}
			lookup.tier = "l1"
			return local, lookup, nil
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
			// A clean miss may be a deletion by another process. Do not retain a
			// local candidate that a later failed lookup could resurrect.
			_ = s.l1.Delete(ctx, key)
			if recordMetrics {
				s.metrics.IncCounter("cache.l2_miss")
				if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("l2_miss", "n/a")
				}
			}
			return nil, lookup, nil
		}
		if recordMetrics {
			s.metrics.IncCounter("cache.l2_get_error")
			if p, ok := s.metrics.(interface{ IncCacheResult(string, string) }); ok {
				p.IncCacheResult("l2_get_error", "n/a")
			}
		}
		log.WithError(err).Error("cache: L2 Get failed, fetching origin before local stale-if-error fallback")
		lookup.failed = true
		candidate := localSIECandidate(local)
		if candidate != nil {
			lookup.tier = "l1"
		}
		return candidate, lookup, fmt.Errorf("cache: L2 Get: %w", err)
	}
	var stored Entry
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		s.observeProcessingError(ctx, "get", "decode")
		if p, ok := s.metrics.(interface{ IncStorageError(string, string) }); ok {
			p.IncStorageError("l2", "get")
		}
		log.WithError(err).Error("cache: failed to decode L2 entry")
		lookup.failed = true
		candidate := localSIECandidate(local)
		if candidate != nil {
			lookup.tier = "l1"
		}
		return candidate, lookup, fmt.Errorf("cache: decode L2 entry: %w", err)
	}
	e := &stored
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
	s.warmL1(ctx, key, e)
	lookup.tier = "l2"
	return e, lookup, nil
}

func localSIECandidate(entry *Entry) *Entry {
	if entry != nil && entry.StatusCode < 500 && entry.StaleIfError > 0 &&
		!time.Now().After(entry.CreatedAt.Add(entry.TTL+entry.StaleIfError)) {
		return entry
	}
	return nil
}

func (s *L2Storage) warmL1(ctx context.Context, key string, entry *Entry) {
	now := time.Now()
	if s.l1TTL > 0 && entry.TTL > 0 && now.Before(entry.CreatedAt.Add(entry.TTL)) {
		warmed := *entry
		warmed.L1FreshUntil = now.Add(s.l1TTL)
		if expiry := entry.CreatedAt.Add(entry.TTL); expiry.Before(warmed.L1FreshUntil) {
			warmed.L1FreshUntil = expiry
		}
		_ = s.l1.Set(ctx, key, &warmed)
	}
}

func (s *L2Storage) Set(ctx context.Context, key string, entry *Entry) error {
	shared := *entry
	shared.L1FreshUntil = time.Time{}
	data, err := json.Marshal(&shared)
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
		return s.l1.Set(ctx, key, &shared)
	}

	// Write-through: warm L1 with a bounded TTL so pods can serve subsequent
	// requests from local memory without a L2 round-trip.
	// Skip warming for non-cacheable entries (TTL <= 0) to avoid polluting L1
	// with entries that should not be served.
	s.warmL1(ctx, key, entry)
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
