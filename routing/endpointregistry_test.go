package routing

import (
	"fmt"
	"runtime/metrics"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEmptyRegistry(t *testing.T) {
	r := NewEndpointRegistry(RegistryOptions{})
	m := r.GetMetrics("some key")

	assert.Equal(t, time.Time{}, m.DetectedTime())
	assert.Equal(t, int64(0), m.InflightRequests())
}

func TestSetAndGet(t *testing.T) {
	r := NewEndpointRegistry(RegistryOptions{})

	mBefore := r.GetMetrics("some key")
	r.IncInflightRequest("some key")
	mAfter := r.GetMetrics("some key")

	assert.Equal(t, int64(0), mBefore.InflightRequests())
	assert.Equal(t, int64(1), mAfter.InflightRequests())

	ts, _ := time.Parse(time.DateOnly, "2023-08-29")
	mBefore = r.GetMetrics("some key")
	r.SetDetectedTime("some key", ts)
	mAfter = r.GetMetrics("some key")

	assert.Equal(t, time.Time{}, mBefore.DetectedTime())
	assert.Equal(t, ts, mAfter.DetectedTime())
}

func TestSetAndGetAnotherKey(t *testing.T) {
	r := NewEndpointRegistry(RegistryOptions{})

	r.IncInflightRequest("some key")
	mToChange := r.GetMetrics("some key")
	mConst := r.GetMetrics("another key")

	assert.Equal(t, int64(0), mConst.InflightRequests())
	assert.Equal(t, int64(1), mToChange.InflightRequests())
}

func printTotalMutexWaitTime(b *testing.B) {
	// Name of the metric we want to read.
	const myMetric = "/sync/mutex/wait/total:seconds"

	// Create a sample for the metric.
	sample := make([]metrics.Sample, 1)
	sample[0].Name = myMetric

	// Sample the metric.
	metrics.Read(sample)

	// Check if the metric is actually supported.
	// If it's not, the resulting value will always have
	// kind KindBad.
	if sample[0].Value.Kind() == metrics.KindBad {
		panic(fmt.Sprintf("metric %q no longer supported", myMetric))
	}

	// Handle the result.
	//
	// It's OK to assume a particular Kind for a metric;
	// they're guaranteed not to change.
	mutexWaitTime := sample[0].Value.Float64()
	b.Logf("total mutex waiting time since the beginning of benchmark: %f\n", mutexWaitTime)
}

func benchmarkIncInflightRequests(b *testing.B, name string, goroutines int) {
	const key string = "some key"
	const mapSize int = 10000

	b.Run(name, func(b *testing.B) {
		r := NewEndpointRegistry(RegistryOptions{})
		for i := 1; i < mapSize; i++ {
			r.IncInflightRequest(fmt.Sprintf("foo-%d", i))
		}
		r.IncInflightRequest(key)
		r.IncInflightRequest(key)

		wg := sync.WaitGroup{}
		b.ResetTimer()
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < b.N/goroutines; n++ {
					r.IncInflightRequest(key)
				}
			}()
		}
		wg.Wait()

		printTotalMutexWaitTime(b)
	})
}

func BenchmarkIncInflightRequests(b *testing.B) {
	goroutinesNums := []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64, 128, 256, 512, 768, 1024, 1536, 2048, 4096}
	for _, goroutines := range goroutinesNums {
		benchmarkIncInflightRequests(b, fmt.Sprintf("%d goroutines", goroutines), goroutines)
	}
}

func benchmarkGetInflightRequests(b *testing.B, name string, goroutines int) {
	const key string = "some key"
	const mapSize int = 10000

	b.Run(name, func(b *testing.B) {
		r := NewEndpointRegistry(RegistryOptions{})
		for i := 1; i < mapSize; i++ {
			r.IncInflightRequest(fmt.Sprintf("foo-%d", i))
		}
		r.IncInflightRequest(key)
		r.IncInflightRequest(key)

		var dummy int64
		wg := sync.WaitGroup{}
		b.ResetTimer()
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < b.N/goroutines; n++ {
					dummy = r.GetMetrics(key).InflightRequests()
				}
			}()
		}
		dummy++
		wg.Wait()

		printTotalMutexWaitTime(b)
	})
}

func BenchmarkGetInflightRequests(b *testing.B) {
	goroutinesNums := []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64, 128, 256, 512, 768, 1024, 1536, 2048, 4096}
	for _, goroutines := range goroutinesNums {
		benchmarkGetInflightRequests(b, fmt.Sprintf("%d goroutines", goroutines), goroutines)
	}
}
