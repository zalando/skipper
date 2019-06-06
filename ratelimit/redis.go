package ratelimit

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
)

// RedisOptions is used to configure the redis.Ring
type RedisOptions struct {
	// Addrs are the list of redis shards
	Addrs []string
	// ReadTimeout for redis socket reads
	ReadTimeout time.Duration
	// WriteTimeout for redis socket writes
	WriteTimeout time.Duration
	// PoolTimeout is the max time.Duration to get a connection from pool
	PoolTimeout time.Duration
	// MinIdleConns is the minimum number of socket connections to redis
	MinIdleConns int
	// MaxIdleConns is the maximum number of socket connections to redis
	MaxIdleConns int
	// ConnMetricsInterval defines the frequency of updating the redis
	// connection related metrics. Defaults to 60 seconds.
	ConnMetricsInterval time.Duration
}

type ring struct {
	ring    *redis.Ring
	metrics metrics.Metrics
}

// clusterLimitRedis stores all data required for the cluster ratelimit.
type clusterLimitRedis struct {
	group   string
	maxHits int64
	window  time.Duration
	ring    *redis.Ring
	metrics metrics.Metrics
}

const (
	DefaultReadTimeout  = 25 * time.Millisecond
	DefaultWriteTimeout = 25 * time.Millisecond
	DefaultPoolTimeout  = 25 * time.Millisecond
	DefaultMinConns     = 100
	DefaultMaxConns     = 100

	defaultConnMetricsInterval = 60 * time.Second
	redisMetricsPrefix         = "swarm.redis."
)

func newRing(ro *RedisOptions, quit <-chan struct{}) *ring {
	var r *ring

	ringOptions := &redis.RingOptions{
		Addrs: map[string]string{},
	}

	if ro != nil {
		for idx, addr := range ro.Addrs {
			ringOptions.Addrs[fmt.Sprintf("redis%d", idx)] = addr
		}
		ringOptions.ReadTimeout = ro.ReadTimeout
		ringOptions.WriteTimeout = ro.WriteTimeout
		ringOptions.PoolTimeout = ro.PoolTimeout
		ringOptions.MinIdleConns = ro.MinIdleConns
		ringOptions.PoolSize = ro.MaxIdleConns

		if ro.ConnMetricsInterval <= 0 {
			ro.ConnMetricsInterval = defaultConnMetricsInterval
		}

		r = new(ring)
		r.ring = redis.NewRing(ringOptions)
		r.metrics = metrics.Default

		go func() {
			for {
				select {
				case <-time.After(ro.ConnMetricsInterval):
					stats := r.ring.PoolStats()
					r.metrics.UpdateGauge(redisMetricsPrefix+"hits", float64(stats.Hits))
					r.metrics.UpdateGauge(redisMetricsPrefix+"idleconns", float64(stats.IdleConns))
					r.metrics.UpdateGauge(redisMetricsPrefix+"misses", float64(stats.Misses))
					r.metrics.UpdateGauge(redisMetricsPrefix+"staleconns", float64(stats.StaleConns))
					r.metrics.UpdateGauge(redisMetricsPrefix+"timeouts", float64(stats.Timeouts))
					r.metrics.UpdateGauge(redisMetricsPrefix+"totalconns", float64(stats.TotalConns))
				case <-quit:
					r.ring.Close()
					return
				}
			}
		}()
	}
	return r
}

// newClusterRateLimiterRedis creates a new clusterLimitRedis for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiterRedis(s Settings, r *ring, group string) *clusterLimitRedis {
	if r == nil {
		return nil
	}

	rl := &clusterLimitRedis{
		group:   group,
		maxHits: int64(s.MaxHits),
		window:  s.TimeWindow,
		ring:    r.ring,
		metrics: r.metrics,
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
	c.metrics.IncCounter(redisMetricsPrefix + "total")
	key := swarmPrefix + c.group + "." + s
	now := time.Now()
	nowNanos := now.UnixNano()
	clearBefore := now.Add(-c.window).UnixNano()

	count := c.allowCheckCard(key, clearBefore)
	// we increase later with ZAdd, so max-1
	if count >= c.maxHits {
		c.metrics.IncCounter(redisMetricsPrefix + "forbids")
		log.Debugf("redis disallow %s request: %d >= %d = %v", key, count, c.maxHits, count > c.maxHits)
		return false
	}

	c.ring.ZAdd(key, redis.Z{Member: nowNanos, Score: float64(nowNanos)})
	c.metrics.IncCounter(redisMetricsPrefix + "allows")
	return true
}

func (c *clusterLimitRedis) allowCheckCard(key string, clearBefore int64) int64 {
	// TODO(sszuecs): https://github.com/go-redis/redis/issues/979 change to TxPipeline: MULTI exec
	// Pipeline is not a transaction, but less roundtrip
	pipe := c.ring.Pipeline()
	defer pipe.Close()
	// drop all elements of the set which occurred before one interval ago.
	pipe.ZRemRangeByScore(key, "0.0", fmt.Sprint(float64(clearBefore)))
	// get cardinality
	zcardResult := pipe.ZCard(key)
	_, err := pipe.Exec()
	if err != nil {
		log.Errorf("Failed to get redis cardinality for %s: %v", key, err)
		return 0
	}
	return zcardResult.Val()
}

// Close can not decide to teardown redis ring, because it is not the
// owner of it.
func (c *clusterLimitRedis) Close() {}

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
		Max:    fmt.Sprint(float64(now.UnixNano())),
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
