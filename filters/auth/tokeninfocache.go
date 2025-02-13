package auth

import (
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
)

type (
	tokeninfoCache struct {
		client  tokeninfoClient
		metrics metrics.Metrics
		size    int
		ttl     time.Duration
		now     func() time.Time

		cache sync.Map     // map[string]*entry
		count atomic.Int64 // estimated number of cached entries, see https://github.com/golang/go/issues/20680
		quit  chan struct{}
	}

	entry struct {
		expiresAt     time.Time
		info          map[string]any
		infoExpiresAt time.Time
	}
)

var _ tokeninfoClient = &tokeninfoCache{}

const expiresInField = "expires_in"

func newTokeninfoCache(client tokeninfoClient, metrics metrics.Metrics, size int, ttl time.Duration) *tokeninfoCache {
	c := &tokeninfoCache{
		client:  client,
		metrics: metrics,
		size:    size,
		ttl:     ttl,
		now:     time.Now,
		quit:    make(chan struct{}),
	}
	go c.evictLoop()
	return c
}

func (c *tokeninfoCache) Close() {
	c.client.Close()
	close(c.quit)
}

func (c *tokeninfoCache) getTokeninfo(token string, ctx filters.FilterContext) (map[string]any, error) {
	if cached := c.cached(token); cached != nil {
		return cached, nil
	}

	info, err := c.client.getTokeninfo(token, ctx)
	if err == nil {
		c.tryCache(token, info)
	}
	return info, err
}

func (c *tokeninfoCache) cached(token string) map[string]any {
	if v, ok := c.cache.Load(token); ok {
		now := c.now()
		e := v.(*entry)
		if now.Before(e.expiresAt) {
			// Clone cached value because callers may modify it,
			// see e.g. [OAuthConfig.GrantTokeninfoKeys] and [grantFilter.setupToken].
			info := maps.Clone(e.info)

			info[expiresInField] = e.infoExpiresAt.Sub(now).Truncate(time.Second).Seconds()
			return info
		}
	}
	return nil
}

func (c *tokeninfoCache) tryCache(token string, info map[string]any) {
	expiresIn := expiresIn(info)
	if expiresIn <= 0 {
		return
	}

	now := c.now()
	e := &entry{
		info:          info,
		infoExpiresAt: now.Add(expiresIn),
	}

	if c.ttl > 0 && expiresIn > c.ttl {
		e.expiresAt = now.Add(c.ttl)
	} else {
		e.expiresAt = e.infoExpiresAt
	}

	if _, loaded := c.cache.Swap(token, e); !loaded {
		c.count.Add(1)
	}
}

func (c *tokeninfoCache) evictLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.quit:
			return
		case <-ticker.C:
			c.evict()
		}
	}
}

func (c *tokeninfoCache) evict() {
	now := c.now()
	// Evict expired entries
	c.cache.Range(func(key, value any) bool {
		e := value.(*entry)
		if now.After(e.expiresAt) {
			if c.cache.CompareAndDelete(key, value) {
				c.count.Add(-1)
			}
		}
		return true
	})

	// Evict random entries until the cache size is within limits
	if c.count.Load() > int64(c.size) {
		c.cache.Range(func(key, value any) bool {
			if c.cache.CompareAndDelete(key, value) {
				c.count.Add(-1)
			}
			return c.count.Load() > int64(c.size)
		})
	}

	if c.metrics != nil {
		c.metrics.UpdateGauge("tokeninfocache.count", float64(c.count.Load()))
	}
}

// Returns the lifetime of the access token if present.
// See https://datatracker.ietf.org/doc/html/rfc6749#section-4.2.2
func expiresIn(info map[string]any) time.Duration {
	if v, ok := info[expiresInField]; ok {
		// https://pkg.go.dev/encoding/json#Unmarshal stores JSON numbers in float64
		if v, ok := v.(float64); ok {
			return time.Duration(v) * time.Second
		}
	}
	return 0
}
