package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"

	"github.com/stretchr/testify/assert"
)

func Test_clusterLimitRedis_WithPass(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	const redisPassword = "pass"

	redisAddr, done := redistest.NewTestRedisWithPassword(t, redisPassword)
	defer done()

	clusterClientlimit := Settings{
		Type:       ClusterClientRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    5,
		TimeWindow: time.Second,
		Group:      "Auth",
	}

	tests := []struct {
		name       string
		settings   Settings
		iterations int
		args       string
		addrs      []string
		password   string
		want       []bool
	}{
		{
			name:       "correct password",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   redisPassword,
			iterations: 6,
			want:       append(repeat(true, 5), false),
		},
		{
			name:       "wrong password, fail open",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   "wrong",
			iterations: 6,
			want:       repeat(true, 6),
		},
		{
			name:       "no password, fail open",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   "",
			iterations: 6,
			want:       repeat(true, 6),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			redisClient := net.NewRedisClient(&net.RedisOptions{
				Addrs:    tt.addrs,
				Password: tt.password,
			})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				tt.settings,
				redisClient,
				tt.settings.Group,
			)

			var got []bool
			for i := 0; i < tt.iterations; i++ {
				got = append(got, c.Allow(context.Background(), tt.args))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Benchmark_clusterLimitRedis_Allow(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping Redis container benchmark in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(b)
	defer done()

	for i := 0; i < 21; i++ {
		benchmarkName := fmt.Sprintf("ratelimit with group name of %d symbols", 1<<i)
		b.Run(benchmarkName, func(b *testing.B) {
			groupName := strings.Repeat("a", 1<<i)
			clusterClientlimit := Settings{
				Type:       ClusterClientRatelimit,
				Lookuper:   NewHeaderLookuper("X-Test"),
				MaxHits:    10,
				TimeWindow: time.Second,
				Group:      groupName,
			}

			redisClient := net.NewRedisClient(&net.RedisOptions{Addrs: []string{redisAddr}})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				clusterClientlimit,
				redisClient,
				clusterClientlimit.Group,
			)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Allow(context.Background(), "constant")
			}
		})
	}
}

func Test_clusterLimitRedis_Allow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientlimit := Settings{
		Type:       ClusterClientRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "B",
	}

	tests := []struct {
		name       string
		settings   Settings
		args       string
		iterations int
		want       []bool
	}{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: 1,
			want:       []bool{true},
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: 1,
			want:       []bool{true},
		},
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: 20,
			want:       append(repeat(true, 9), repeat(false, 11)...),
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: 12,
			want:       append(repeat(true, 9), repeat(false, 3)...),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			redisClient := net.NewRedisClient(&net.RedisOptions{Addrs: []string{redisAddr}})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				tt.settings,
				redisClient,
				tt.settings.Group,
			)

			var got []bool
			for i := 0; i < tt.iterations; i++ {
				got = append(got, c.Allow(context.Background(), tt.args))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_clusterLimitRedis_Delta(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientlimit := Settings{
		Type:       ClusterClientRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "B",
	}

	tests := []struct {
		name       string
		settings   Settings
		args       string
		iterations int
		want       time.Duration
	}{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: 1,
			want:       200 * time.Millisecond,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: 1,
			want:       200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			redisClient := net.NewRedisClient(&net.RedisOptions{Addrs: []string{redisAddr}})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				tt.settings,
				redisClient,
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			got := c.Delta(tt.args)
			// Allow for some timing variance
			tolerance := 200 * time.Millisecond
			if got < tt.want-tolerance || got > tt.want+tolerance {
				t.Errorf("clusterLimitRedis.Delta() = %v, want approx %v (tolerance %v)", got, tt.want, tolerance)
			}
		})
	}
}

func Test_clusterLimitRedis_Oldest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientlimit := Settings{
		Type:       ClusterClientRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "B",
	}

	tests := []struct {
		name       string
		settings   Settings
		args       string
		iterations int
		want       time.Duration
	}{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: clusterlimit.MaxHits + 1,
			want:       100 * time.Millisecond,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: clusterClientlimit.MaxHits,
			want:       100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			redisClient := net.NewRedisClient(&net.RedisOptions{Addrs: []string{redisAddr}})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				tt.settings,
				redisClient,
				tt.settings.Group,
			)

			now := time.Now()
			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			got := c.Oldest(tt.args)
			if got.Before(now.Add(-tt.want)) && now.Add(tt.want).Before(got) {
				t.Errorf("clusterLimitRedis.Oldest() = %v, not within +/- %v from now %v", got, tt.want, now)
			}
		})
	}
}

func Test_clusterLimitRedis_RetryAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: 10 * time.Second,
		Group:      "A",
	}
	clusterClientlimit := Settings{
		Type:       ClusterClientRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: 5 * time.Second,
		Group:      "B",
	}

	tests := []struct {
		name       string
		settings   Settings
		args       string
		iterations int
		want       int
	}{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: clusterlimit.MaxHits + 1,
			want:       10,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: clusterClientlimit.MaxHits,
			want:       5,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			redisClient := net.NewRedisClient(&net.RedisOptions{Addrs: []string{redisAddr}})
			defer redisClient.Close()
			c := newClusterRateLimiterRedis(
				tt.settings,
				redisClient,
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			if got := c.RetryAfter(tt.args); got != tt.want {
				t.Errorf("clusterLimitRedis.RetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFailOpenOnRedisError(t *testing.T) {
	dm := metrics.Default
	defer func() { metrics.Default = dm }()

	m := &metricstest.MockMetrics{}
	metrics.Default = m

	settings := Settings{
		Type:       ClusterServiceRatelimit,
		MaxHits:    10,
		TimeWindow: 10 * time.Second,
		Group:      "agroup",
	}
	// redis unavailable
	redisClient := net.NewRedisClient(&net.RedisOptions{})
	defer redisClient.Close()

	c := newClusterRateLimiterRedis(
		settings,
		redisClient,
		settings.Group,
	)

	allow := c.Allow(context.Background(), "akey")
	if !allow {
		t.Error("expected allow on error")
	}
	m.WithCounters(func(counters map[string]int64) {
		if counters["swarm.redis.total"] != 1 {
			t.Error("expected 1 total")
		}
		if counters["swarm.redis.allows"] != 1 {
			t.Error("expected 1 allow on error")
		}
		if counters["swarm.redis.forbids"] != 0 {
			t.Error("expected no forbids on error")
		}
	})
	m.WithMeasures(func(measures map[string][]time.Duration) {
		if _, ok := measures["swarm.redis.query.allow.failure.agroup"]; !ok {
			t.Error("expected query allow failure on error")
		}
	})
}

func repeat(b bool, n int) (result []bool) {
	for i := 0; i < n; i++ {
		result = append(result, b)
	}
	return
}
