package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

// LRUStorage wraps ShardedByteLRU and implements Storage.
// It owns all cache semantics (serialisation, TTL expiry); ShardedByteLRU
// remains a pure byte store.
type LRUStorage struct {
	lru *ShardedByteLRU
}

// NewLRUStorage returns an LRUStorage backed by a ShardedByteLRU sized to totalMaxBytes.
func NewLRUStorage(totalMaxBytes int64, onEvict func()) *LRUStorage {
	return &LRUStorage{
		lru: NewShardedByteLRU(totalMaxBytes, onEvict),
	}
}

func (s *LRUStorage) Get(_ context.Context, key string) (*Entry, error) {
	data, ok := s.lru.Get(key)
	if !ok {
		return nil, nil
	}

	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("cache: decode entry: %w", err)
	}

	// TTL==0 means "always stale but keep for conditional revalidation" (no-cache entries).
	// Only hard-evict when TTL>0 and the entry has passed its full retention window.
	if e.TTL > 0 {
		sieWindow := max(e.StaleIfError, e.StaleWhileRevalidate)
		if time.Now().After(e.CreatedAt.Add(e.TTL + sieWindow)) {
			s.lru.Delete(key)
			return nil, nil
		}
	}

	return &e, nil
}

func (s *LRUStorage) Set(_ context.Context, key string, entry *Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache: encode entry: %w", err)
	}
	if s.lru.ExceedsShard(data) {
		log.WithFields(log.Fields{
			"key":        key,
			"size_bytes": len(data),
			"shard_max":  s.lru.shards[0].maxBytes,
		}).Warn("cache: entry exceeds shard capacity and will not be stored")
	}
	s.lru.Set(key, data)
	return nil
}

func (s *LRUStorage) Delete(_ context.Context, key string) error {
	s.lru.Delete(key)
	return nil
}
