package cache

import (
	"container/list"
	"hash/fnv"
	"sync"
)

// shardCount is set to 256 to balance lock contention with baseline memory overhead.
// As a power of 2, it allows Go compiler to optimize modulo operations into
// fast bitwise AND operations.
const shardCount = 256

// lruItem holds raw []byte instead of Go struct pointers.
// This eliminates Garbage Collector (GC) scanning overhead and allows us to
// enforce strict immutability of cached responses.
type lruItem struct {
	key  string
	data []byte
	size int64
}

// lruShard is a bounded LRU cache protected by a single mutex.
type lruShard struct {
	maxBytes     int64
	currentBytes int64
	ll           *list.List
	cache        map[string]*list.Element
	onEvict      func() // called once per evicted item; nil = no-op

	// mu is a standard Mutex (not RWMutex) because LRU reads (Get)
	// modify linked list. Contention is mitigated by sharding.
	mu sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()

	size := int64(len(data))

	// If item exceeds shard's capacity, we cannot cache it.
	// We must also remove any existing smaller version of this key
	// to prevent serving stale or inconsistent data.
	if size > s.maxBytes {
		if ele, ok := s.cache[key]; ok {
			s.ll.Remove(ele)
			item := ele.Value.(*lruItem)
			delete(s.cache, key)
			s.currentBytes -= item.size
		}
		return
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

	for s.currentBytes > s.maxBytes {
		s.removeOldest()
	}
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

func (s *lruShard) removeOldest() {
	ele := s.ll.Back()
	if ele != nil {
		s.ll.Remove(ele)
		item := ele.Value.(*lruItem)
		delete(s.cache, item.key)
		s.currentBytes -= item.size
		if s.onEvict != nil {
			s.onEvict()
		}
	}
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

// getShard routes a key to its shard via FNV-1a. Keys are expected to be
// pre-hashed (e.g. SHA-256) by the caller.
func (s *ShardedByteLRU) getShard(key string) *lruShard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return s.shards[h.Sum32()%shardCount]
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
