package ratelimit

import (
	"testing"
	"time"

	"github.com/hashicorp/golang-lru"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"
)

type redisTest struct {
	name           string
	settings       Settings
	iterations     int
	delay          time.Duration
	concurrency    int
	args           string
	addrs          []string
	password       string
	want           bool
	wantAllowed    int
	wantDenied     int
	wantDuration   time.Duration
	wantOldest     time.Duration
	wantRetryAfter int
}

type createRateLimiter func(s Settings, group, redisAddr, password string) (l limiter, close func())

func createFullSync(s Settings, group, redisAddr, password string) (limiter, func()) {
	ringClient := net.NewRedisRingClient(&net.RedisOptions{
		Addrs:    []string{redisAddr},
		Password: password,
	})

	l := newClusterRateLimiterRedis(s, ringClient, group)
	return l, func() {
		l.Close()
	}
}

func createCached(s Settings, group, redisAddr, password string) (limiter, func()) {
	ringClient := net.NewRedisRingClient(&net.RedisOptions{
		Addrs:    []string{redisAddr},
		Password: password,
	})

	cache, err := lru.New(10 * 1024)
	if err != nil {
		panic(err)
	}

	l := newClusterLimitRedisCached(s, ringClient, cache, group, 0)
	return l, func() {
		l.Close()
	}
}

func runRedisTests(t *testing.T, tests []redisTest, redisAddr string, cl createRateLimiter) {
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, close := cl(tt.settings, tt.settings.Group, redisAddr, tt.password)
			defer close()

			start := time.Now()
			var (
				got     bool
				allowed int
				denied  int
			)

			if tt.concurrency == 0 {
				for i := 0; i < tt.iterations; i++ {
					got = c.Allow(tt.args)
					time.Sleep(tt.delay)
				}
			} else {
				gotc := make(chan bool, tt.iterations*tt.concurrency)
				for i := 0; i < tt.concurrency; i++ {
					go func() {
						for i := 0; i < tt.iterations; i++ {
							gotc <- c.Allow(tt.args)
							time.Sleep(tt.delay)
						}
					}()
				}

				for i := 0; i < cap(gotc); i++ {
					if <-gotc {
						allowed++
					} else {
						denied++
					}
				}
			}

			if tt.concurrency == 0 && got != tt.want {
				t.Errorf("clusterLimitRedis.Allow() = %v, want %v", got, tt.want)
			}

			if tt.concurrency > 0 {
				if allowed != tt.wantAllowed {
					t.Errorf(
						"clusterLimitRedis.Allow() = %d, want %d",
						allowed,
						tt.wantAllowed,
					)
				}

				if denied != tt.wantDenied {
					t.Errorf(
						"!clusterLimitRedis.Allow() = %d, want %d",
						denied,
						tt.wantDenied,
					)
				}
			}

			if tt.wantDuration > 0 {
				gotDuration := c.Delta(tt.args)
				if tt.wantDuration-100*time.Millisecond < gotDuration &&
					gotDuration < tt.wantDuration+100*time.Millisecond {
					t.Errorf(
						"clusterLimitRedis.Delta() = %v, want %v",
						gotDuration,
						tt.wantDuration,
					)
				}
			}

			if tt.wantOldest > 0 {
				gotOldest := c.Oldest(tt.args)
				if gotOldest.Before(start.Add(-tt.wantOldest)) &&
					start.Add(tt.wantOldest).Before(gotOldest) {
					t.Errorf(
						"clusterLimitRedis.Oldest() = %v, not within +/- %v from start %v",
						gotOldest,
						tt.wantOldest,
						start,
					)
				}
			}

			if tt.wantRetryAfter > 0 {
				if gotRA := c.RetryAfter(tt.args); gotRA != tt.wantRetryAfter {
					t.Errorf(
						"clusterLimitRedis.RetryAfter() = %v, want %v",
						gotRA,
						tt.wantRetryAfter,
					)
				}
			}
		})
	}
}

func Test_clusterLimitRedis_WithPass(t *testing.T) {
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

	tests := []redisTest{
		{
			name:       "correct password",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   redisPassword,
			iterations: 6,
			want:       false,
		},
		{
			name:       "wrong password",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   "wrong",
			iterations: 6,
			want:       true,
		},
		{
			name:       "no password",
			settings:   clusterClientlimit,
			args:       "clientAuth",
			addrs:      []string{redisAddr},
			password:   "",
			iterations: 6,
			want:       true,
		},
	}

	t.Run("redis with full sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createFullSync)
	})

	t.Run("redis with cached sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createCached)
	})
}

func Test_clusterLimitRedis_Allow(t *testing.T) {
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

	tests := []redisTest{
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

	t.Run("redis with full sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createFullSync)
	})

	t.Run("redis with cached sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createCached)
	})
}

func Test_clusterLimitRedis_Delta(t *testing.T) {
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

	tests := []redisTest{
		{
			name:         "simple test clusterRatelimit",
			settings:     clusterlimit,
			args:         "clientA",
			iterations:   1,
			want:         true,
			wantDuration: 200 * time.Millisecond,
		},
		{
			name:         "simple test clusterClientRatelimit",
			settings:     clusterClientlimit,
			args:         "clientB",
			iterations:   1,
			want:         true,
			wantDuration: 200 * time.Millisecond,
		},
	}

	t.Run("redis with full sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createFullSync)
	})

	t.Run("redis with cached sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createCached)
	})
}

func Test_clusterLimitRedis_Oldest(t *testing.T) {
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

	tests := []redisTest{
		{
			name:       "simple test clusterRatelimit",
			settings:   clusterlimit,
			args:       "clientA",
			iterations: clusterlimit.MaxHits + 1,
			wantOldest: 100 * time.Millisecond,
		},
		{
			name:       "simple test clusterClientRatelimit",
			settings:   clusterClientlimit,
			args:       "clientB",
			iterations: clusterClientlimit.MaxHits,
			want:       true,
			wantOldest: 100 * time.Millisecond,
		},
	}

	t.Run("redis with full sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createFullSync)
	})

	t.Run("redis with cached sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createCached)
	})
}

func Test_clusterLimitRedis_RetryAfter(t *testing.T) {
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

	tests := []redisTest{
		{
			name:           "simple test clusterRatelimit",
			settings:       clusterlimit,
			args:           "clientA",
			iterations:     clusterlimit.MaxHits + 1,
			wantRetryAfter: 10,
		},
		{
			name:           "simple test clusterClientRatelimit",
			settings:       clusterClientlimit,
			args:           "clientB",
			iterations:     clusterClientlimit.MaxHits,
			want:           true,
			wantRetryAfter: 5,
		},
	}

	t.Run("redis with full sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createFullSync)
	})

	t.Run("redis with cached sync", func(t *testing.T) {
		runRedisTests(t, tests, redisAddr, createCached)
	})
}
