package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zalando/skipper/metrics"
)

func TestFifo(t *testing.T) {
	waitForStatus := func(t *testing.T, fq *FifoQueue, s QueueStatus) {
		t.Helper()
		if fq == nil {
			t.Fatal("fq nil")
		}
		timeout := time.After(120 * time.Millisecond)
		for {
			time.Sleep(time.Millisecond)
			cur := fq.Status()
			if cur == s {
				return
			}

			select {
			case <-timeout:
				t.Fatalf("failed to reach status, want %v, got: %v", s, cur)
			default:
			}
		}
	}

	t.Run("queue full", func(t *testing.T) {
		reg := RegistryWith(Options{
			MetricsUpdateTimeout:   100 * time.Millisecond,
			EnableRouteFIFOMetrics: true,
			Metrics:                metrics.Default,
		})
		cfg := Config{
			MaxConcurrency: 1,
			MaxQueueSize:   2,
			Timeout:        500 * time.Millisecond,
			CloseTimeout:   1000 * time.Millisecond,
		}
		fq := reg.newFifoQueue("", cfg)
		ctx := context.Background()
		f, err := fq.Wait(ctx)
		if err != nil {
			t.Fatalf("Failed to call wait: %v", err)
		}
		f()

		go fq.Wait(ctx)
		go fq.Wait(ctx)
		go fq.Wait(ctx)
		waitForStatus(t, fq, QueueStatus{ActiveRequests: 1, QueuedRequests: 2})

		f, err = fq.Wait(ctx)
		if err != ErrQueueFull {
			t.Fatalf("Failed to get ErrQueueFull: %v", err)
		}
		if err == nil {
			f()
		}
		waitForStatus(t, fq, QueueStatus{ActiveRequests: 1, QueuedRequests: 2})
	})

	t.Run("semaphore actions and close", func(t *testing.T) {
		reg := NewRegistry()
		cfg := Config{
			MaxConcurrency: 1,
			MaxQueueSize:   2,
			Timeout:        200 * time.Millisecond,
			CloseTimeout:   100 * time.Millisecond,
		}
		fq := reg.newFifoQueue("foo", cfg)
		ctx := context.Background()
		f, err := fq.Wait(ctx)
		if err == nil {
			f()
		}
		fq.close()
		fq.close()
	})

}
