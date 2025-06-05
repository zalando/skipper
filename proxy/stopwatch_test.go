package proxy

import (
	"testing"
	"time"
)

func TestStopWatch(t *testing.T) {
	watch := NewStopWatch()

	watch.Start()
	time.Sleep(5 * time.Millisecond)
	watch.Stop()

	elapsed := watch.Elapsed()
	if elapsed < 5*time.Millisecond || elapsed >= 6*time.Millisecond {
		t.Errorf("Expected elapsed time to be at least 5ms and less than 6ms, got %v", watch.Elapsed())
	}

	watch.Reset()
	elapsedAfterReset := watch.Elapsed()
	if elapsedAfterReset != 0 {
		t.Errorf("Expected elapsed time to be 0 after reset, got %v", watch.Elapsed())
	}

	watch.Start()
	time.Sleep(2 * time.Millisecond)
	watch.Stop()
	elapsed = watch.Elapsed()
	if elapsed < 2*time.Millisecond || elapsed >= 3*time.Millisecond {
		t.Errorf("Expected elapsed time to be atleast 2ms and less than 3ms, got %v", watch.Elapsed())
	}
}
