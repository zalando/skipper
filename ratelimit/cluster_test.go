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

	myring := net.NewRedisRingClient(
		&net.RedisOptions{
			Addrs: []string{redisAddr},
		},
	)
	defer myring.Close()

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
		ring     *net.RedisRingClient
		group    string
		want     limiter
	}{
		{
			name:     "no swarmer nor ring",
			settings: Settings{},
			swarm:    nil,
			ring:     nil,
			group:    "",
			want:     voidRatelimit{},
		},
		{
			name: "no swarmer, a ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
			swarm: nil,
			ring:  myring,
			group: "mygroup",
			want: &clusterLimitRedis{
				group:      "mygroup",
				maxHits:    10,
				window:     3 * time.Second,
				ringClient: myring,
			},
		},
		{
			name:     "swarmer, no ring",
			settings: settings,
			swarm:    fake,
			ring:     nil,
			group:    "mygroup",
			want:     newClusterRateLimiterSwim(settings, fake, "mygroup"),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.want.Close()

			got := newClusterRateLimiter(tt.settings, tt.swarm, tt.ring, tt.group)
			defer got.Close()

			// internals in swim are created and won't be equal according to reflect.Deepequal
			gotT := fmt.Sprintf("%T", got)
			wantT := fmt.Sprintf("%T", tt.want)
			if gotT != wantT {
				t.Errorf("Failed to get clusterRatlimiter want %v, got %v", tt.want, got)
			}
		})
	}
}
