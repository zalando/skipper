package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/net/valkeytest"
)

func Test_newClusterRateLimiter(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	valkeyAddr, done2 := valkeytest.NewTestValkey(t)
	defer done2()

	redisRing := net.NewRedisRingClient(
		&net.RedisOptions{
			Addrs: []string{redisAddr},
		},
	)
	defer redisRing.Close()

	valkeyRing, err := net.NewValkeyRingClient(&net.ValkeyOptions{
		Addrs: []string{valkeyAddr},
	})
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer valkeyRing.Close()

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
		name       string
		settings   Settings
		swarm      Swarmer
		redisRing  *net.RedisRingClient
		valkeyRing *net.ValkeyRingClient
		group      string
		want       limiter
	}{
		{
			name:      "no swarmer nor ring",
			settings:  Settings{},
			swarm:     nil,
			redisRing: nil,
			group:     "",
			want:      voidRatelimit{},
		},
		{
			name: "no swarmer, a redis ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
			swarm:     nil,
			redisRing: redisRing,
			group:     "mygroup",
			want: &clusterLimitRedis{
				group:      "mygroup",
				maxHits:    10,
				window:     3 * time.Second,
				ringClient: redisRing,
			},
		},
		{
			name: "no swarmer, a valkey ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
			swarm:      nil,
			valkeyRing: valkeyRing,
			group:      "mygroup",
			want: &clusterLimitValkey{
				group:      "mygroup",
				maxHits:    10,
				window:     3 * time.Second,
				ringClient: valkeyRing,
			},
		},
		{
			name:      "swarmer, no ring",
			settings:  settings,
			swarm:     fake,
			redisRing: nil,
			group:     "mygroup",
			want:      newClusterRateLimiterSwim(settings, fake, "mygroup"),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.want.Close()

			got := newClusterRateLimiter(tt.settings, tt.swarm, tt.redisRing, tt.valkeyRing, tt.group)
			defer got.Close()

			// internals in swim are created and won't be equal according to reflect.Deepequal
			gotT := fmt.Sprintf("%T", got)
			wantT := fmt.Sprintf("%T", tt.want)
			if gotT != wantT {
				t.Errorf("Failed to get clusterRatelimiter want %v, got %v", tt.want, got)
			}
		})
	}
}
