package cache

import (
	"context"
	"net/http"
	"time"
)

type l2Purpose uint8

const (
	l2PurposeUnknown l2Purpose = iota
	l2PurposeResponse
	l2PurposeMetadata
)

type l2PurposeContextKey struct{}

func withL2Purpose(ctx context.Context, purpose l2Purpose) context.Context {
	return context.WithValue(ctx, l2PurposeContextKey{}, purpose)
}

func l2PurposeFromContext(ctx context.Context) string {
	switch purpose, _ := ctx.Value(l2PurposeContextKey{}).(l2Purpose); purpose {
	case l2PurposeResponse:
		return "response"
	case l2PurposeMetadata:
		return "metadata"
	default:
		return "unknown"
	}
}

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

	// GetSilent behaves like Get but suppresses the legacy l1_hit, l2_hit,
	// l2_miss and l2_get_error lookup outcomes and their labeled counterparts.
	// L2 operation and processing metrics are still recorded.
	// Use it for internal plumbing lookups — e.g. the
	// Vary-sentinel check in filter.go — that are not themselves a caching
	// decision and would otherwise pollute those metrics on every request.
	GetSilent(ctx context.Context, key string) (*Entry, error)

	// Set stores or overwrites the entry for key.
	Set(ctx context.Context, key string, entry *Entry) error

	// Delete removes the entry for key. It is not an error if the key does not exist.
	Delete(ctx context.Context, key string) error
}
