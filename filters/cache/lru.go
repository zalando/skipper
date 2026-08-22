package cache

import (
	"container/list"
	"sync"

	"github.com/cespare/xxhash/v2"
)

// shardCount balances lock contention against memory overhead.
// Power of 2 lets the compiler reduce modulo to bitwise AND.
const shardCount = 256

// lruItem stores raw bytes to avoid GC scanning of pointer-heavy response structs.
type lruItem struct {
	key  string
	data []byte
	size int64
}

// lruShard is a bounded LRU cache protected by a single mutex.
// Mutex (not RWMutex) is required because Get promotes entries in the list.
type lruShard struct {
	// Immutable after construction; not protected by mu.
	maxBytes int64
	onEvict  func() // called once per evicted item

	mu           sync.Mutex
	currentBytes int64
	ll           *list.List
	cache        map[string]*list.Element
}

func newLRUShard(maxBytes int64, onEvict func()) *lruShard {
	return &lruShard{
		maxBytes: maxBytes,
		ll:       list.New(),
		cache:    make(map[string]*list.Element),
		onEvict:  onEvict,
	}
}

func (s *lruShard) set(key string, data []byte) {
	evictions := s.setLocked(key, data)
	// onEvict is called outside the lock: callbacks may call Bytes() which acquires shard mutexes.
	if s.onEvict != nil {
		for range evictions {
			s.onEvict()
		}
	}
}

func (s *lruShard) setLocked(key string, data []byte) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	size := int64(len(data))

	// Entry exceeds shard capacity; evict any existing version to avoid serving stale data.
	if size > s.maxBytes {
		if ele, ok := s.cache[key]; ok {
			s.ll.Remove(ele)
			item := ele.Value.(*lruItem)
			delete(s.cache, key)
			s.currentBytes -= item.size
		}
		return 0
	}

	if ele, ok := s.cache[key]; ok {
		s.ll.MoveToFront(ele)
		item := ele.Value.(*lruItem)
		s.currentBytes -= item.size
		item.data = data
		item.size = size
		s.currentBytes += size
	} else {
		ele := s.ll.PushFront(&lruItem{key, data, size})
		s.cache[key] = ele
		s.currentBytes += size
	}

	var evictions int
	for s.currentBytes > s.maxBytes {
		if s.removeOldest() {
			evictions++
		}
	}
	return evictions
}

func (s *lruShard) get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ele, ok := s.cache[key]; ok {
		s.ll.MoveToFront(ele)
		item := ele.Value.(*lruItem)

		dst := make([]byte, len(item.data))
		copy(dst, item.data)
		return dst, true
	}
	return nil, false
}

func (s *lruShard) delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ele, ok := s.cache[key]; ok {
		s.ll.Remove(ele)
		item := ele.Value.(*lruItem)
		delete(s.cache, key)
		s.currentBytes -= item.size
	}
}

// removeOldest evicts the LRU item. Returns false when the list is empty.
// Caller must invoke onEvict outside the lock.
func (s *lruShard) removeOldest() bool {
	ele := s.ll.Back()
	if ele == nil {
		return false
	}
	s.ll.Remove(ele)
	item := ele.Value.(*lruItem)
	delete(s.cache, item.key)
	s.currentBytes -= item.size
	return true
}

// ShardedByteLRU manages an array of LRU shards to reduce lock contention
// in highly concurrent reverse proxy environments.
type ShardedByteLRU struct {
	shards [shardCount]*lruShard
}

// NewShardedByteLRU distributes total allowed memory evenly across all shards.
func NewShardedByteLRU(totalMaxBytes int64, onEvict func()) *ShardedByteLRU {
	s := &ShardedByteLRU{}
	bytesPerShard := totalMaxBytes / int64(shardCount)

	for i := 0; i < shardCount; i++ {
		s.shards[i] = newLRUShard(bytesPerShard, onEvict)
	}
	return s
}

// getShard routes a key to its shard via xxhash. Keys are expected to be
// pre-hashed (e.g. SHA-256) by the caller.
func (s *ShardedByteLRU) getShard(key string) *lruShard {
	return s.shards[xxhash.Sum64String(key)%shardCount]
}

// ExceedsShard reports whether data is too large to fit in any shard.
func (s *ShardedByteLRU) ExceedsShard(data []byte) bool {
	return int64(len(data)) > s.shards[0].maxBytes
}

func (s *ShardedByteLRU) Set(key string, data []byte) {
	s.getShard(key).set(key, data)
}

func (s *ShardedByteLRU) Get(key string) ([]byte, bool) {
	return s.getShard(key).get(key)
}

func (s *ShardedByteLRU) Delete(key string) {
	s.getShard(key).delete(key)
}

// Bytes returns the total number of bytes currently stored across all shards.
func (s *ShardedByteLRU) Bytes() int64 {
	var total int64
	for _, shard := range s.shards {
		shard.mu.Lock()
		total += shard.currentBytes
		shard.mu.Unlock()
	}
	return total
}
