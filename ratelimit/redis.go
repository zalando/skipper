package ratelimit

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

// RedisOptions is used to configure the redis.Ring
type RedisOptions struct {
	// Addrs are the list of redis shards
	Addrs []string
}

// clusterLimitRedis stores all data required for the cluster ratelimit.
type clusterLimitRedis struct {
	group   string
	maxHits int64
	window  time.Duration
	ring    *redis.Ring
}

func newRing(ro *RedisOptions) *redis.Ring {
	var ring *redis.Ring
	if ro != nil {
		ringOptions := &redis.RingOptions{
			Addrs: map[string]string{},
		}
		for idx, addr := range ro.Addrs {
			ringOptions.Addrs[fmt.Sprintf("server%d", idx)] = addr
		}
		ring = redis.NewRing(ringOptions)
		// TODO(sszuecs): maybe wrap with context and expose a flag
		//ring = ring.WithContext(context.Background())
	}
	return ring
}

// newClusterRateLimiterRedis creates a new clusterLimitRedis for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiterRedis(s Settings, ring *redis.Ring, group string) *clusterLimitRedis {
	if ring == nil {
		return nil
	}

	rl := &clusterLimitRedis{
		group:   group,
		maxHits: int64(s.MaxHits),
		window:  s.TimeWindow,
		ring:    ring,
	}

	var err error

	err = backoff.Retry(func() error {
		_, err = rl.ring.Ping().Result()
		if err != nil {
			log.Infof("Failed to ping redis, retry with backoff: %v", err)
		}
		return err
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 7))

	if err != nil {
		log.Errorf("Failed to connect to redis: %v", err)
		return nil
	}
	log.Debug("Redis ring is reachable")

	return rl
}

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
//
// Performance considerations:
//
// In case of deny it will use ZREMRANGEBYSCORE and ZCARD commands in
// one pipeline to remove old items in the list of hits.
// In case of allow it will additionally use ZADD with a second
// roundtrip.
func (c *clusterLimitRedis) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()
	nowSec := now.UnixNano()
	clearBefore := now.Add(-c.window).UnixNano()

	count := c.allowCheckCard(key, clearBefore)
	// we increase later with ZAdd, so max-1
	if count >= c.maxHits {
		log.Debugf("redis disallow %s request: %d >= %d = %v", key, count, c.maxHits, count > c.maxHits)
		return false
	}

	c.ring.ZAdd(key, redis.Z{Member: nowSec, Score: float64(nowSec)})
	return true
}

func (c *clusterLimitRedis) allowCheckCard(key string, clearBefore int64) int64 {
	// TODO(sszuecs): https://github.com/go-redis/redis/issues/979 change to TxPipeline: MULTI exec
	// Pipeline is not a transaction, but less roundtrip
	pipe := c.ring.Pipeline()
	defer pipe.Close()
	// drop all elements of the set which occurred before one interval ago.
	pipe.ZRemRangeByScore(key, "0.0", fmt.Sprintf("%v", float64(clearBefore)))
	// get cardinality
	zcardResult := pipe.ZCard(key)
	_, err := pipe.Exec()
	if err != nil {
		log.Errorf("Failed to get redis cardinality for %s: %v", key, err)
		return 0
	}
	return zcardResult.Val()
}

// Close all redis ring shards
func (c *clusterLimitRedis) Close() { c.ring.Close() }

// Delta returns the time.Duration until the next call is allowed,
// negative means immediate calls are allowed
func (c *clusterLimitRedis) Delta(s string) time.Duration {
	now := time.Now()
	oldest := c.Oldest(s)
	gab := now.Sub(oldest)
	return c.window - gab
}

// Oldest returns the oldest known request time.
//
// Performance considerations:
//
// It will use ZRANGEBYSCORE with offset 0 and count 1 to get the
// oldest item stored in redis.
func (c *clusterLimitRedis) Oldest(s string) time.Time {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()

	res := c.ring.ZRangeByScoreWithScores(key, redis.ZRangeBy{
		Min:    "0.0",
		Max:    fmt.Sprintf("%v", float64(now.UnixNano())),
		Offset: 0,
		Count:  1,
	})

	if zs := res.Val(); len(zs) > 0 {
		z := zs[0]
		if s, ok := z.Member.(string); ok {
			oldest, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				log.Errorf("Failed to convert '%v' to int64: %v", s, err)
			}
			return time.Unix(0, oldest)
		}
		log.Errorf("Failed to convert redis data to int64: %v", z.Member)
	}
	log.Debugf("Oldest() for key %s got no valid data: %v", key, res)
	return time.Time{}
}

// Resize is noop to implement the limiter interface
func (*clusterLimitRedis) Resize(string, int) {}

// RetryAfter returns seconds until next call is allowed similar to
// Delta(), but returns at least one 1 in all cases. That is being
// done, because if not the ratelimit would be too few ratelimits,
// because of how it's used in the proxy and the nature of cluster
// ratelimits being not strongly consistent across calls to Allow()
// and RetryAfter().
//
// Performance considerations: It uses Oldest() to get the data from
// Redis.
func (c *clusterLimitRedis) RetryAfter(s string) int {
	retr := c.Delta(s)
	res := int(retr / time.Second)
	if res > 0 {
		return res + 1
	}
	// Less than 1s to wait -> so set to 1
	return 1
}
