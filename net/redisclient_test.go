package net

import (
	"context"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/tracing/tracers/basic"
)

func startRedis(port, password string) func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	cmdArgs := []string{"--port", port}
	if password != "" {
		cmdArgs = append(cmdArgs, "--requirepass")
		cmdArgs = append(cmdArgs, password)
	}
	cmd := exec.CommandContext(ctx, "redis-server", cmdArgs...)
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Run '%q %q' failed, caused by: %s", cmd.Path, cmd.Args, err)
	}

	return func() { cancel(); _ = cmd.Wait() }
}

func TestRedisClient(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}

	redisPort := "16383"
	cancel := startRedis(redisPort, "")
	defer cancel()

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		wantErr bool
	}{
		{
			name: "All defaults",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: false,
		},
		{
			name: "With tracer",
			options: &RedisOptions{
				Addrs:  []string{"127.0.0.1:" + redisPort},
				Tracer: tracer,
			},
			wantErr: false,
		},
		{
			name: "With metrics",
			options: &RedisOptions{
				Addrs:               []string{"127.0.0.1:" + redisPort},
				ConnMetricsInterval: 10 * time.Millisecond,
			},
			wantErr: false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()

			if !cli.RingAvailable() {
				t.Errorf("Failed to have a connected redis client, ring not available")
			}

			// can't compare these
			// if tt.options.Tracer != opentracing.Tracer{} { // cli.tracer == opentracing.Tracer{}{
			// 	t.Errorf("Found an unexpected tracer, want: %v, got: %v", tt.options.Tracer, cli.tracer)
			// }

			if tt.options.ConnMetricsInterval != defaultConnMetricsInterval {
				cli.StartMetricsCollection()
				time.Sleep(tt.options.ConnMetricsInterval)
			}
		})
	}
}

func TestRedisClientGetSet(t *testing.T) {
	redisPort := "16384"
	cancel := startRedis(redisPort, "")
	defer cancel()

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		key     string
		value   interface{}
		expire  time.Duration
		wait    time.Duration
		expect  interface{}
		wantErr bool
	}{
		{
			name: "add none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key:     "k1",
			value:   "foo",
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, with expiration",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get none, with expiration, wait to expire",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			wait:    1100 * time.Millisecond,
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()
			ctx := context.Background()

			_, err := cli.Set(ctx, tt.key, tt.value, tt.expire)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Set error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			time.Sleep(tt.wait)

			val, err := cli.Get(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Get error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.expect {
				t.Errorf("Failed to get correct Get value, want '%v', got '%v'", tt.expect, val)
			}
		})
	}
}

func TestRedisClientZAddZCard(t *testing.T) {
	redisPort := "16384"
	cancel := startRedis(redisPort, "")
	defer cancel()

	type valScore struct {
		val   int64
		score float64
	}
	for _, tt := range []struct {
		name    string
		options *RedisOptions
		h       map[string][]valScore
		key     string
		zcard   int64
		wantErr bool
	}{
		{
			name: "add none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: true,
		},
		{
			name: "add one",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			zcard:   1,
			wantErr: false,
		},
		{
			name: "add one more values",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			zcard:   3,
			wantErr: false,
		},
		{
			name: "add 2 keys and values",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
				"k2": {
					{
						val:   1,
						score: 1.0,
					},
				},
			},
			zcard:   3,
			wantErr: false,
		},
		{
			name: "add 2 keys and values",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k1": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
				"k2": {
					{
						val:   1,
						score: 1.0,
					},
				},
			},
			zcard:   1,
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}
				}
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}

func TestRedisClientExpire(t *testing.T) {
	redisPort := "16385"
	cancel := startRedis(redisPort, "")
	defer cancel()

	type valScore struct {
		val   int64
		score float64
	}
	for _, tt := range []struct {
		name             string
		options          *RedisOptions
		h                map[string][]valScore
		key              string
		wait             time.Duration
		expire           time.Duration // >=1s, because Redis
		zcard            int64
		zcardAfterExpire int64
		wantErr          bool
	}{
		{
			name: "add none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			zcard:            0,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
		{
			name: "add one which does not expire",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			expire:           time.Second,
			zcard:            1,
			zcardAfterExpire: 1,
			wantErr:          false,
		},
		{
			name: "add one which does expire",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			expire:           1 * time.Second,
			wait:             1100 * time.Millisecond,
			zcard:            1,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
		{
			name: "add one more values expire all",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			expire:           1 * time.Second,
			wait:             1100 * time.Millisecond,
			zcard:            3,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}

				}
				cli.Expire(ctx, k, tt.expire)
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			time.Sleep(tt.wait)

			val, err = cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcardAfterExpire {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcardAfterExpire, val)
			}

		})
	}
}

func TestRedisClientZRemRangeByScore(t *testing.T) {
	redisPort := "16384"
	cancel := startRedis(redisPort, "")
	defer cancel()

	type valScore struct {
		val   int64
		score float64
	}
	for _, tt := range []struct {
		name          string
		options       *RedisOptions
		h             map[string][]valScore
		key           string
		delScoreMin   float64
		delScoreMax   float64
		zcard         int64
		zcardAfterRem int64
		wantErr       bool
	}{
		{
			name: "none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: true,
		},
		{
			name: "delete none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			zcard:         1,
			zcardAfterRem: 1,
			delScoreMin:   6.0,
			delScoreMax:   7.0,
			wantErr:       false,
		},
		{
			name: "delete one",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			zcard:         1,
			zcardAfterRem: 0,
			delScoreMin:   1.0,
			delScoreMax:   7.0,
			wantErr:       false,
		},
		{
			name: "delete one have more values",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			zcard:         3,
			zcardAfterRem: 2,
			delScoreMin:   1.0,
			delScoreMax:   1.5,
			wantErr:       false,
		},
		{
			name: "delete 2 have more values offset score",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			zcard:         3,
			zcardAfterRem: 1,
			delScoreMin:   2.0,
			delScoreMax:   5.0,
			wantErr:       false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}
				}
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			_, err = cli.ZRemRangeByScore(ctx, tt.key, tt.delScoreMin, tt.delScoreMax)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZRemRangeByScore error = %v, wantErr %v", err, tt.wantErr)
			}

			val, err = cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcardAfterRem {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}

func TestRedisClientZRangeByScoreWithScoresFirst(t *testing.T) {
	redisPort := "16384"
	cancel := startRedis(redisPort, "")
	defer cancel()

	type valScore struct {
		val   int64
		score float64
	}
	for _, tt := range []struct {
		name     string
		options  *RedisOptions
		h        map[string][]valScore
		key      string
		min      float64
		max      float64
		offset   int64
		count    int64
		expected string
		wantErr  bool
	}{
		{
			name: "none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: true,
		},
		{
			name: "one key, have one value, get first by min max",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			min:      1.0,
			max:      7.0,
			expected: "10",
			wantErr:  false,
		},
		{
			name: "one key, have one value, get none by min max",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			min:     6.0,
			max:     7.0,
			wantErr: false,
		},
		{
			name: "one key, have one value, get none by offset",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   10,
						score: 5.0,
					},
				},
			},
			min:     1.0,
			max:     7.0,
			offset:  3,
			wantErr: false,
		},
		{
			name: "one key, have more values, get last by offset",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			min:      1.0,
			max:      5.0,
			offset:   2,
			count:    10,
			expected: "3",
			wantErr:  false,
		},
		{
			name: "one key, have more values, get second by offset",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			min:      1.0,
			max:      5.0,
			offset:   1,
			count:    10,
			expected: "2",
			wantErr:  false,
		},
		{
			name: "one key, have more values, select all get first",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   1,
						score: 1.0,
					}, {
						val:   2,
						score: 2.0,
					}, {
						val:   3,
						score: 3.0,
					},
				},
			},
			min:      0.0,
			max:      5.0,
			offset:   0,
			count:    10,
			expected: "1",
			wantErr:  false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}
				}
			}

			res, err := cli.ZRangeByScoreWithScoresFirst(ctx, tt.key, tt.min, tt.max, tt.offset, tt.count)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZRangeByScoreWithScoresFirst error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.expected == "" {
				if res != nil {
					t.Errorf("Expected nil got: '%v'", res)
				}
			} else {
				diff := cmp.Diff(res, tt.expected)
				if diff != "" {
					t.Error(diff)
				}
			}

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}
