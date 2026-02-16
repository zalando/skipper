package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/valkeytest"

	"github.com/stretchr/testify/assert"
)

func Test_clusterLimitValkey_WithPass(t *testing.T) {
	const valkeyPassword = "pass"

	valkeyAddr, done := valkeytest.NewTestValkeyWithPassword(t, valkeyPassword)
	defer done()

	clusterClientLimit := Settings{
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
		wantErr    bool
	}{
		{
			name:       "correct password",
			settings:   clusterClientLimit,
			args:       "clientAuth",
			addrs:      []string{valkeyAddr},
			password:   valkeyPassword,
			iterations: 6,
			want:       append(repeat(true, 5), false),
		},
		{
			name:     "wrong password, fail",
			addrs:    []string{valkeyAddr},
			password: "wrong",
			wantErr:  true,
		},
		{
			name:     "no password, fail",
			addrs:    []string{valkeyAddr},
			password: "",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{
				Addrs:    []string{valkeyAddr},
				Password: tt.password,
			})
			if err != nil && tt.wantErr {
				// valkey client returns an error not as redis which needs special handling to detect errors and ignore errors on creation of the client.
				return
			}
			if err != nil {
				t.Fatalf("Failed to create ValkeyRingClient: %v", err)
			}

			defer ringClient.Close()

			c := newClusterRateLimiterValkey(
				tt.settings,
				ringClient,
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

func Benchmark_clusterLimitValkey_Allow(b *testing.B) {
	valkeyAddr, done := valkeytest.NewTestValkey(b)
	defer done()

	for i := range 21 {
		benchmarkName := fmt.Sprintf("ratelimit with group name of %d symbols", 1<<i)
		b.Run(benchmarkName, func(b *testing.B) {
			groupName := strings.Repeat("a", 1<<i)
			clusterClientLimit := Settings{
				Type:       ClusterClientRatelimit,
				Lookuper:   NewHeaderLookuper("X-Test"),
				MaxHits:    10,
				TimeWindow: time.Second,
				Group:      groupName,
			}

			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
			assert.Nil(b, err, "Failed to create ValkeyRingClient")
			defer ringClient.Close()
			c := newClusterRateLimiterValkey(
				clusterClientLimit,
				ringClient,
				clusterClientLimit.Group,
			)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Allow(context.Background(), "constant")
			}
		})
	}
}

func Test_clusterLimitValkey_Allow(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientLimit := Settings{
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
			settings:   clusterClientLimit,
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
			settings:   clusterClientLimit,
			args:       "clientB",
			iterations: 12,
			want:       append(repeat(true, 9), repeat(false, 3)...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
			assert.Nil(t, err, "Failed to create ValkeyRingClient")
			defer ringClient.Close()
			c := newClusterRateLimiterValkey(
				tt.settings,
				ringClient,
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

func Test_clusterLimitValkey_Delta(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientLimit := Settings{
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
			settings:   clusterClientLimit,
			args:       "clientB",
			iterations: 1,
			want:       200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
			assert.Nil(t, err, "Failed to create ValkeyRingClient")
			defer ringClient.Close()
			c := newClusterRateLimiterValkey(
				tt.settings,
				ringClient,
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			got := c.Delta(tt.args)
			if tt.want-100*time.Millisecond < got && got < tt.want+100*time.Millisecond {
				t.Errorf("clusterLimitValkey.Delta() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clusterLimitValkey_Oldest(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: time.Second,
		Group:      "A",
	}
	clusterClientLimit := Settings{
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
			settings:   clusterClientLimit,
			args:       "clientB",
			iterations: clusterClientLimit.MaxHits,
			want:       100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
			assert.Nil(t, err, "Failed to create ValkeyRingClient")
			defer ringClient.Close()
			c := newClusterRateLimiterValkey(
				tt.settings,
				ringClient,
				tt.settings.Group,
			)

			now := time.Now()
			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			got := c.Oldest(tt.args)
			if got.Before(now.Add(-tt.want)) && now.Add(tt.want).Before(got) {
				t.Errorf("clusterLimitValkey.Oldest() = %v, not within +/- %v from now %v", got, tt.want, now)
			}
		})
	}
}

func Test_clusterLimitValkey_RetryAfter(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	clusterlimit := Settings{
		Type:       ClusterServiceRatelimit,
		Lookuper:   NewHeaderLookuper("X-Test"),
		MaxHits:    10,
		TimeWindow: 10 * time.Second,
		Group:      "A",
	}
	clusterClientLimit := Settings{
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
			settings:   clusterClientLimit,
			args:       "clientB",
			iterations: clusterClientLimit.MaxHits,
			want:       5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
			assert.Nil(t, err, "Failed to create ValkeyRingClient")
			defer ringClient.Close()
			c := newClusterRateLimiterValkey(
				tt.settings,
				ringClient,
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(context.Background(), tt.args)
			}
			if got := c.RetryAfter(tt.args); got != tt.want {
				t.Errorf("clusterLimitValkey.RetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

/* TODO(sszuecs): fail open on rate limit side
func TestFailOpenOnValkeyError(t *testing.T) {
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
	// valkey unavailable
	ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{})
	assert.Nil(t, err, "Failed to create ValkeyRingClient")
	defer ringClient.Close()

	c := newClusterRateLimiterValkey(
		settings,
		ringClient,
		settings.Group,
	)

	allow := c.Allow(context.Background(), "akey")
	if !allow {
		t.Error("expected allow on error")
	}
	m.WithCounters(func(counters map[string]int64) {
		if counters["swarm.valkey.total"] != 1 {
			t.Error("expected 1 total")
		}
		if counters["swarm.valkey.allows"] != 1 {
			t.Error("expected 1 allow on error")
		}
		if counters["swarm.valkey.forbids"] != 0 {
			t.Error("expected no forbids on error")
		}
	})
	m.WithMeasures(func(measures map[string][]time.Duration) {
		if _, ok := measures["swarm.valkey.query.allow.failure.agroup"]; !ok {
			t.Error("expected query allow failure on error")
		}
	})
}
*/
