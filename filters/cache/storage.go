package cache

import (
	"context"
	"net/http"
	"time"
)

type Entry struct {
	StatusCode           int
	Payload              []byte
	Header               http.Header
	CreatedAt            time.Time
	TTL                  time.Duration
	StaleWhileRevalidate time.Duration
	ETag                 string
	LastModified         string
	VaryHeaders          []string
}

// IsStale returns true when the entry is past its TTL but still within the SWR window.
func (e *Entry) IsStale(now time.Time) bool {
	return now.After(e.CreatedAt.Add(e.TTL)) && now.Before(e.CreatedAt.Add(e.TTL+e.StaleWhileRevalidate))
}

type Storage interface {
	// Should return (nil, nil) if the key is not found
	Get(ctx context.Context, key string) (*Entry, error)

	Set(ctx context.Context, key string, entry *Entry) error

	Delete(ctx context.Context, key string) error
}
