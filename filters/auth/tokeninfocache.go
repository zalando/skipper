package auth

import (
	"container/list"
	"sync"
	"time"

	"github.com/zalando/skipper/filters"
)

type (
	tokeninfoCache struct {
		client tokeninfoClient
		size   int
		ttl    time.Duration
		now    func() time.Time

		mu    sync.Mutex
		cache map[string]*entry
		// least recently used token at the end
		history *list.List
	}

	entry struct {
		cachedAt  time.Time
		expiresAt time.Time
		info      map[string]any
		// reference in the history
		href *list.Element
	}
)

var _ tokeninfoClient = &tokeninfoCache{}

const expiresInField = "expires_in"

func newTokeninfoCache(client tokeninfoClient, size int, ttl time.Duration) *tokeninfoCache {
	return &tokeninfoCache{
		client:  client,
		size:    size,
		ttl:     ttl,
		now:     time.Now,
		cache:   make(map[string]*entry, size),
		history: list.New(),
	}
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
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.cache[token]; ok {
		if now.Before(e.expiresAt) {
			c.history.MoveToFront(e.href)
			// It might be ok to return cached value
			// without adjusting "expires_in" to avoid copy
			// when "expires_in" did not change (same second)
			// or for small TTL values
			info := shallowCopyOf(e.info)

			elapsed := now.Sub(e.cachedAt).Truncate(time.Second).Seconds()
			info[expiresInField] = info[expiresInField].(float64) - elapsed
			return info
		} else {
			// remove expired
			delete(c.cache, token)
			c.history.Remove(e.href)
		}
	}
	return nil
}

func (c *tokeninfoCache) tryCache(token string, info map[string]any) {
	expiresIn := expiresIn(info)
	if expiresIn <= 0 {
		return
	}
	if c.ttl > 0 && expiresIn > c.ttl {
		expiresIn = c.ttl
	}

	now := c.now()
	expiresAt := now.Add(expiresIn)

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.cache[token]; ok {
		// update
		e.cachedAt = now
		e.expiresAt = expiresAt
		e.info = info
		c.history.MoveToFront(e.href)
		return
	}

	// create
	c.cache[token] = &entry{
		cachedAt:  now,
		expiresAt: expiresAt,
		info:      info,
		href:      c.history.PushFront(token),
	}

	// remove least used
	if len(c.cache) > c.size {
		leastUsed := c.history.Back()
		delete(c.cache, leastUsed.Value.(string))
		c.history.Remove(leastUsed)
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

func shallowCopyOf(info map[string]any) map[string]any {
	m := make(map[string]any, len(info))
	for k, v := range info {
		m[k] = v
	}
	return m
}
