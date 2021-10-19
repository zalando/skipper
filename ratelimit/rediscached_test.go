package ratelimit

import (
	"testing"
	"time"

	"github.com/zalando/skipper/net/redistest"
)

func TestClusterLimitRedisAllowThrottled(t *testing.T) {
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

	tests := []redisTest{{
		name:       "simple test clusterRatelimit",
		settings:   clusterlimit,
		args:       "clientA",
		iterations: 1,
		delay:      30 * time.Millisecond,
		want:       true,
	}, {
		name:       "simple test clusterClientRatelimit",
		settings:   clusterClientlimit,
		args:       "clientB",
		iterations: 1,
		delay:      30 * time.Millisecond,
		want:       true,
	}, {
		name:       "simple test clusterRatelimit",
		settings:   clusterlimit,
		args:       "clientA",
		iterations: 20,
		delay:      30 * time.Millisecond,
		want:       false,
	}, {
		name:       "simple test clusterClientRatelimit",
		settings:   clusterClientlimit,
		args:       "clientB",
		iterations: 12,
		delay:      30 * time.Millisecond,
		want:       false,
	}}

	runRedisTests(t, tests, redisAddr, createCached)
}

func TestClusterLimitRedisRetryAfterThrottled(t *testing.T) {
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

	tests := []redisTest{{
		name:           "simple test clusterRatelimit",
		settings:       clusterlimit,
		args:           "clientA",
		iterations:     clusterlimit.MaxHits + 1,
		delay:          30 * time.Millisecond,
		wantRetryAfter: 10,
	}, {
		name:           "simple test clusterClientRatelimit",
		settings:       clusterClientlimit,
		args:           "clientB",
		iterations:     clusterClientlimit.MaxHits,
		delay:          30 * time.Millisecond,
		want:           true,
		wantRetryAfter: 5,
	}}

	runRedisTests(t, tests, redisAddr, createCached)
}
