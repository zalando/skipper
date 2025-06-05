package proxy

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testClock struct {
	mu sync.Mutex
	time.Time
}

func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Time = c.Time.Add(d)
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Time
}

func TestStopWatch(t *testing.T) {
	start, err := time.Parse(time.RFC3339, "2022-11-10T00:36:41+01:00")
	require.NoError(t, err)

	clock := testClock{Time: start}
	watch := NewStopWatch(clock.now)

	watch.Start()
	clock.add(5 * time.Millisecond)
	watch.Stop()

	elapsed := watch.Elapsed()
	if elapsed != 5*time.Millisecond {
		t.Errorf("Expected elapsed time to be 5ms, got %v", elapsed)
	}

	watch.Reset()
	elapsedAfterReset := watch.Elapsed()
	if elapsedAfterReset != 0 {
		t.Errorf("Expected elapsed time to be 0 after reset, got %v", elapsed)
	}

	watch.Start()
	clock.add(2 * time.Millisecond)
	watch.Stop()
	elapsed = watch.Elapsed()
	if elapsed != 2*time.Millisecond {
		t.Errorf("Expected elapsed time to be 2ms, got %v", elapsed)
	}
}
