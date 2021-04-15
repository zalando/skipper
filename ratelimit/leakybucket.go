package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/zalando/skipper/metrics"
)

// The leaky bucket is an algorithm based on an analogy of how a bucket with a constant leak will overflow if either
// the average rate at which water is poured in exceeds the rate at which the bucket leaks or if more water than
// the capacity of the bucket is poured in all at once.
// See https://en.wikipedia.org/wiki/Leaky_bucket
type ClusterLeakyBucket struct {
	capacity      int
	emission      time.Duration
	script        *redis.Script
	ring          *redis.Ring
	metrics       metrics.Metrics
	tracer        opentracing.Tracer
	keyPrefix     string
	metricSuccess string
	metricFailure string
	now           func() time.Time
}

const (
	leakyBucketPrefix        = "leakybucket.redis."
	leakyBucketTotal         = leakyBucketPrefix + "total"
	leakyBucketAllows        = leakyBucketPrefix + "allows"
	leakyBucketForbids       = leakyBucketPrefix + "forbids"
	leakyBucketSuccessPrefix = leakyBucketPrefix + "success."
	leakyBucketFailurePrefix = leakyBucketPrefix + "failure."
	leakyBucketSpanName      = "redis_leakybucket"
)

func NewClusterLeakyBucket(r *Registry, group string, capacity int, emission time.Duration) *ClusterLeakyBucket {
	return newClusterLeakyBucket(r.redisRing, group, capacity, emission, time.Now)
}

func newClusterLeakyBucket(rr *ring, group string, capacity int, emission time.Duration, now func() time.Time) *ClusterLeakyBucket {
	return &ClusterLeakyBucket{
		capacity, emission, makeScript(),
		rr.ring, rr.metrics, rr.tracer,
		"lb." + group + ".", leakyBucketSuccessPrefix + group, leakyBucketFailurePrefix + group,
		now,
	}
}

func makeScript() *redis.Script {
	// Implements leaky bucket algorithm as Redis lua script.
	// Redis guarantees that a script is executed in an atomic way:
	// no other script or Redis command will be executed while a script is being executed.
	//
	// Possible optimization: substitute capacity and emission in script source code
	// on script creation in order not to send them over the wire on each call.
	// This way every distinct bucket configuration will get its own script.
	//
	// See https://redis.io/commands/eval
	return redis.NewScript(`
	local label     = KEYS[1]           -- bucket label
	local now       = tonumber(ARGV[1]) -- current time in ns (now >= 0)
	local increment = tonumber(ARGV[2]) -- increment in units (increment <= capacity)
	local capacity  = tonumber(ARGV[3]) -- bucket capacity in units (increment <= capacity)
	local emission  = tonumber(ARGV[4]) -- time to leak one unit in ns (emission > 0)

	--
	-- Theoretical Arrival Time - time when bucket drains out
	-- if bucket does not exist or is drained out, consider empty
	--
	local tat = redis.call('GET', label)
	if not tat then
		tat = now
	else
		tat = tonumber(tat)
		if tat < now then
			tat = now
		end
	end

	--
	-- bucket level: time to drain / emission = (tat - now) / emission
	-- free capacity after increment: capacity - increment - bucket level
	-- retry after: negative free capacity * emission
	--
	local free = (capacity - increment) * emission - (tat - now)
	if free >= 0 then
		local new = tat + increment * emission
		local expires_ms = math.ceil((new - now) / 1000000)

		redis.call('SET', label, new, 'PX', expires_ms)
	end
	return free`)
}

func (b *ClusterLeakyBucket) Check(ctx context.Context, key string, increment int) (allow bool, retry time.Duration, err error) {
	b.metrics.IncCounter(leakyBucketTotal)

	if increment <= b.capacity {
		span := b.startSpan(ctx)
		now := b.now()

		allow, retry, err = b.addToBucket(ctx, key, increment, now)

		if err != nil {
			b.metrics.MeasureSince(b.metricFailure, now)
			ext.Error.Set(span, true)
		} else {
			b.metrics.MeasureSince(b.metricSuccess, now)
		}
		span.Finish()
	} else {
		// not allowed to add more than capacity and retry is not possible
		allow, retry = false, 0
	}

	if allow {
		b.metrics.IncCounter(leakyBucketAllows)
	} else {
		b.metrics.IncCounter(leakyBucketForbids)
	}
	return
}

func (b *ClusterLeakyBucket) startSpan(ctx context.Context) (span opentracing.Span) {
	parent := opentracing.SpanFromContext(ctx)
	if parent != nil {
		span = b.tracer.StartSpan(leakyBucketSpanName, opentracing.ChildOf(parent.Context()))
	} else {
		span = opentracing.NoopTracer{}.StartSpan("")
	}
	ext.Component.Set(span, "skipper")
	ext.SpanKind.Set(span, "client")
	return
}

func (b *ClusterLeakyBucket) addToBucket(ctx context.Context, key string, increment int, now time.Time) (allow bool, retry time.Duration, err error) {
	r, err := b.script.Run(ctx, b.ring,
		[]string{b.getBucketLabel(key)},
		now.UnixNano(),
		increment,
		b.capacity,
		int64(b.emission),
	).Int64()

	if err == nil {
		if r >= 0 {
			allow, retry = true, 0
		} else {
			allow, retry = false, time.Duration(-r)
		}
	}
	return
}

func (b *ClusterLeakyBucket) getBucketLabel(key string) string {
	h := sha256.Sum256([]byte(key))
	return b.keyPrefix + hex.EncodeToString(h[:8])
}
