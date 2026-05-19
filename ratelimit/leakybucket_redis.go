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

type ClusterLeakyBucket struct {
	capacity    int
	emission    time.Duration
	labelPrefix string
	script      *net.RedisScript
	ringClient  *net.RedisRingClient
	metrics     metrics.Metrics
	now         func() time.Time
}

const (
	leakyBucketRedisKeyPrefix = "lkb."
	leakyBucketMetricPrefix   = "leakybucket.redis."
	leakyBucketMetricLatency  = leakyBucketMetricPrefix + "latency"
	leakyBucketSpanName       = "redis_leakybucket"
)

func newClusterLeakyBucketRedis(ringClient *net.RedisRingClient, capacity int, emission time.Duration, now func() time.Time) *ClusterLeakyBucket {
	return &ClusterLeakyBucket{
		capacity:    capacity,
		emission:    emission,
		labelPrefix: fmt.Sprintf("%d-%v-", capacity, emission),
		script:      ringClient.NewScript(leakyBucketScript),
		ringClient:  ringClient,
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
	r, err := b.ringClient.RunScript(ctx, b.script,
		[]string{b.getBucketId(label)},
		b.capacity,
		b.emission.Microseconds(),
		increment,
		now.UnixMicro(),
	)

	if err != nil {
		return
	}

	x := r.(int64)
	if x >= 0 {
		added, retry = true, 0
	} else {
		added, retry = false, -time.Duration(x)*time.Microsecond
	}
	return
}

func (b *ClusterLeakyBucket) getBucketId(label string) string {
	return leakyBucketRedisKeyPrefix + getHashedKey(b.labelPrefix+label)
}

func (b *ClusterLeakyBucket) startSpan(ctx context.Context) (span opentracing.Span) {
	spanOpts := []opentracing.StartSpanOption{opentracing.Tags{
		string(ext.Component): "skipper",
		string(ext.SpanKind):  "client",
	}}
	if parent := opentracing.SpanFromContext(ctx); parent != nil {
		spanOpts = append(spanOpts, opentracing.ChildOf(parent.Context()))
	}
	return b.ringClient.StartSpan(leakyBucketSpanName, spanOpts...)
}
