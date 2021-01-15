package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
)

// clusterLimitRedis stores all data required for the cluster ratelimit.
type clusterLimitRedis struct {
	group      string
	maxHits    int64
	window     time.Duration
	ringClient *net.RedisRingClient
	metrics    metrics.Metrics
}

const (
	redisMetricsPrefix               = "swarm.redis."
	allowMetricsFormat               = redisMetricsPrefix + "query.allow.%s"
	retryAfterMetricsFormat          = redisMetricsPrefix + "query.retryafter.%s"
	allowMetricsFormatWithGroup      = redisMetricsPrefix + "query.allow.%s.%s"
	retryAfterMetricsFormatWithGroup = redisMetricsPrefix + "query.retryafter.%s.%s"

	allowAddSpanName           = "redis_allow_add_card"
	allowExpireSpanName        = "redis_allow_expire"
	allowCheckSpanName         = "redis_allow_check_card"
	allowCheckRemRangeSpanName = "redis_allow_check_rem_range"
	oldestScoreSpanName        = "redis_oldest_score"
)

// newClusterRateLimiterRedis creates a new clusterLimitRedis for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiterRedis(s Settings, r *net.RedisRingClient, group string) *clusterLimitRedis {
	if r == nil {
		return nil
	}

	rl := &clusterLimitRedis{
		group:      group,
		maxHits:    int64(s.MaxHits),
		window:     s.TimeWindow,
		ringClient: r,
		metrics:    metrics.Default,
	}

	return rl
}

func (c *clusterLimitRedis) prefixKey(clearText string) string {
	return fmt.Sprintf(swarmKeyFormat, c.group, clearText)
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

func (c *clusterLimitRedis) startSpan(ctx context.Context, spanName string) func(bool) {
	nop := func(bool) {}
	if ctx == nil {
		return nop
	}

	parentSpan := opentracing.SpanFromContext(ctx)
	if parentSpan == nil {
		return nop
	}

	span := c.ringClient.Tracer().StartSpan(spanName, opentracing.ChildOf(parentSpan.Context()))
	ext.Component.Set(span, "skipper")
	ext.SpanKind.Set(span, "client")
	span.SetTag("group", c.group)
	span.SetTag("max_hits", c.maxHits)
	span.SetTag("window", c.window.String())

	return func(failed bool) {
		if failed {
			ext.Error.Set(span, true)
		}

		span.Finish()
	}
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
func (c *clusterLimitRedis) AllowContext(ctx context.Context, clearText string) bool {
	s := getHashedKey(clearText)
	c.metrics.IncCounter(redisMetricsPrefix + "total")
	key := c.prefixKey(s)

	now := time.Now()
	var queryFailure bool
	defer c.measureQuery(allowMetricsFormat, allowMetricsFormatWithGroup, &queryFailure, now)

	nowNanos := now.UnixNano()
	clearBefore := now.Add(-c.window).UnixNano()

	count, err := c.allowCheckCard(ctx, key, clearBefore)
	if err != nil {
		log.Errorf("Failed to get from redis cardinality: %v", err)
		queryFailure = true
		// we don't return here, as we still want to record the request with ZAdd, but we mark it as a
		// failure for the metrics
	}

	// we increase later with ZAdd, so max-1
	if err == nil && count >= c.maxHits {
		c.metrics.IncCounter(redisMetricsPrefix + "forbids")
		log.Debugf("redis disallow request: %d >= %d = %v", count, c.maxHits, count > c.maxHits)
		return false
	}

	finishSpan := c.startSpan(ctx, allowAddSpanName)
	zaddResult := c.ringClient.ZAdd(ctx, key, &redis.Z{Member: nowNanos, Score: float64(nowNanos)})
	err = zaddResult.Err()
	finishSpan(err != nil)
	if err != nil {
		log.Errorf("Failed to redis ZAdd proceeding with Expire: %v", err)
		queryFailure = true
	}

	finishSpan = c.startSpan(ctx, allowExpireSpanName)
	expireResult := c.ringClient.Expire(ctx, key, c.window+time.Second)
	err = expireResult.Err()
	finishSpan(err != nil)
	if err != nil {
		log.Errorf("Failed to redis Expire: %v", err)
		queryFailure = true
		return true
	}

	c.metrics.IncCounter(redisMetricsPrefix + "allows")
	return true
}

// Allow is like AllowContext, but not using a context.
func (c *clusterLimitRedis) Allow(clearText string) bool {
	return c.AllowContext(context.Background(), clearText)
}

func (c *clusterLimitRedis) allowCheckCard(ctx context.Context, key string, clearBefore int64) (int64, error) {
	// drop all elements of the set which occurred before one interval ago.
	finishSpan := c.startSpan(ctx, allowCheckRemRangeSpanName)
	zremRangeResult := c.ringClient.ZRemRangeByScore(ctx, key, "0.0", fmt.Sprint(float64(clearBefore)))
	err := zremRangeResult.Err()
	finishSpan(err != nil)
	if err != nil {
		return 0, fmt.Errorf("zremrangebyscore: %w", err)
	}

	// get cardinality
	finishSpan = c.startSpan(ctx, allowCheckSpanName)
	zcardResult := c.ringClient.ZCard(ctx, key)
	err = zcardResult.Err()
	finishSpan(err != nil)
	if err != nil {
		return 0, fmt.Errorf("zcard: %w", err)
	}

	return zcardResult.Val(), nil
}

// Close can not decide to teardown redis ring, because it is not the
// owner of it.
func (c *clusterLimitRedis) Close() {}

func (c *clusterLimitRedis) deltaFrom(ctx context.Context, clearText string, from time.Time) (time.Duration, error) {
	oldest, err := c.oldest(ctx, clearText)
	if err != nil {
		return 0, err
	}

	gap := from.Sub(oldest)
	return c.window - gap, nil
}

// Delta returns the time.Duration until the next call is allowed,
// negative means immediate calls are allowed
func (c *clusterLimitRedis) Delta(clearText string) time.Duration {
	now := time.Now()
	d, err := c.deltaFrom(context.Background(), clearText, now)
	if err != nil {
		log.Errorf("Failed to redis get the duration until the next call is allowed: %v", err)

		// Earlier, we returned duration since time=0 in these error cases. It is more graceful to the
		// client applications to return 0.
		return 0
	}

	return d
}

func (c *clusterLimitRedis) oldest(ctx context.Context, clearText string) (time.Time, error) {
	s := getHashedKey(clearText)
	key := c.prefixKey(s)
	now := time.Now()

	finishSpan := c.startSpan(ctx, oldestScoreSpanName)
	res := c.ringClient.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:    "0.0",
		Max:    fmt.Sprint(float64(now.UnixNano())),
		Offset: 0,
		Count:  1,
	})

	zs, err := res.Result()
	if err != nil {
		finishSpan(true)
		return time.Time{}, err
	}

	if len(zs) == 0 {
		log.Debugf("redis Oldest() got no valid data: %v", res)
		finishSpan(false)
		return time.Time{}, nil
	}

	z := zs[0]
	s, ok := z.Member.(string)
	if !ok {
		finishSpan(true)
		return time.Time{}, errors.New("failed to evaluate redis data")
	}

	oldest, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		finishSpan(true)
		return time.Time{}, fmt.Errorf("failed to convert value to int64: %w", err)
	}

	finishSpan(false)
	return time.Unix(0, oldest), nil
}

// Oldest returns the oldest known request time.
//
// Performance considerations:
//
// It will use ZRANGEBYSCORE with offset 0 and count 1 to get the
// oldest item stored in redis.
func (c *clusterLimitRedis) Oldest(clearText string) time.Time {
	t, err := c.oldest(context.Background(), clearText)
	if err != nil {
		log.Errorf("Failed to get from redis the oldest known request time: %v", err)
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
// If a context is provided, it uses it for creating an OpenTracing span.
func (c *clusterLimitRedis) RetryAfterContext(ctx context.Context, clearText string) int {
	// If less than 1s to wait -> so set to 1
	const minWait = 1

	now := time.Now()
	var queryFailure bool
	defer c.measureQuery(retryAfterMetricsFormat, retryAfterMetricsFormatWithGroup, &queryFailure, now)

	retr, err := c.deltaFrom(ctx, clearText, now)
	if err != nil {
		log.Errorf("Failed to get from redis the duration to wait with the next request: %v", err)
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
func (c *clusterLimitRedis) RetryAfter(clearText string) int {
	return c.RetryAfterContext(context.Background(), clearText)
}
