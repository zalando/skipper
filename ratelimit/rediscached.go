package ratelimit

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aryszka/forget"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
)

const (
	redisCacheNamespace      = "redis-ratelimit-cache"
	defaultCachePeriodFactor = 256
)

// appr. 65B + the key size + cache overhead ~120B
// the keys can have very varying size, e.g. with auth tokens close to 1kB
// => recommended cache chunk size: 256B
// (https://pkg.go.dev/github.com/aryszka/forget?utm_source=godoc#hdr-Memory)
//
type cacheItem struct {
	// time.Time implements gob.GobEncoder and gob.GobDecoder
	LastSync time.Time
	Oldest   time.Time

	SyncedSum int
	LocalSum  int
	FailOpen  bool
}

type redisCache interface {
	get(string) (cacheItem, bool)
	set(string, cacheItem)
}

type cache struct {
	cache     *forget.CacheSpaces
	namespace string
	ttl       time.Duration
}

type clusterLimitRedisCached struct {
	redis       *net.RedisRingClient
	cache       redisCache
	window      time.Duration
	group       string
	maxHits     int
	cachePeriod time.Duration
}

func newCache(c *forget.CacheSpaces, namespace string, ttl time.Duration) *cache {
	return &cache{
		cache:     c,
		namespace: namespace,
		ttl:       ttl,
	}
}

func newClusterLimitRedisCached(
	s Settings,
	r *net.RedisRingClient,
	c *forget.CacheSpaces,
	group string,
	cachePeriodFactor int,
) *clusterLimitRedisCached {
	// we rely here primarily on the LRU mechanism and not on the TTL of the cached items, but for cleanup,
	// it is safe to set the TTL to the double of the rate limiting time window
	//
	cacheItemTTL := 2 * s.TimeWindow
	cache := newCache(c, redisCacheNamespace, cacheItemTTL)

	// together with the time window, this controls the frequency of the Redis calls and the precision of
	// the rate limiting. The higher value results in the higher number of Redis calls and higher
	// precision. Example:
	//
	// timeWindow=1m, cachePeriodFactor=256
	// => 1m/256=234ms (redis sync every 234ms), (256 - 1)/256=99.6% rate limiting precision over 1m
	//
	if cachePeriodFactor <= 0 {
		cachePeriodFactor = defaultCachePeriodFactor
	}

	cachePeriod := time.Duration(int(s.TimeWindow) / cachePeriodFactor)

	return &clusterLimitRedisCached{
		redis:       r,
		cache:       cache,
		window:      s.TimeWindow,
		group:       group,
		maxHits:     s.MaxHits,
		cachePeriod: cachePeriod,
	}
}

func (f *cache) get(key string) (item cacheItem, ok bool) {
	var r io.ReadCloser
	r, ok = f.cache.Get(f.namespace, key)
	if !ok {
		return
	}

	defer r.Close()
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&item); err != nil {
		log.Errorf("Error while decoding a cached item: %v.", err)
		ok = false
		return
	}

	return
}

func (f *cache) set(key string, item cacheItem) {
	// forget.CacheSpaces.Set returns false only in case of a closed cache
	w, ok := f.cache.Set(f.namespace, key, f.ttl)
	if !ok {
		log.Error("Cached redis rate limit: trying to write to a closed cache.")
	}

	defer w.Close()
	enc := gob.NewEncoder(w)
	if err := enc.Encode(item); err != nil {
		// we can ignore this error for the control flow:
		log.Errorf("Error while encoding cache item to memory: %v.", err)
	}
}

// TODO: z cards in Redis represent a set by the value, which means that there is a small chance that the same
// sum and the same timestamp from different Skipper instances will conflict. Consider adding a salt. Note that
// this affects the other Redis rate limiting implementation, too
//
func redisValue(sum int, timestamp time.Time) string {
	return fmt.Sprintf("%d|%d", sum, timestamp.UnixNano())
}

func fromRedisValue(v string) (sum int, timestamp time.Time, err error) {
	p := strings.Split(v, "|")
	if len(p) != 2 {
		err = fmt.Errorf("invalid redis value: %s", v)
		return
	}

	if sum, err = strconv.Atoi(p[0]); err != nil {
		err = fmt.Errorf("invalid redis value: %s; %w", v, err)
		return
	}

	var nano64 int64
	if nano64, err = strconv.ParseInt(p[1], 10, 64); err != nil {
		err = fmt.Errorf("invalid redis value: %s; %w", v, err)
		return
	}

	timestamp = time.Unix(0, nano64)
	return
}

func (c *clusterLimitRedisCached) sync(ctx context.Context, key string, now time.Time) {
	oldest := now.Add(-c.window)
	cached, _ := c.cache.get(key)
	cached.FailOpen = false
	defer func(pcached *cacheItem) {
		cached := *pcached

		// using a fresh timestamp after several network calls:
		cached.LastSync = time.Now()
		c.cache.set(key, cached)
	}(&cached)

	// we don't need to abort on the below error, but we only want to add new items to Redis if the delete
	// hadn't failed to avoid overloading its storage
	//
	_, errRem := c.redis.ZRemRangeByScore(ctx, key, 0.0, float64(oldest.UnixNano()))
	if errRem != nil {
		log.Errorf("Error while cleaning up old rate entries: %v.", errRem)
	}

	if errRem == nil && cached.LocalSum > 0 {
		_, err := c.redis.ZAdd(
			ctx,
			key,
			redisValue(cached.LocalSum, now),
			float64(now.UnixNano()),
		)

		if err != nil {
			log.Errorf("Error while storing local rate in redis: %v.", err)
		} else {
			cached.LocalSum = 0
		}
	}

	// if getting the entries fails, we need to fail open, because we don't know enough information about
	// the request rate in the other Skipper instances
	//
	values, err := c.redis.ZRangeByScoreAll(ctx, key, float64(oldest.UnixNano()), float64(now.UnixNano()))
	if err != nil {
		cached.FailOpen = true
		return
	}

	if len(values) == 0 {
		cached.Oldest = now
	} else {
		_, oldest, err := fromRedisValue(values[0])
		if err != nil {
			log.Errorf("Invalid entry in redis: %v.", err)
		} else {
			cached.Oldest = oldest
		}
	}

	// the following iteration is limited by c.window / c.cachePeriod, therefore c.cachePeriod should be
	// chosen to keep the number of values in a small range, e.g. 128 or 256 or so. This value also defines
	// the precision of the rate limiting as (N-1)/N, e.g: (128-1)/128 > 99%. A sane default ratio should
	// be defined. This means, it's an O(N) in-process operation, where N is cache period factor, but it is
	// a negligibly small N
	//
	cached.SyncedSum = 0
	for _, v := range values {
		sum, _, err := fromRedisValue(v)
		if err != nil {
			log.Errorf("Invalid entry in redis: %v.", err)
			continue
		}

		cached.SyncedSum += sum
	}

	// ensuring cleanup in cases of requests with a given key stop coming in:
	if _, err := c.redis.Expire(ctx, key, c.window+time.Minute); err != nil {
		log.Errorf("Error while refreshing the expiration of a redis key: %v.", err)
	}
}

func (c *clusterLimitRedisCached) AllowContext(ctx context.Context, key string) bool {
	key = getHashedKey(key)
	key = prefixKey(c.group, key)
	now := time.Now()
	cached, ok := c.cache.get(key)
	if !ok || cached.LastSync.Before(now.Add(-c.cachePeriod)) {
		c.sync(ctx, key, now)
		cached, _ = c.cache.get(key)
	}

	if cached.FailOpen {
		return true
	}

	// adding local sum here only improves the precision, but it doesn't represent the local sums of the
	// other Skipper instances. When the rate limit is defined as part of a contract, the service provider
	// may temporarily allow a slightly higher rate than the contract, but never would reject a request
	// rate below the one defined by the contract. This way, the rate limiting errs on behalf of the user
	// and not the service provider. Note that full precision is not possible anyway because of timing
	// concerns will be always involved
	//
	if cached.SyncedSum+cached.LocalSum >= c.maxHits {
		return false
	}

	cached.LocalSum++
	c.cache.set(key, cached)
	return true
}

func (c *clusterLimitRedisCached) Allow(key string) bool {
	return c.AllowContext(context.Background(), key)
}

func (c *clusterLimitRedisCached) oldest(ctx context.Context, key string) time.Time {
	key = getHashedKey(key)
	key = prefixKey(c.group, key)
	now := time.Now()
	cached, ok := c.cache.get(key)
	if !ok || cached.LastSync.Before(now.Add(-c.cachePeriod)) {
		c.sync(ctx, key, now)
		cached, _ = c.cache.get(key)
	}

	return cached.Oldest
}

func (c *clusterLimitRedisCached) deltaFrom(ctx context.Context, key string, from time.Time) time.Duration {
	return from.Sub(c.oldest(ctx, key))
}

func (c *clusterLimitRedisCached) Delta(key string) time.Duration {
	return c.deltaFrom(context.Background(), key, time.Now())
}

func (c *clusterLimitRedisCached) Oldest(key string) time.Time {
	return c.oldest(context.Background(), key)
}

func (c *clusterLimitRedisCached) RetryAfterContext(ctx context.Context, key string) int {
	const minWait = time.Second
	retr := c.deltaFrom(ctx, key, time.Now())
	if retr <= 0 {
		retr += minWait
	}

	return int(retr / time.Second)
}

func (c *clusterLimitRedisCached) RetryAfter(key string) int {
	return c.RetryAfterContext(context.Background(), key)
}

// Resize is noop to implement the limiter interface
func (*clusterLimitRedisCached) Resize(string, int) {}

func (c *clusterLimitRedisCached) Close() {
	// we don't need to close neither the redis ring or the cache here, as it is a shared resource
}
