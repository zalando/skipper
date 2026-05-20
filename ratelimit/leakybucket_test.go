package ratelimit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/net/valkeytest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type attempt struct {
	tplus int
	added bool
	retry int
}

func TestLeakyBucketAdd(t *testing.T) {
	verifyAttempts(t, 3, time.Minute, 1, []attempt{
		// initial burst of three units fills up capacity
		{+0, true, 0}, // just added unit leaks after 60s
		{+1, true, 0}, // after 59
		{+2, true, 0}, // after 58
		// the bucket is full
		{+3, false, 57},
		{+4, false, 56},
		// ...
		{+58, false, 2},
		{+59, false, 1},
		// by this point one unit has leaked
		{+60, true, 0},
		// the bucket is full again
		{+61, false, 59},
		{+62, false, 58},
		{+63, false, 57},
		// ... wait two minutes to add two units
		{+180, true, 0},
		{+181, true, 0},
		// the bucket is full again
		{+182, false, 58},
		{+183, false, 57},
		// ...
	})
}

func TestLeakyBucketAddMoreThanCapacity(t *testing.T) {
	verifyAttempts(t, 1, time.Minute, 2, []attempt{
		{+0, false, 0},  // not allowed and no retry possible
		{+61, false, 0}, // even after a minute
	})
}

func TestLeakyBucketAddAtSlowRate(t *testing.T) {
	verifyAttempts(t, 1, time.Second/2, 1, []attempt{
		{+0, true, 0},
		{+1, true, 0},
		{+2, true, 0},
	})
}

func verifyAttempts(t *testing.T, capacity int, emission time.Duration, increment int, attempts []attempt) {
	t.Helper()

	type backend struct {
		name      string
		newBucket func(t *testing.T, now func() time.Time) LeakyBucketLimiter
	}

	backends := []backend{
		{
			name: "redis",
			newBucket: func(t *testing.T, now func() time.Time) LeakyBucketLimiter {
				t.Helper()
				redisAddr, done := redistest.NewTestRedis(t)
				t.Cleanup(done)
				ringClient := net.NewRedisRingClient(&net.RedisOptions{Addrs: []string{redisAddr}})
				t.Cleanup(ringClient.Close)
				return newClusterLeakyBucketRedis(ringClient, capacity, emission, now)
			},
		},
		{
			name: "valkey",
			newBucket: func(t *testing.T, now func() time.Time) LeakyBucketLimiter {
				t.Helper()
				valkeyAddr, done := valkeytest.NewTestValkey(t)
				t.Cleanup(done)
				ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
				require.NoError(t, err)
				t.Cleanup(func() { ringClient.Close() })
				return newClusterLeakyBucketValkey(ringClient, capacity, emission, now)
			},
		},
	}

	for _, b := range backends {
		b := b
		t.Run(b.name, func(t *testing.T) {
			now := time.Now()
			bucket := b.newBucket(t, func() time.Time { return now })
			t0 := now
			for _, a := range attempts {
				now = t0.Add(time.Duration(a.tplus) * time.Second)
				added, retry, err := bucket.Add(context.Background(), "alabel", increment)
				if err != nil {
					t.Fatal(err)
				}
				if a.added != added {
					t.Errorf("error at %+d: added mismatch, expected %v, got %v", a.tplus, a.added, added)
				}
				expectedRetry := time.Duration(a.retry) * time.Second
				if expectedRetry != retry {
					t.Errorf("error at %+d: retry mismatch, expected %v, got %v", a.tplus, expectedRetry, retry)
				}
			}
		})
	}
}

func TestLeakyBucketError(t *testing.T) {
	t.Run("redis", func(t *testing.T) {
		ringClient := net.NewRedisRingClient(&net.RedisOptions{Addrs: []string{"no-such-host.test:123"}})
		defer ringClient.Close()

		bucket := newClusterLeakyBucketRedis(ringClient, 1, time.Minute, time.Now)
		_, _, err := bucket.Add(context.Background(), "alabel", 1)

		assert.Error(t, err)
	})
	t.Run("valkey", func(t *testing.T) {
		ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{"no-such-host.test:123"}})
		if err != nil {
			// error at construction time is acceptable for Valkey with unreachable address
			return
		}
		defer ringClient.Close()

		bucket := newClusterLeakyBucketValkey(ringClient, 1, time.Minute, time.Now)
		_, _, err = bucket.Add(context.Background(), "alabel", 1)

		assert.Error(t, err)
	})
}

func TestLeakyBucketId(t *testing.T) {
	const label = "alabel"

	t.Run("redis", func(t *testing.T) {
		b1 := newClusterLeakyBucketRedis(nil, 1, time.Minute, time.Now)
		b2 := newClusterLeakyBucketRedis(nil, 2, time.Minute, time.Now)
		assert.NotEqual(t, b1.getBucketId(label), b2.getBucketId(label))
	})
	t.Run("valkey", func(t *testing.T) {
		b1 := newClusterLeakyBucketValkey(nil, 1, time.Minute, time.Now)
		b2 := newClusterLeakyBucketValkey(nil, 2, time.Minute, time.Now)
		assert.NotEqual(t, b1.getBucketId(label), b2.getBucketId(label))
	})
}

func TestLeakyBucketStoredNumberPrecision(t *testing.T) {
	const (
		capacity  = 10
		emission  = time.Second
		label     = "alabel"
		increment = 2
	)

	t.Run("redis", func(t *testing.T) {
		redisAddr, done := redistest.NewTestRedis(t)
		defer done()

		ringClient := net.NewRedisRingClient(&net.RedisOptions{Addrs: []string{redisAddr}})
		defer ringClient.Close()

		now := time.Now()
		b := newClusterLeakyBucketRedis(ringClient, capacity, emission, func() time.Time { return now })

		_, _, err := b.Add(context.Background(), label, increment)
		require.NoError(t, err)

		v, err := ringClient.Get(context.Background(), b.getBucketId(label))
		require.NoError(t, err)

		expected := now.UnixMicro() + (increment * emission).Microseconds()
		assert.Equal(t, fmt.Sprintf("%d", expected), v)
	})
	t.Run("valkey", func(t *testing.T) {
		valkeyAddr, done := valkeytest.NewTestValkey(t)
		defer done()

		ringClient, err := net.NewValkeyRingClient(&net.ValkeyOptions{Addrs: []string{valkeyAddr}})
		require.NoError(t, err)
		defer ringClient.Close()

		now := time.Now()
		b := newClusterLeakyBucketValkey(ringClient, capacity, emission, func() time.Time { return now })

		_, _, err = b.Add(context.Background(), label, increment)
		require.NoError(t, err)

		v, err := ringClient.Get(context.Background(), b.getBucketId(label))
		require.NoError(t, err)

		expected := now.UnixMicro() + (increment * emission).Microseconds()
		assert.Equal(t, fmt.Sprintf("%d", expected), v)
	})
}
