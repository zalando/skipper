package net

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/tracing/tracers/basic"
)

func TestRedisContainer(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	if redisAddr == "" {
		t.Fatal("Failed to create redis 1")
	}
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()
	if redisAddr2 == "" {
		t.Fatal("Failed to create redis 2")
	}
}

func Test_hasAll(t *testing.T) {
	for _, tt := range []struct {
		name string
		a    []string
		h    map[string]struct{}
		want bool
	}{
		{
			name: "both empty",
			a:    nil,
			h:    nil,
			want: true,
		},
		{
			name: "a empty",
			a:    nil,
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "h empty",
			a:    []string{"foo"},
			h:    nil,
			want: false,
		},
		{
			name: "both set equal",
			a:    []string{"foo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: true,
		},
		{
			name: "both set notequal",
			a:    []string{"fo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "both set multiple equal",
			a:    []string{"bar", "foo"},
			h: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
			want: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAll(tt.a, tt.h)
			if tt.want != got {
				t.Fatalf("Failed to get %v for hasall(%v, %v)", tt.want, tt.a, tt.h)
			}
		})
	}
}

func TestRedisClient(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}
	defer tracer.Close()

	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()

	updater := &addressUpdater{addrs: []string{redisAddr, redisAddr2}}

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		wantErr bool
	}{
		{
			name: "All defaults",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
			},
			wantErr: false,
		},
		{
			name: "With AddrUpdater",
			options: &RedisOptions{
				AddrUpdater:    updater.update,
				UpdateInterval: 10 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name: "With tracer",
			options: &RedisOptions{
				Addrs:  []string{redisAddr},
				Tracer: tracer,
			},
			wantErr: false,
		},
		{
			name: "With metrics",
			options: &RedisOptions{
				Addrs:               []string{redisAddr},
				ConnMetricsInterval: 10 * time.Millisecond,
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer func() {
				if !cli.closed {
					t.Error("Failed to close redis ring client")
				}
			}()
			defer cli.Close()

			if !cli.RingAvailable() {
				t.Error("Failed to have a connected redis client, ring not available")
			}

			if tt.options.AddrUpdater != nil {
				// test address updater is called
				initial := updater.calls()

				time.Sleep(2 * cli.options.UpdateInterval)

				if updater.calls() == initial {
					t.Errorf("expected updater call")
				}

				// test close stops background update
				cli.Close()

				time.Sleep(2 * cli.options.UpdateInterval)

				afterClose := updater.calls()

				time.Sleep(2 * cli.options.UpdateInterval)

				if updater.calls() != afterClose {
					t.Errorf("expected no updater call")
				}

				if !cli.closed {
					t.Error("Failed to close")
				}
			}

			if tt.options.Tracer != nil {
				span := cli.StartSpan("test")
				span.Finish()
			}

			if tt.options.ConnMetricsInterval != defaultConnMetricsInterval {
				cli.StartMetricsCollection()
				time.Sleep(tt.options.ConnMetricsInterval)
			}
		})
	}
}

func TestRedisClientGetSet(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

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
				Addrs: []string{redisAddr},
			},
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
			},
			key:     "k1",
			value:   "foo",
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, with expiration",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			wait:    1100 * time.Millisecond,
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration, with Rendezvous hash",
			options: &RedisOptions{
				Addrs:         []string{redisAddr},
				HashAlgorithm: "rendezvous",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Rendezvous Vnodes hash",
			options: &RedisOptions{
				Addrs:         []string{redisAddr},
				HashAlgorithm: "rendezvousVnodes",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Jump hash",
			options: &RedisOptions{
				Addrs:         []string{redisAddr},
				HashAlgorithm: "jump",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Multiprobe hash",
			options: &RedisOptions{
				Addrs:         []string{redisAddr},
				HashAlgorithm: "mpchash",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
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
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

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
				Addrs: []string{redisAddr},
			},
			wantErr: true,
		},
		{
			name: "add one",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

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
				Addrs: []string{redisAddr},
			},
			zcard:            0,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
		{
			name: "add one which does not expire",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

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
				Addrs: []string{redisAddr},
			},
			wantErr: true,
		},
		{
			name: "delete none",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

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
				Addrs: []string{redisAddr},
			},
			wantErr: true,
		},
		{
			name: "one key, have one value, get first by min max",
			options: &RedisOptions{
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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
				Addrs: []string{redisAddr},
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

func TestRedisClientSetAddr(t *testing.T) {
	redisAddr1, done1 := redistest.NewTestRedis(t)
	defer done1()
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()

	for _, tt := range []struct {
		name        string
		options     *RedisOptions
		redisUpdate []string
		keys        []string
		vals        []string
	}{
		{
			name: "no redis change",
			options: &RedisOptions{
				Addrs: []string{redisAddr1, redisAddr2},
			},
			keys: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
			vals: []string{"bar1", "bar2", "bar3", "bar4", "bar5"},
		},
		{
			name: "with redis change",
			options: &RedisOptions{
				Addrs: []string{redisAddr1},
			},
			redisUpdate: []string{
				redisAddr1,
				redisAddr2,
			},
			keys: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
			vals: []string{"bar1", "bar2", "bar3", "bar4", "bar5"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRedisRingClient(tt.options)
			defer r.Close()
			for i := 0; i < len(tt.keys); i++ {
				r.Set(context.Background(), tt.keys[i], tt.vals[i], time.Second)
			}
			if len(tt.redisUpdate) != len(tt.options.Addrs) {
				r.SetAddrs(context.Background(), tt.redisUpdate)
			}
			for i := 0; i < len(tt.keys); i++ {
				got, err := r.Get(context.Background(), tt.keys[i])
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.vals[i] {
					t.Errorf("Failed to get key '%s' wanted '%s', got '%s'", tt.keys[i], tt.vals[i], got)
				}
			}
		})
	}
}

func TestRedisClientFailingAddrUpdater(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cli := NewRedisRingClient(&RedisOptions{
			AddrUpdater: func() ([]string, error) {
				return nil, fmt.Errorf("failed to get addresses")
			},
			UpdateInterval: 1 * time.Second,
		})
		defer cli.Close()

		if cli.RingAvailable() {
			t.Error("Unexpected available ring")
		}
	})
}
