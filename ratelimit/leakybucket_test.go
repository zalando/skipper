//+build redis

package ratelimit

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// TODO: refactor into utility
func newTestRedis(t *testing.T) (port string, cancel func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()

	ctx, cc := context.WithTimeout(context.Background(), 10*time.Second)

	cmd := exec.CommandContext(ctx, "redis-server", "--port", port)
	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:" + port})
	for _, err := rdb.Ping(ctx).Result(); ctx.Err() == nil && err != nil; _, err = rdb.Ping(ctx).Result() {
		t.Log("waiting for redis server")
		time.Sleep(1 * time.Millisecond)
	}
	rdb.Close()

	return port, func() { cc(); cmd.Wait() }
}

type attempt struct {
	tplus int
	allow bool
	retry int
}

func TestBucketCheck(t *testing.T) {
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

func TestBucketAddMoreThanCapacity(t *testing.T) {
	verifyAttempts(t, 1, time.Minute, 2, []attempt{
		{+0, false, 0},  // not allowed and no retry possible
		{+61, false, 0}, // even after a minute
	})
}

func TestBucketAddAtSlowRate(t *testing.T) {
	verifyAttempts(t, 1, time.Second/2, 1, []attempt{
		{+0, true, 0},
		{+1, true, 0},
		{+2, true, 0},
	})
}

func verifyAttempts(t *testing.T, capacity int, emission time.Duration, increment int, attempts []attempt) {
	port, cancel := newTestRedis(t)
	defer cancel()

	q := make(chan struct{})
	defer close(q)

	ring := newRing(&RedisOptions{Addrs: []string{"localhost:" + port}}, q)

	now := time.Now()
	bucket := newClusterLeakyBucket(ring, t.Name(), capacity, emission, func() time.Time { return now })

	t0 := now
	for _, a := range attempts {
		now = t0.Add(time.Duration(a.tplus) * time.Second)
		allow, retry, err := bucket.Check(context.Background(), "key", increment)
		if err != nil {
			t.Fatal(err)
		}
		if a.allow != allow {
			t.Errorf("error at %+d: allow mismatch, expected %v, got %v", a.tplus, a.allow, allow)
		}
		expectedRetry := time.Duration(a.retry) * time.Second
		if expectedRetry != retry {
			t.Errorf("error at %+d: retry mismatch, expected %v, got %v", a.tplus, expectedRetry, retry)
		}
	}
}

func TestBucketRedisError(t *testing.T) {
	q := make(chan struct{})
	defer close(q)

	ring := newRing(&RedisOptions{Addrs: []string{"no-such-host.test:123"}}, q)

	bucket := newClusterLeakyBucket(ring, t.Name(), 1, time.Minute, time.Now)
	_, _, err := bucket.Check(context.Background(), "key", 1)
	if err == nil {
		t.Error("error expected")
	}
	t.Log(err)
}
