package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-redis/redis/v7"
	"github.com/opentracing/opentracing-go"
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
	// Tracer provides OpenTracing for Redis queries.
	Tracer opentracing.Tracer
}

type ring struct {
	ring    *redis.Ring
	metrics metrics.Metrics
	tracer  opentracing.Tracer
}

// clusterLimitRedis stores all data required for the cluster ratelimit.
type clusterLimitRedis struct {
	group   string
	maxHits int64
	window  time.Duration
	ring    *redis.Ring
	metrics metrics.Metrics
	tracer  opentracing.Tracer
}

const (
	DefaultReadTimeout  = 25 * time.Millisecond
	DefaultWriteTimeout = 25 * time.Millisecond
	DefaultPoolTimeout  = 25 * time.Millisecond
	DefaultMinConns     = 100
	DefaultMaxConns     = 100

	defaultConnMetricsInterval       = 60 * time.Second
	redisMetricsPrefix               = "swarm.redis."
	allowMetricsFormat               = redisMetricsPrefix + "query.allow.%s"
	retryAfterMetricsFormat          = redisMetricsPrefix + "query.retryafter.%s"
	allowMetricsFormatWithGroup      = redisMetricsPrefix + "query.allow.%s.%s"
	retryAfterMetricsFormatWithGroup = redisMetricsPrefix + "query.retryafter.%s.%s"
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
		r.tracer = ro.Tracer

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
		tracer:  r.tracer,
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

func (c *clusterLimitRedis) prefixKey(s string) string {
	return fmt.Sprintf(swarmKeyFormat, c.group, s)
}

func (c *clusterLimitRedis) measureQuery(format, groupFormat string, fail *bool, start time.Time) {
	result := "success"
	if fail != nil && *fail {
		result = "failure"
	}

	var key string
	if c.group == "" {
		key = fmt.Sprintf(format, result)
	} else {
		key = fmt.Sprintf(groupFormat, result, c.group)
	}

	c.metrics.MeasureSince(key, start)
}

// AllowContext returns true if the request calculated across the cluster of
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
//
// If a context is provided, it uses it for creating an OpenTracing span.
func (c *clusterLimitRedis) AllowContext(ctx context.Context, s string) bool {
	c.metrics.IncCounter(redisMetricsPrefix + "total")
	key := c.prefixKey(s)

	now := time.Now()
	var queryFailure bool
	defer c.measureQuery(allowMetricsFormat, allowMetricsFormatWithGroup, &queryFailure, now)

	nowNanos := now.UnixNano()
	clearBefore := now.Add(-c.window).UnixNano()

	count, err := c.allowCheckCard(key, clearBefore)
	if err != nil {
		log.Errorf("Failed to get redis cardinality for %s: %v", key, err)
		queryFailure = true
		// we don't return here, as we still want to record the request with ZAdd, but we mark it as a
		// failure for the metrics
	}

	// we increase later with ZAdd, so max-1
	if err == nil && count >= c.maxHits {
		c.metrics.IncCounter(redisMetricsPrefix + "forbids")
		log.Debugf("redis disallow %s request: %d >= %d = %v", key, count, c.maxHits, count > c.maxHits)
		return false
	}

	pipe := c.ring.TxPipeline()
	defer pipe.Close()
	pipe.ZAdd(key, &redis.Z{Member: nowNanos, Score: float64(nowNanos)})
	pipe.Expire(key, c.window+time.Second)
	_, err = pipe.Exec()
	if err != nil {
		log.Errorf("Failed to ZAdd and Expire for %s: %v", key, err)
		queryFailure = true
		return true
	}

	c.metrics.IncCounter(redisMetricsPrefix + "allows")
	return true
}

// Allow is like AllowContext, but not using a context.
func (c *clusterLimitRedis) Allow(s string) bool {
	return c.AllowContext(nil, s)
}

func (c *clusterLimitRedis) allowCheckCard(key string, clearBefore int64) (int64, error) {
	pipe := c.ring.TxPipeline()
	defer pipe.Close()
	// drop all elements of the set which occurred before one interval ago.
	pipe.ZRemRangeByScore(key, "0.0", fmt.Sprint(float64(clearBefore)))
	// get cardinality
	zcardResult := pipe.ZCard(key)
	_, err := pipe.Exec()
	if err != nil {
		return 0, err
	}
	return zcardResult.Val(), nil
}

// Close can not decide to teardown redis ring, because it is not the
// owner of it.
func (c *clusterLimitRedis) Close() {}

func (c *clusterLimitRedis) deltaFrom(s string, from time.Time) (time.Duration, error) {
	oldest, err := c.oldest(s)
	if err != nil {
		return 0, err
	}

	gab := from.Sub(oldest)
	return c.window - gab, nil
}

// Delta returns the time.Duration until the next call is allowed,
// negative means immediate calls are allowed
func (c *clusterLimitRedis) Delta(s string) time.Duration {
	now := time.Now()
	d, err := c.deltaFrom(s, now)
	if err != nil {
		log.Errorf("Failed to get the duration until the next call is allowed: %v", err)

		// Earlier, we returned duration since time=0 in these error cases. It is more graceful to the
		// client applications to return 0.
		return 0
	}

	return d
}

func (c *clusterLimitRedis) oldest(s string) (time.Time, error) {
	key := c.prefixKey(s)
	now := time.Now()

	res := c.ring.ZRangeByScoreWithScores(key, &redis.ZRangeBy{
		Min:    "0.0",
		Max:    fmt.Sprint(float64(now.UnixNano())),
		Offset: 0,
		Count:  1,
	})

	zs, err := res.Result()
	if err != nil {
		return time.Time{}, err
	}

	if len(zs) > 0 {
		z := zs[0]
		if s, ok := z.Member.(string); ok {
			oldest, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("failed to convert '%v' to int64: %w", s, err)
			}
			return time.Unix(0, oldest), nil
		}
		return time.Time{}, fmt.Errorf("failed to convert redis data to int64: %v", z.Member)
	}
	log.Debugf("Oldest() for key %s got no valid data: %v", key, res)
	return time.Time{}, nil
}

// Oldest returns the oldest known request time.
//
// Performance considerations:
//
// It will use ZRANGEBYSCORE with offset 0 and count 1 to get the
// oldest item stored in redis.
func (c *clusterLimitRedis) Oldest(s string) time.Time {
	t, err := c.oldest(s)
	if err != nil {
		log.Errorf("Failed to get the oldest known request time: %v", err)
		return time.Time{}
	}

	return t
}

// Resize is noop to implement the limiter interface
func (*clusterLimitRedis) Resize(string, int) {}

// RetryAfterContext returns seconds until next call is allowed similar to
// Delta(), but returns at least one 1 in all cases. That is being
// done, because if not the ratelimit would be too few ratelimits,
// because of how it's used in the proxy and the nature of cluster
// ratelimits being not strongly consistent across calls to Allow()
// and RetryAfter() (or AllowContext and RetryAfterContext accordingly).
//
// Performance considerations: It uses Oldest() to get the data from
// Redis.
//
// If a context is provided, it uses it for creating an OpenTracing span.
func (c *clusterLimitRedis) RetryAfterContext(ctx context.Context, s string) int {
	// If less than 1s to wait -> so set to 1
	const minWait = 1

	now := time.Now()
	var queryFailure bool
	defer c.measureQuery(retryAfterMetricsFormat, retryAfterMetricsFormatWithGroup, &queryFailure, now)

	retr, err := c.deltaFrom(s, now)
	if err != nil {
		log.Errorf("Failed to get the duration to wait with the next request: %v", err)
		queryFailure = true
		return minWait
	}

	res := int(retr / time.Second)
	if res > 0 {
		return res + 1
	}

	return minWait
}

// RetryAfter is like RetryAfterContext, but not using a context.
func (c *clusterLimitRedis) RetryAfter(s string) int {
	return c.RetryAfterContext(nil, s)
}
