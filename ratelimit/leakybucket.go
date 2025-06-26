package ratelimit

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
)

// ClusterLeakyBucket implements a distributed leaky bucket rate limiter using Redis.
type ClusterLeakyBucket struct {
	capacity    int
	emission    time.Duration
	labelPrefix string
	script      *net.RedisScript
	redisClient *net.RedisClient
	metrics     metrics.Metrics
	now         func() time.Time
}

const (
	leakyBucketRedisKeyPrefix = "lkb."
	leakyBucketMetricPrefix   = "leakybucket.redis."
	leakyBucketMetricLatency  = leakyBucketMetricPrefix + "latency"
	leakyBucketSpanName       = "redis_leakybucket"
)

// Implements leaky bucket algorithm as a Redis lua script.
// Redis guarantees that a script is executed in an atomic way:
// no other script or Redis command will be executed while a script is being executed.
//
// Possible optimization: substitute capacity and emission in script source code
// on script creation in order not to send them over the wire on each call.
// This way every distinct bucket configuration will get its own script.
//
// See https://redis.io/commands/eval
//
//go:embed leakybucket.lua
var leakyBucketScript string

// NewClusterLeakyBucket creates a class of leaky buckets of a given capacity and emission.
// Emission is the reciprocal of the leak rate and equals the time to leak one unit.
//
// The leaky bucket is an algorithm based on an analogy of how a bucket with a constant leak will overflow if either
// the average rate at which water is poured in exceeds the rate at which the bucket leaks or if more water than
// the capacity of the bucket is poured in all at once.
// See https://en.wikipedia.org/wiki/Leaky_bucket
func NewClusterLeakyBucket(r *Registry, capacity int, emission time.Duration) *ClusterLeakyBucket {
	return newClusterLeakyBucket(r.redisClient, capacity, emission, time.Now)
}

func newClusterLeakyBucket(redisClient *net.RedisClient, capacity int, emission time.Duration, now func() time.Time) *ClusterLeakyBucket {
	return &ClusterLeakyBucket{
		capacity:    capacity,
		emission:    emission,
		labelPrefix: fmt.Sprintf("%d-%v-", capacity, emission),
		script:      redisClient.NewScript(leakyBucketScript),
		redisClient: redisClient,
		metrics:     metrics.Default,
		now:         now,
	}
}

// Add adds an increment amount to the bucket identified by the label.
// It returns true if the amount was successfully added to the bucket or a time to wait for the next attempt.
// It also returns any error occurred during the attempt.
func (b *ClusterLeakyBucket) Add(ctx context.Context, label string, increment int) (added bool, retry time.Duration, err error) {
	if increment > b.capacity {
		// not allowed to add more than capacity and retry is not possible
		return false, 0, nil
	}

	now := b.now()
	span := b.startSpan(ctx)
	defer span.Finish()
	defer b.metrics.MeasureSince(leakyBucketMetricLatency, now)

	added, retry, err = b.add(ctx, label, increment, now)
	if err != nil {
		ext.Error.Set(span, true)
	}
	return
}

func (b *ClusterLeakyBucket) add(ctx context.Context, label string, increment int, now time.Time) (added bool, retry time.Duration, err error) {
	r, err := b.redisClient.RunScript(ctx, b.script,
		[]string{b.getBucketId(label)}, // KEYS[1] = bucket ID
		b.capacity,                     // ARGV[1] = capacity
		b.emission.Microseconds(),      // ARGV[2] = emission rate in microseconds
		increment,                      // ARGV[3] = increment amount
		now.UnixMicro(),                // ARGV[4] = current time in microseconds
	)

	if err == nil {
		x := r.(int64)
		if x >= 0 {
			added, retry = true, 0
		} else {
			added, retry = false, -time.Duration(x)*time.Microsecond
		}
	}
	return
}

func (b *ClusterLeakyBucket) getBucketId(label string) string {
	return leakyBucketRedisKeyPrefix + getHashedKey(b.labelPrefix+label)
}

func (b *ClusterLeakyBucket) startSpan(ctx context.Context) (span opentracing.Span) {
	spanOpts := []opentracing.StartSpanOption{opentracing.Tags{
		string(ext.Component):     "skipper",
		string(ext.DBType):        "redis",
		string(ext.SpanKind):      ext.SpanKindRPCClientEnum,
		"ratelimit_type":          "clusterLeakyBucket",
		"leakybucket_capacity":    b.capacity,
		"leakybucket_emission_µs": b.emission.Microseconds(),
	}}
	if parent := opentracing.SpanFromContext(ctx); parent != nil {
		spanOpts = append(spanOpts, opentracing.ChildOf(parent.Context()))
	}
	return b.redisClient.StartSpan(leakyBucketSpanName, spanOpts...)
}
