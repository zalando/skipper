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

	for _, tt := range []struct {
		name       string
		settings   Settings
		redisRing  *net.RedisRingClient
		valkeyRing *net.ValkeyRingClient
		group      string
		want       limiter
	}{
		{
			name:      "no ring",
			settings:  Settings{},
			redisRing: nil,
			group:     "",
			want:      voidRatelimit{},
		},
		{
			name: "a redis ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
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
			name: "a valkey ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
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
			name: "a redis and valkey ring",
			settings: Settings{
				MaxHits:    10,
				TimeWindow: 3 * time.Second,
			},
			redisRing:  redisRing,
			valkeyRing: valkeyRing,
			group:      "mygroup",
			want: &clusterLimitValkey{
				group:      "mygroup",
				maxHits:    10,
				window:     3 * time.Second,
				ringClient: valkeyRing,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.want.Close()

			got := newClusterRateLimiter(tt.settings, tt.redisRing, tt.valkeyRing, tt.group)
			defer got.Close()

			gotT := fmt.Sprintf("%T", got)
			wantT := fmt.Sprintf("%T", tt.want)
			if gotT != wantT {
				t.Errorf("Failed to get clusterRatelimiter want %v, got %v", tt.want, got)
			}
		})
	}
}
