// +build !race redis

package ratelimit

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"testing"
	"time"
)

func startRedis2(port string) func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	cmd := exec.CommandContext(ctx, "redis-server", "--port", port)
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Run '%q %q' failed, caused by: %s", cmd.Path, cmd.Args, err)
	}
	return func() { cancel(); _ = cmd.Wait() }
}

func Test_newClusterRateLimiter(t *testing.T) {
	cancel := startRedis2("16079")
	defer cancel()

	quit := make(chan struct{})
	myring := newRing(
		&RedisOptions{
			Addrs: []string{"127.0.0.1:16079"},
		},
		quit,
	)
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
		ring     *ring
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
				group:   "mygroup",
				maxHits: 10,
				window:  3 * time.Second,
				ring:    myring.ring,
				metrics: myring.metrics,
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
			got := newClusterRateLimiter(tt.settings, tt.swarm, tt.ring, tt.group)
			// internals in swim are created and won't be equal with reflect.Deepequal
			gotT := fmt.Sprintf("%T", got)
			wantT := fmt.Sprintf("%T", tt.want)
			if gotT != wantT {
				t.Errorf("Failed to get clusterRatlimiter want %v, got %v", tt.want, got)
			}
		})
	}

}
