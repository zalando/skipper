package net

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/logging"
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
	// DialTimeout is the max time.Duration to dial a new connection
	DialTimeout time.Duration

	// PoolTimeout is the max time.Duration to get a connection from pool
	PoolTimeout time.Duration
	// IdleTimeout requires a non 0 IdleCheckFrequency
	IdleTimeout time.Duration
	// IdleCheckFrequency - reaper frequency, only used if IdleTimeout > 0
	IdleCheckFrequency time.Duration
	// MaxConnAge
	MaxConnAge time.Duration
	// MinIdleConns is the minimum number of socket connections to redis
	MinIdleConns int
	// MaxIdleConns is the maximum number of socket connections to redis
	MaxIdleConns int

	// HeartbeatFrequency frequency of PING commands sent to check
	// shards availability.
	HeartbeatFrequency time.Duration

	// ConnMetricsInterval defines the frequency of updating the redis
	// connection related metrics. Defaults to 60 seconds.
	ConnMetricsInterval time.Duration
	// MetricsPrefix is the prefix for redis ring client metrics,
	// defaults to "swarm.redis." if not set
	MetricsPrefix string
	// Tracer provides OpenTracing for Redis queries.
	Tracer opentracing.Tracer
	// Log is the logger that is used
	Log logging.Logger
}

type RedisRingClient struct {
	ring          *redis.Ring
	log           logging.Logger
	metrics       metrics.Metrics
	metricsPrefix string
	options       *RedisOptions
	tracer        opentracing.Tracer
	quit          chan struct{}
}

const (
	DefaultReadTimeout  = 25 * time.Millisecond
	DefaultWriteTimeout = 25 * time.Millisecond
	DefaultPoolTimeout  = 25 * time.Millisecond
	DefaultDialTimeout  = 25 * time.Millisecond
	DefaultMinConns     = 100
	DefaultMaxConns     = 100

	defaultConnMetricsInterval = 60 * time.Second
)

func NewRedisRingClient(ro *RedisOptions) *RedisRingClient {
	r := new(RedisRingClient)
	r.quit = make(chan struct{})
	r.metrics = metrics.Default
	r.tracer = &opentracing.NoopTracer{}

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
		ringOptions.DialTimeout = ro.DialTimeout
		ringOptions.MinIdleConns = ro.MinIdleConns
		ringOptions.PoolSize = ro.MaxIdleConns
		if ro.ConnMetricsInterval <= 0 {
			ro.ConnMetricsInterval = defaultConnMetricsInterval
		}
		if ro.Tracer != nil {
			r.tracer = ro.Tracer
		}

		r.options = ro
		r.ring = redis.NewRing(ringOptions)
		r.log = ro.Log
		r.metricsPrefix = ro.MetricsPrefix
	}

	return r
}

func (r *RedisRingClient) RingAvailable() bool {
	var err error
	err = backoff.Retry(func() error {
		_, err = r.ring.Ping(context.Background()).Result()
		if err != nil {
			r.log.Infof("Failed to ping redis, retry with backoff: %v", err)
		}
		return err
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 7))

	return err == nil
}

func (r *RedisRingClient) StartMetricsCollection() {
	go func() {
		for {
			select {
			case <-time.After(r.options.ConnMetricsInterval):
				stats := r.ring.PoolStats()
				r.metrics.UpdateGauge(r.metricsPrefix+"hits", float64(stats.Hits))
				r.metrics.UpdateGauge(r.metricsPrefix+"idleconns", float64(stats.IdleConns))
				r.metrics.UpdateGauge(r.metricsPrefix+"misses", float64(stats.Misses))
				r.metrics.UpdateGauge(r.metricsPrefix+"staleconns", float64(stats.StaleConns))
				r.metrics.UpdateGauge(r.metricsPrefix+"timeouts", float64(stats.Timeouts))
				r.metrics.UpdateGauge(r.metricsPrefix+"totalconns", float64(stats.TotalConns))
			case <-r.quit:
				return
			}
		}
	}()
}

func (r *RedisRingClient) Metrics() metrics.Metrics {
	return r.metrics
}

func (r *RedisRingClient) Tracer() opentracing.Tracer {
	return r.tracer
}

func (r *RedisRingClient) Close() {
	if r != nil {
		close(r.quit)
	}
}

func (r *RedisRingClient) ZAdd(ctx context.Context, key string, members ...*redis.Z) *redis.IntCmd {
	return r.ring.ZAdd(ctx, key, members...)
}

func (r *RedisRingClient) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return r.ring.Expire(ctx, key, expiration)
}

func (r *RedisRingClient) ZRemRangeByScore(ctx context.Context, key, min, max string) *redis.IntCmd {
	return r.ring.ZRemRangeByScore(ctx, key, min, max)
}

func (r *RedisRingClient) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return r.ring.ZCard(ctx, key)
}

func (r *RedisRingClient) ZRangeByScoreWithScores(ctx context.Context, key string, opt *redis.ZRangeBy) *redis.ZSliceCmd {
	return r.ring.ZRangeByScoreWithScores(ctx, key, opt)
}
