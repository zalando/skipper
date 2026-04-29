package cache

import (
	"context"
	"net/http"
	"time"
)

type Entry struct {
	StatusCode int
	Payload    []byte
	Header     http.Header
	CreatedAt  time.Time // needed to distinguish between a "Soft Expiry" (time for background refresh) and a "Hard Expiry" (too old to serve even as stale)
	TTL        time.Duration
}

type Storage interface {
	// Get returns the entry. It should return (nil, nil) if the key is not found.
	Get(ctx context.Context, key string) (*Entry, error)

	// Set stores the entry.
	Set(ctx context.Context, key string, entry *Entry) error

	// Delete removes an entry (used for manual invalidation/cleanup).
	Delete(ctx context.Context, key string) error
}
