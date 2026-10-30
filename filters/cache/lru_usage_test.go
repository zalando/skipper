package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShardedByteLRUUsage(t *testing.T) {
	lru := NewShardedByteLRU(1<<20, func() {})
	lru.Set("a", make([]byte, 10))
	lru.Set("b", make([]byte, 20))

	bytes, entries := lru.Usage()
	assert.Equal(t, int64(2), entries)
	assert.Equal(t, lru.Bytes(), bytes)
	assert.Positive(t, bytes)
}
