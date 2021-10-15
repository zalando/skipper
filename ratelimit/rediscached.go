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

type cacheItem struct {
	lastSync  time.Time
	oldest    time.Time
	syncedSum int
	localSum  int
	failOpen  bool
}

type redisCache interface {
	get(string) (cacheItem, bool)
	set(string, cacheItem)
}

type forgetCache struct {
	namespace string
	cache     *forget.CacheSpaces
	ttl time.Duration
}

type clusterLimitRedisCached struct {
	window      time.Duration
	cache       redisCache
	redis       *net.RedisRingClient
	group       string
	cachePeriod time.Duration
	maxHits     int
}

func (f forgetCache) get(key string) (item cacheItem, ok bool) {
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

	ok = true
	return
}

func (f forgetCache) set(key string, item cacheItem) {
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

func (f forgetCache) Close() {
	f.cache.Close()
}

// TODO: z cards in Redis represent a set by the value, which means that there is a small chance that the same
// sum and the same timestamp from different Skipper instances will conflict. Consider adding a salt. Note that
// this affects the other Redis rate limiting implementation, too
func redisValue(sum int, timestamp time.Time) string {
	return fmt.Sprintf("%d|%d", sum, timestamp.UnixNano())
}

func fromRedisValue(v string) (sum int, timestamp time.Time, err error) {
	p := strings.Split(v, "|")
	if len(p) != 2 {
		err = fmt.Errorf("invalid redis value: %s", v)
		return
	}

	var sum64 int64
	if sum64, err = strconv.ParseInt(p[0], 10, 64); err != nil {
		err = fmt.Errorf("invalid redis value: %s; %w", v, err)
		return
	}

	var nano64 int64
	if nano64, err = strconv.ParseInt(p[1], 10, 64); err != nil {
		err = fmt.Errorf("invalid redis value: %s; %w", v, err)
		return
	}

	sum = int(sum64)
	timestamp = time.Unix(0, nano64)
	return
}

func (c *clusterLimitRedisCached) sync(ctx context.Context, key string, now time.Time) {
	oldest := now.Add(-c.window)
	cached, _ := c.cache.get(key)
	cached.failOpen = false
	defer func(pcached *cacheItem) {
		cached := *pcached

		// using a fresh timestamp after several network calls:
		cached.lastSync = time.Now()
		c.cache.set(key, cached)
	}(&cached)

	// we don't need to abort on the below error, but we only want to add new items to Redis if the delete
	// hadn't failed to avoid overloading its storage
	//
	_, errRem := c.redis.ZRemRangeByScore(ctx, key, 0.0, float64(oldest.UnixNano()))
	if errRem != nil {
		log.Errorf("Error while cleaning up old rate entries: %v.", errRem)
	}

	if errRem == nil && cached.localSum > 0 {
		_, err := c.redis.ZAdd(
			ctx,
			key,
			redisValue(cached.localSum, now),
			float64(now.UnixNano()),
		)

		if err != nil {
			log.Errorf("Error while storing local rate in redis: %v.", err)
		} else {
			cached.localSum = 0
		}
	}

	// if getting the entries fails, we need to fail open, because we don't know enough information about
	// the other Skipper instances
	//
	values, err := c.redis.ZRangeByScoreAll(ctx, key, float64(oldest.UnixNano()), float64(now.UnixNano()))
	if err != nil {
		cached.failOpen = true
		return
	}

	if len(values) == 0 {
		cached.oldest = now
	} else {
		_, oldest, err := fromRedisValue(values[0])
		if err != nil {
			log.Errorf("Invalid entry in redis: %v.", err)
		} else {
			cached.oldest = oldest
		}
	}

	// the following iteration is limited by c.window / c.cachePeriod, therefore c.cachePeriod should be
	// chosen to keep the number of values in a small range, e.g. 128 or 256 or so. This value also defines
	// the precision of the rate limiting as (N-1)/N, e.g: (128-1)/128 > 99%. A sane default ratio should
	// be defined
	//
	cached.syncedSum = 0
	for _, v := range values {
		sum, _, err := fromRedisValue(v)
		if err != nil {
			log.Errorf("Invalid entry in redis: %v.", err)
			continue
		}

		cached.syncedSum += sum
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
	if !ok || cached.lastSync.Before(now.Add(-c.cachePeriod)) {
		c.sync(ctx, key, now)
		cached, _ = c.cache.get(key)
	}

	if cached.failOpen {
		return true
	}

	// adding local sum here only improves the precision, but it doesn't represent the local sums of the
	// other Skipper instances. When the rate limit is defined as part of a contract, the service provider
	// may temporarily allow a slightly higher rate than the contract, but never would reject a request
	// rate below the one defined by the contract. This way, the rate limiting errs on behalf of the user
	// and not the service provider. Note that full precision is not possible anyway because of timing
	// concerns will be always involved
	//
	if cached.syncedSum+cached.localSum >= c.maxHits {
		return false
	}

	cached.localSum++
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
	if !ok || cached.lastSync.Before(now.Add(-c.cachePeriod)) {
		c.sync(ctx, key, now)
		cached, _ = c.cache.get(key)
	}

	return cached.oldest
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

func (c *clusterLimitRedisCached) Close() {
	c.redis.Close()
	if closer, ok := c.cache.(interface{ Close() }); ok {
		closer.Close()
	}
}
