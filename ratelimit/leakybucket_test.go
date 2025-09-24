package ratelimit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type attempt struct {
	tplus int
	added bool
	retry int
}

func TestLeakyBucketAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
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
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	verifyAttempts(t, 1, time.Minute, 2, []attempt{
		{+0, false, 0},  // not allowed and no retry possible (increment > capacity)
		{+61, false, 0}, // even after a minute
	})
}

func TestLeakyBucketAddAtSlowRate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	verifyAttempts(t, 1, time.Second/2, 1, []attempt{
		{+0, true, 0},
		{+1, true, 0},
		{+2, true, 0},
	})
}

func verifyAttempts(t *testing.T, capacity int, emission time.Duration, increment int, attempts []attempt) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	redisClient := net.NewRedisClient(
		&net.RedisOptions{
			Addrs: []string{redisAddr},
		},
	)
	defer redisClient.Close()

	now := time.Now()
	bucket := newClusterLeakyBucket(redisClient, capacity, emission, func() time.Time { return now })

	t0 := now
	for _, a := range attempts {
		now = t0.Add(time.Duration(a.tplus) * time.Second)
		added, retry, err := bucket.add(context.Background(), "alabel", increment, now)
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
}

func TestLeakyBucketRedisError(t *testing.T) {
	redisClient := net.NewRedisClient(
		&net.RedisOptions{
			Addrs: []string{"no-such-host.test:123"},
		},
	)
	defer redisClient.Close()

	bucket := newClusterLeakyBucket(redisClient, 1, time.Minute, time.Now)
	_, _, err := bucket.add(context.Background(), "alabel", 1, time.Now())

	assert.Error(t, err)
}

func TestLeakyBucketId(t *testing.T) {
	const label = "alabel"

	b1 := newClusterLeakyBucket(nil, 1, time.Minute, time.Now)
	b2 := newClusterLeakyBucket(nil, 2, time.Minute, time.Now)

	assert.NotEqual(t, b1.getBucketId(label), b2.getBucketId(label))
}

func TestLeakyBucketRedisStoredNumberPrecision(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode")
	}
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	redisClient := net.NewRedisClient(
		&net.RedisOptions{
			Addrs: []string{redisAddr},
		},
	)
	defer redisClient.Close()

	const (
		capacity  = 10
		emission  = time.Second
		label     = "alabel"
		increment = 2
	)

	now := time.Now()
	b := newClusterLeakyBucket(redisClient, capacity, emission, func() time.Time { return now })

	_, _, err := b.add(context.Background(), label, increment, now)
	require.NoError(t, err)

	v, err := redisClient.Get(context.Background(), b.getBucketId(label))
	require.NoError(t, err)

	expected := now.UnixMicro() + (increment * emission).Microseconds()

	assert.Equal(t, fmt.Sprintf("%d", expected), v)
}
