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

// ValkeyStorage implements Storage using a ValkeyRingClient (L2) with
// automatic fallback to LRUStorage (L1) on any Valkey error.
// This handles brief unavailability during rolling Valkey node updates
// without dropping requests or hitting the origin.
type ValkeyStorage struct {
	ring *skpnet.ValkeyRingClient
	l1   *LRUStorage
}

func NewValkeyStorage(ring *skpnet.ValkeyRingClient, l1 *LRUStorage) *ValkeyStorage {
	return &ValkeyStorage{ring: ring, l1: l1}
}

func (s *ValkeyStorage) Get(ctx context.Context, key string) (*Entry, error) {
	data, err := s.ring.Get(ctx, key)
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		metrics.Default.IncCounter("valkey_fallback")
		log.WithError(err).Debug("cache: valkey Get failed, falling back to L1")
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

	ttl := entry.TTL + max(entry.StaleIfError, entry.StaleWhileRevalidate)
	if ttl <= 0 {
		ttl = time.Minute
	}

	if err := s.ring.SetWithExpire(ctx, key, string(data), ttl); err != nil {
		metrics.Default.IncCounter("valkey_fallback")
		log.WithError(err).Debug("cache: valkey Set failed, falling back to L1")
		return s.l1.Set(ctx, key, entry)
	}
	return nil
}

func (s *ValkeyStorage) Delete(ctx context.Context, key string) error {
	// ValkeyRingClient exposes no DEL; a negative TTL triggers immediate expiry.
	// Valkey errors here are best-effort — the L1 delete below always runs.
	if _, err := s.ring.Expire(ctx, key, -1); err != nil {
		log.WithError(err).Debug("cache: valkey Delete failed")
	}
	return s.l1.Delete(ctx, key)
}
