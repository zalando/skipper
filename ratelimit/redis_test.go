package ratelimit

import (
	"context"
	"log"
	"os/exec"
	"testing"
	"time"
)

func startRedis(port string) func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	cmd := exec.CommandContext(ctx, "redis-server", "--port", port)
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Run '%q %q' failed, caused by: %s", cmd.Path, cmd.Args, err)
	}
	return func() { cancel(); _ = cmd.Wait() }
}

func Test_clusterLimitRedis_Allow(t *testing.T) {
	redisPort := "16379"

	cancel := startRedis(redisPort)
	defer cancel()

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
		want       bool
	}{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: 1,
			want:       true,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: 1,
			want:       true,
		},
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: 20,
			want:       false,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: 12,
			want:       false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := newClusterRateLimiterRedis(
				tt.settings,
				&RedisOptions{Addrs: []string{"127.0.0.1:" + redisPort}},
				tt.settings.Group,
			)

			var got bool
			for i := 0; i < tt.iterations; i++ {
				got = c.Allow(tt.args)
			}
			if got != tt.want {
				t.Errorf("clusterLimitRedis.Allow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clusterLimitRedis_Delta(t *testing.T) {
	redisPort := "16380"

	cancel := startRedis(redisPort)
	defer cancel()

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
			c := newClusterRateLimiterRedis(
				tt.settings,
				&RedisOptions{Addrs: []string{"127.0.0.1:" + redisPort}},
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(tt.args)
			}
			got := c.Delta(tt.args)
			if tt.want-100*time.Millisecond < got && got < tt.want+100*time.Millisecond {
				t.Errorf("clusterLimitRedis.Delta() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clusterLimitRedis_Oldest(t *testing.T) {
	redisPort := "16381"

	cancel := startRedis(redisPort)
	defer cancel()

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
			c := newClusterRateLimiterRedis(
				tt.settings,
				&RedisOptions{Addrs: []string{"127.0.0.1:" + redisPort}},
				tt.settings.Group,
			)

			now := time.Now()
			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(tt.args)
			}
			got := c.Oldest(tt.args)
			if got.Before(now.Add(-tt.want)) && now.Add(tt.want).Before(got) {
				t.Errorf("clusterLimitRedis.Oldest() = %v, not within +/- %v from now %v", got, tt.want, now)
			}
		})
	}
}

func Test_clusterLimitRedis_RetryAfter(t *testing.T) {
	redisPort := "16382"

	cancel := startRedis(redisPort)
	defer cancel()

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
			c := newClusterRateLimiterRedis(
				tt.settings,
				&RedisOptions{Addrs: []string{"127.0.0.1:" + redisPort}},
				tt.settings.Group,
			)

			for i := 0; i < tt.iterations; i++ {
				_ = c.Allow(tt.args)
			}
			if got := c.RetryAfter(tt.args); got != tt.want {
				t.Errorf("clusterLimitRedis.RetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}
