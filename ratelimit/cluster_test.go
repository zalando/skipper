package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"
)

func Test_newClusterRateLimiter(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	// Unified client for both Ring and Cluster modes
	myRedisClient := net.NewRedisClient(
		&net.RedisOptions{
			Addrs: []string{redisAddr},
		},
	)
	defer myRedisClient.Close()

	fake, err := newFakeSwarm("foo01", 3*time.Second)
	if err != nil {
		t.Fatalf("Failed to create fake swarm to test: %v", err)
	}
	defer fake.Leave()
	settings := Settings{
		MaxHits:    10,
		TimeWindow: 3 * time.Second,
		Type:       ClusterServiceRatelimit,
	}

	for _, tt := range []struct {
		name     string
		settings Settings
		swarm    Swarmer
		client   *net.RedisClient
		group    string
		want     limiter
	}{
		{
			name:     "no swarmer nor ring",
			settings: Settings{},
			swarm:    nil,
			client:   nil,
			group:    "",
			want:     voidRatelimit{},
		},
		{
			name: "no swarmer, a ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
			swarm:  nil,
			client: myRedisClient,
			group:  "mygroup",
			want: &clusterLimitRedis{
				group:       "mygroup",
				maxHits:     10,
				window:      3 * time.Second,
				redisClient: myRedisClient,
			},
		},
		{
			name:     "swarmer, no ring",
			settings: settings,
			swarm:    fake,
			client:   nil,
			group:    "mygroup",
			want:     newClusterRateLimiterSwim(settings, fake, "mygroup"),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.want.Close()

			got := newClusterRateLimiter(tt.settings, tt.swarm, tt.client, tt.group)
			defer got.Close()

			// internals in swim are created and won't be equal with reflect.Deepequal
			gotT := fmt.Sprintf("%T", got)
			wantT := fmt.Sprintf("%T", tt.want)
			if gotT != wantT {
				t.Errorf("Failed to get clusterRatlimiter want %v, got %v", tt.want, got)
			}
		})
	}
}
