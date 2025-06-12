package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStopWatch(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2022-11-10T00:36:41+01:00")

	watch := newStopWatch()

	watch.now = func() time.Time {
		return now
	}

	watch.Start()
	now = now.Add(5 * time.Millisecond)
	watch.Stop()

	elapsed := watch.Elapsed()
	assert.Equal(t, 5*time.Millisecond, elapsed, "Expected elapsed time to be 5ms")

	watch.Reset()
	elapsedAfterReset := watch.Elapsed()
	assert.Equal(t, 0*time.Millisecond, elapsedAfterReset, "Expected elapsed time to be 0 after reset")

	watch.Start()
	now = now.Add(2 * time.Millisecond)
	watch.Stop()
	elapsed = watch.Elapsed()
	assert.Equal(t, 2*time.Millisecond, elapsed, "Expected elapsed time to be 2ms after second start and stop")
}
