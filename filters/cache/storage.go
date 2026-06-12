package cache

import (
	"context"
	"net/http"
	"time"
)

// Entry holds a cached HTTP response and the metadata required for freshness
// evaluation, conditional revalidation, and Age header calculation.
type Entry struct {
	// StatusCode is the HTTP status code of the cached response.
	StatusCode int
	// Payload is the serialised response body.
	Payload []byte
	// Header contains the response headers as stored at cache time.
	Header http.Header
	// CreatedAt is the wall-clock time at which this entry was stored.
	CreatedAt time.Time
	// TTL is the freshness lifetime of the entry.
	TTL time.Duration
	// StaleWhileRevalidate is the window after TTL expiry during which a stale
	// response may be served while a background revalidation is in flight.
	StaleWhileRevalidate time.Duration
	// ETag is the entity tag from the upstream response, used for conditional
	// revalidation (If-None-Match).
	ETag string
	// LastModified is the Last-Modified value from the upstream response, used
	// for conditional revalidation (If-Modified-Since).
	LastModified string
	// VaryHeaders lists the request header names captured from the original
	// request that were used to derive the cache key, matching the upstream
	// Vary response header.
	VaryHeaders []string
	// CorrectedInitialAge is the age correction term defined in RFC 9111 §4.2.3.
	// When zero, setAgeHeader falls back to the legacy elapsed-time formula.
	CorrectedInitialAge time.Duration
	// ResponseTime is the local time at which the upstream response was received,
	// used together with CorrectedInitialAge for RFC 9111 §4.2.3 age calculation.
	ResponseTime time.Time
	// StaleIfError extends the hard-expiry retention window so the entry remains
	// retrievable during upstream error periods (RFC 5861 stale-if-error).
	StaleIfError time.Duration
}

// IsStale reports whether the entry is past its TTL but still within
// the stale-while-revalidate window relative to now.
func (e *Entry) IsStale(now time.Time) bool {
	return now.After(e.CreatedAt.Add(e.TTL)) && now.Before(e.CreatedAt.Add(e.TTL+e.StaleWhileRevalidate))
}

// IsUsable reports whether the entry is fresh or within the stale-while-revalidate window.
// Entries past TTL+SWR are retained only for stale-if-error and must not be served.
func (e *Entry) IsUsable(now time.Time) bool {
	return now.Before(e.CreatedAt.Add(e.TTL + e.StaleWhileRevalidate))
}

// Storage is the backing store abstraction for cached entries.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Get returns the entry for key, or (nil, nil) if the key is not found.
	Get(ctx context.Context, key string) (*Entry, error)

	// Set stores or overwrites the entry for key.
	Set(ctx context.Context, key string, entry *Entry) error

	// Delete removes the entry for key. It is not an error if the key does not exist.
	Delete(ctx context.Context, key string) error
}
