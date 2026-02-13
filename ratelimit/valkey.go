package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
	"golang.org/x/time/rate"
)

// clusterLimitValkey stores all data required for the cluster ratelimit.
type clusterLimitValkey struct {
	failClosed bool
	typ        string
	group      string
	maxHits    int64
	window     time.Duration
	ringClient *net.ValkeyRingClient
	metrics    metrics.Metrics
	sometimes  rate.Sometimes
}

const (
	valkeyMetricsPrefix                    = "swarm.valkey."
	valkeyAllowMetricsFormat               = valkeyMetricsPrefix + "query.allow.%s"
	valkeyRetryAfterMetricsFormat          = valkeyMetricsPrefix + "query.retryafter.%s"
	valkeyAllowMetricsFormatWithGroup      = valkeyMetricsPrefix + "query.allow.%s.%s"
	valkeyRetryAfterMetricsFormatWithGroup = valkeyMetricsPrefix + "query.retryafter.%s.%s"

	valkeyAllowSpanName       = "valkey_allow"
	valkeyOldestScoreSpanName = "valkey_oldest_score"

	valkeyErrorTag                    = "valkey.error"
	valkeyErrorFailedToDetermineAllow = "failedToDetermineAllow"
	valkeyErrorFailedZRange           = "failedZRange"
	valkeyErrorFailedToEvaluate       = "failedToEvaluate"
	valkeyErrorFailedToConvert        = "failedToConvert"
)

// newClusterRateLimiterValkey creates a new clusterLimitValkey for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiterValkey(s Settings, r *net.ValkeyRingClient, group string) *clusterLimitValkey {
	if r == nil {
		return nil
	}

	rl := &clusterLimitValkey{
		failClosed: s.FailClosed,
		typ:        s.Type.String(),
		group:      group,
		maxHits:    int64(s.MaxHits),
		window:     s.TimeWindow,
		ringClient: r,
		metrics:    metrics.Default,
		sometimes:  rate.Sometimes{First: 3, Interval: 1 * time.Second},
	}

	return rl
}

func (c *clusterLimitValkey) prefixKey(clearText string) string {
	return fmt.Sprintf(swarmKeyFormat, c.group, clearText)
}

func (c *clusterLimitValkey) measureQuery(format, groupFormat string, fail *bool, start time.Time) {
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

func (c *clusterLimitValkey) commonTags() opentracing.Tags {
	return opentracing.Tags{
		string(ext.Component): "skipper",
		string(ext.SpanKind):  "client",
		"ratelimit_type":      c.typ,
		"group":               c.group,
		"max_hits":            c.maxHits,
		"window":              c.window.String(),
	}
}

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share its own data
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
// Uses provided context for creating an OpenTracing span.
func (c *clusterLimitValkey) Allow(ctx context.Context, clearText string) bool {
	c.metrics.IncCounter(valkeyMetricsPrefix + "total")
	now := time.Now()

	var span opentracing.Span
	if parentSpan := parentSpan(ctx); parentSpan != nil {
		span = c.ringClient.StartSpan(valkeyAllowSpanName, opentracing.ChildOf(parentSpan.Context()), c.commonTags())
		defer span.Finish()
	}

	allow, err := c.allow(ctx, clearText)
	failed := err != nil
	if failed {
		allow = !c.failClosed
		msgFmt := "Failed to determine if operation is allowed: %v"
		valkeySetError(span, fmt.Sprintf(msgFmt, err), valkeyErrorFailedToDetermineAllow)
		c.logError(msgFmt, err)
	}
	if span != nil {
		span.SetTag("allowed", allow)
	}

	c.measureQuery(valkeyAllowMetricsFormat, valkeyAllowMetricsFormatWithGroup, &failed, now)

	if allow {
		c.metrics.IncCounter(valkeyMetricsPrefix + "allows")
	} else {
		c.metrics.IncCounter(valkeyMetricsPrefix + "forbids") // TODO(sszuecs) forbids or better deny?
	}
	return allow
}

func (c *clusterLimitValkey) allow(ctx context.Context, clearText string) (bool, error) {
	s := getHashedKey(clearText)
	key := c.prefixKey(s)

	now := time.Now()
	nowNanos := now.UnixNano()
	clearBefore := fmt.Sprintf("%d", now.Add(-c.window).UnixNano())

	// drop all elements of the set which occurred before one interval ago.
	_, err := c.ringClient.ZRemRangeByScore(ctx, key, "0.0", clearBefore)
	if err != nil {
		return false, err
	}

	// get cardinality
	count, err := c.ringClient.ZCard(ctx, key)
	if err != nil {
		return false, err
	}

	// we increase later with ZAdd, so max-1
	if count >= c.maxHits {
		return false, nil
	}

	_, err = c.ringClient.ZAdd(ctx, key, fmt.Sprintf("%d", nowNanos), float64(nowNanos))
	if err != nil {
		return false, err
	}

	_, err = c.ringClient.Expire(ctx, key, c.window+time.Second)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Close cannot decide to teardown valkey ring, because it is not the
// owner of it.
func (c *clusterLimitValkey) Close() {}

func (c *clusterLimitValkey) deltaFrom(ctx context.Context, clearText string, from time.Time) (time.Duration, error) {
	oldest, err := c.oldest(ctx, clearText)
	if err != nil {
		return 0, err
	}

	gap := from.Sub(oldest)
	return c.window - gap, nil
}

// Delta returns the time.Duration until the next call is allowed,
// negative means immediate calls are allowed
func (c *clusterLimitValkey) Delta(clearText string) time.Duration {
	now := time.Now()
	d, err := c.deltaFrom(context.Background(), clearText, now)
	if err != nil {
		c.logError("Failed to get the duration until the next call is allowed: %v", err)

		// Earlier, we returned duration since time=0 in these error cases. It is more graceful to the
		// client applications to return 0.
		return 0
	}

	return d
}

func valkeySetError(span opentracing.Span, msg string, tagValue string) {
	setError(span, "valkey", msg, tagValue)
}

func (c *clusterLimitValkey) logError(format string, err error) {
	c.sometimes.Do(func() {
		log.Errorf(format, err)
	})
}

func (c *clusterLimitValkey) oldest(ctx context.Context, clearText string) (time.Time, error) {
	s := getHashedKey(clearText)
	key := c.prefixKey(s)
	now := time.Now()

	var span opentracing.Span
	if parentSpan := parentSpan(ctx); parentSpan != nil {
		span = c.ringClient.StartSpan(valkeyOldestScoreSpanName, opentracing.ChildOf(parentSpan.Context()), c.commonTags())
		defer span.Finish()
	}

	res, err := c.ringClient.ZRangeByScoreWithScoresFirst(ctx, key, "0.0", fmt.Sprintf("%d", now.UnixNano()), 0, 1)
	if err != nil {
		valkeySetError(span, fmt.Sprintf("Failed to execute ZRangeByScoreWithScoresFirst: %v", err), valkeyErrorFailedZRange)
		return time.Time{}, err
	}

	if len(res) == 0 {
		return time.Time{}, nil
	}

	oldest, err := strconv.ParseInt(res, 10, 64)
	if err != nil {
		msg := "failed to convert valkey msg to int64"
		valkeySetError(span, msg, valkeyErrorFailedToConvert)
		return time.Time{}, errors.New(msg)
	}

	return time.Unix(0, oldest), nil
}

// Oldest returns the oldest known request time.
//
// Performance considerations:
//
// It will use ZRANGEBYSCORE with offset 0 and count 1 to get the
// oldest item stored in valkey.
func (c *clusterLimitValkey) Oldest(clearText string) time.Time {
	t, err := c.oldest(context.Background(), clearText)
	if err != nil {
		c.logError("Failed to get the oldest known request time: %v", err)
		return time.Time{}
	}

	return t
}

// Resize is noop to implement the limiter interface
func (*clusterLimitValkey) Resize(string, int) {}

// RetryAfterContext returns seconds until next call is allowed similar to
// Delta(), but returns at least one 1 in all cases. That is being
// done, because if not the ratelimit would be too few ratelimits,
// because of how it's used in the proxy and the nature of cluster
// ratelimits being not strongly consistent across calls to Allow()
// and RetryAfter() (or Allow and RetryAfterContext accordingly).
//
// Uses context for creating an OpenTracing span.
func (c *clusterLimitValkey) RetryAfterContext(ctx context.Context, clearText string) int {
	// If less than 1s to wait -> so set to 1
	const minWait = 1

	now := time.Now()
	var queryFailure bool
	defer c.measureQuery(valkeyRetryAfterMetricsFormat, valkeyRetryAfterMetricsFormatWithGroup, &queryFailure, now)

	retr, err := c.deltaFrom(ctx, clearText, now)
	if err != nil {
		c.logError("Failed to get the duration to wait until the next request: %v", err)
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
func (c *clusterLimitValkey) RetryAfter(clearText string) int {
	return c.RetryAfterContext(context.Background(), clearText)
}
