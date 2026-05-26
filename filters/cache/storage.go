package cache

import (
	"context"
	"net/http"
	"time"
)

// Entry holds cached HTTP response and metadata required for freshness
// evaluation, conditional revalidation, and Age header calculation.
type Entry struct {
	// StatusCode is HTTP status code of cached response.
	StatusCode int
	// Payload is serialised response body.
	Payload []byte
	// Header contains response headers as stored at cache time.
	Header http.Header
	// CreatedAt is wall-clock time at which this entry was stored.
	CreatedAt time.Time
	// TTL is freshness lifetime of entry.
	TTL time.Duration
	// StaleWhileRevalidate is window after TTL expiry during which stale
	// response may be served while background revalidation is in flight.
	StaleWhileRevalidate time.Duration
	// ETag is entity tag from upstream response, used for conditional
	// revalidation (If-None-Match).
	ETag string
	// LastModified is Last-Modified value from upstream response, used
	// for conditional revalidation (If-Modified-Since).
	LastModified string
	// VaryHeaders lists request header names captured from original
	// request that were used to derive cache key, matching upstream
	// Vary response header.
	VaryHeaders []string
	// CorrectedInitialAge is age correction term (RFC 9111 §4.2.3).
	// When zero, setAgeHeader falls back to legacy elapsed-time formula.
	CorrectedInitialAge time.Duration
	// ResponseTime is local time at which upstream response was received,
	// used together with CorrectedInitialAge for age calculation.
	ResponseTime time.Time
	// StaleIfError extends hard-expiry retention window so entry remains
	// retrievable during upstream error periods (RFC 5861 stale-if-error).
	StaleIfError time.Duration
}

// IsStale reports whether entry is past its TTL but still within
// stale-while-revalidate window relative to now.
func (e *Entry) IsStale(now time.Time) bool {
	return now.After(e.CreatedAt.Add(e.TTL)) && now.Before(e.CreatedAt.Add(e.TTL+e.StaleWhileRevalidate))
}

// IsUsable reports whether entry is fresh or within stale-while-revalidate window.
// Entries past TTL+SWR are retained only for stale-if-error and must not be served.
func (e *Entry) IsUsable(now time.Time) bool {
	return now.Before(e.CreatedAt.Add(e.TTL + e.StaleWhileRevalidate))
}

// Storage is backing store abstraction for cached entries.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Get returns entry for key, or (nil, nil) if key is not found.
	Get(ctx context.Context, key string) (*Entry, error)

	// Set stores or overwrites entry for key.
	Set(ctx context.Context, key string, entry *Entry) error

	// Delete removes entry for key. It is not an error if key does not exist.
	Delete(ctx context.Context, key string) error
}
