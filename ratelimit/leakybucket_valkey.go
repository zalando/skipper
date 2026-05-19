package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/valkey-io/valkey-go"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
)

type ClusterLeakyBucketValkey struct {
	capacity    int
	emission    time.Duration
	labelPrefix string
	script      *valkey.Lua
	ringClient  *net.ValkeyRingClient
	metrics     metrics.Metrics
	now         func() time.Time
}

const (
	leakyBucketValkeyMetricPrefix  = "leakybucket.valkey."
	leakyBucketValkeyMetricLatency = leakyBucketValkeyMetricPrefix + "latency"
	leakyBucketValkeySpanName      = "valkey_leakybucket"
)

func newClusterLeakyBucketValkey(ringClient *net.ValkeyRingClient, capacity int, emission time.Duration, now func() time.Time) *ClusterLeakyBucketValkey {
	return &ClusterLeakyBucketValkey{
		capacity:    capacity,
		emission:    emission,
		labelPrefix: fmt.Sprintf("%d-%v-", capacity, emission),
		script:      net.NewScript(leakyBucketScript),
		ringClient:  ringClient,
		metrics:     metrics.Default,
		now:         now,
	}
}

// Add adds an increment amount to the bucket identified by the label.
// It returns true if the amount was successfully added to the bucket or a time to wait for the next attempt.
// It also returns any error occurred during the attempt.
func (b *ClusterLeakyBucketValkey) Add(ctx context.Context, label string, increment int) (added bool, retry time.Duration, err error) {
	if increment > b.capacity {
		// not allowed to add more than capacity and retry is not possible
		return false, 0, nil
	}

	now := b.now()
	span := b.startSpan(ctx)
	defer span.Finish()
	defer b.metrics.MeasureSince(leakyBucketValkeyMetricLatency, now)

	added, retry, err = b.add(ctx, label, increment, now)
	if err != nil {
		ext.Error.Set(span, true)
	}
	return
}

func (b *ClusterLeakyBucketValkey) add(ctx context.Context, label string, increment int, now time.Time) (added bool, retry time.Duration, err error) {
	msg, err := b.ringClient.RunScript(ctx, b.script,
		[]string{b.getBucketId(label)},
		strconv.FormatInt(int64(b.capacity), 10),
		strconv.FormatInt(b.emission.Microseconds(), 10),
		strconv.Itoa(increment),
		strconv.FormatInt(now.UnixMicro(), 10),
	)
	if err != nil {
		return
	}

	x, err := msg.ToInt64()
	if err != nil {
		return
	}

	if x >= 0 {
		added, retry = true, 0
	} else {
		added, retry = false, -time.Duration(x)*time.Microsecond
	}
	return
}

func (b *ClusterLeakyBucketValkey) getBucketId(label string) string {
	return leakyBucketRedisKeyPrefix + getHashedKey(b.labelPrefix+label)
}

func (b *ClusterLeakyBucketValkey) startSpan(ctx context.Context) (span opentracing.Span) {
	spanOpts := []opentracing.StartSpanOption{opentracing.Tags{
		string(ext.Component): "skipper",
		string(ext.SpanKind):  "client",
	}}
	if parent := opentracing.SpanFromContext(ctx); parent != nil {
		spanOpts = append(spanOpts, opentracing.ChildOf(parent.Context()))
	}
	return b.ringClient.StartSpan(leakyBucketValkeySpanName, spanOpts...)
}
