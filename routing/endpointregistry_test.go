package routing_test

import (
	"fmt"
	"runtime/metrics"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

func TestEmptyRegistry(t *testing.T) {
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})
	m := r.GetMetrics("some key")

	assert.Equal(t, time.Time{}, m.GetDetectedTime())
	assert.Equal(t, time.Time{}, m.GetLastSeenTime())
	assert.Equal(t, int(0), m.GetInflightRequests())
}

func TestSetAndGet(t *testing.T) {
	now := time.Now()
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

	mBefore := r.GetMetrics("some key")
	assert.Equal(t, time.Time{}, mBefore.GetDetectedTime())
	assert.Equal(t, time.Time{}, mBefore.GetLastSeenTime())
	assert.Equal(t, int(0), mBefore.GetInflightRequests())

	r.GetMetrics("some key").SetDetectedTime(now.Add(-time.Second))
	r.GetMetrics("some key").SetLastSeenTime(now)
	r.GetMetrics("some key").IncInflightRequest()
	mAfter := r.GetMetrics("some key")

	assert.Equal(t, now.Add(-time.Second), mBefore.GetDetectedTime())
	assert.Equal(t, now, mBefore.GetLastSeenTime())
	assert.Equal(t, int(1), mBefore.GetInflightRequests())

	assert.Equal(t, now.Add(-time.Second), mAfter.GetDetectedTime())
	assert.Equal(t, now, mAfter.GetLastSeenTime())
	assert.Equal(t, int(1), mAfter.GetInflightRequests())
}

func TestSetAndGetAnotherKey(t *testing.T) {
	now := time.Now()
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

	mToChange := r.GetMetrics("some key")
	mToChange.IncInflightRequest()
	mToChange.SetDetectedTime(now.Add(-time.Second))
	mToChange.SetLastSeenTime(now)
	mConst := r.GetMetrics("another key")

	assert.Equal(t, int(0), mConst.GetInflightRequests())
	assert.Equal(t, time.Time{}, mConst.GetDetectedTime())
	assert.Equal(t, time.Time{}, mConst.GetLastSeenTime())

	assert.Equal(t, int(1), mToChange.GetInflightRequests())
	assert.Equal(t, now.Add(-time.Second), mToChange.GetDetectedTime())
	assert.Equal(t, now, mToChange.GetLastSeenTime())
}

func TestDoRemovesOldEntries(t *testing.T) {
	beginTestTs := time.Now()
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

	routing.SetNow(r, func() time.Time {
		return beginTestTs
	})
	route := &routing.Route{
		LBEndpoints: []routing.LBEndpoint{
			{Host: "endpoint1.test:80", Metrics: r.GetMetrics("endpoint1.test:80")},
			{Host: "endpoint2.test:80", Metrics: r.GetMetrics("endpoint2.test:80")},
		},
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
		},
	}
	r.Do([]*routing.Route{route})

	mExist := r.GetMetrics("endpoint1.test:80")
	mExistYet := r.GetMetrics("endpoint2.test:80")
	assert.Equal(t, beginTestTs, mExist.GetDetectedTime())
	assert.Equal(t, beginTestTs, mExistYet.GetDetectedTime())

	mExist.IncInflightRequest()
	mExistYet.IncInflightRequest()
	mExistYet.DecInflightRequest()

	routing.SetNow(r, func() time.Time {
		return beginTestTs.Add(routing.ExportDefaultLastSeenTimeout + time.Second)
	})
	route = &routing.Route{
		LBEndpoints: []routing.LBEndpoint{
			{Host: "endpoint1.test:80", Metrics: r.GetMetrics("endpoint1.test:80")},
		},
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
		},
	}
	route.BackendType = eskip.LBBackend
	r.Do([]*routing.Route{route})

	mExist = r.GetMetrics("endpoint1.test:80")
	mRemoved := r.GetMetrics("endpoint2.test:80")

	assert.Equal(t, beginTestTs, mExist.GetDetectedTime())
	assert.Equal(t, int(1), mExist.GetInflightRequests())

	assert.Equal(t, time.Time{}, mRemoved.GetDetectedTime())
	assert.Equal(t, int(0), mRemoved.GetInflightRequests())
}

func TestMetricsMethodsDoNotAllocate(t *testing.T) {
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})
	metrics := r.GetMetrics("some key")
	now := time.Now()
	metrics.SetDetectedTime(now.Add(-time.Hour))
	metrics.SetLastSeenTime(now)

	allocs := testing.AllocsPerRun(100, func() {
		assert.Equal(t, int(0), metrics.GetInflightRequests())
		metrics.IncInflightRequest()
		assert.Equal(t, int(1), metrics.GetInflightRequests())
		metrics.DecInflightRequest()
		assert.Equal(t, int(0), metrics.GetInflightRequests())

		metrics.GetDetectedTime()
		metrics.GetLastSeenTime()
	})
	assert.Equal(t, now.Add(-time.Hour), metrics.GetDetectedTime())
	assert.Equal(t, now, metrics.GetLastSeenTime())

	assert.Equal(t, 0.0, allocs)
}

func TestRaceReadWrite(t *testing.T) {
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})
	duration := time.Second

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		stop := time.After(duration)
		for {
			r.GetMetrics("some key")
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	go func() {
		defer wg.Done()
		stop := time.After(duration)
		for {
			r.GetMetrics("some key").IncInflightRequest()
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	wg.Wait()
}

func TestRaceTwoWriters(t *testing.T) {
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})
	duration := time.Second

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		stop := time.After(duration)
		for {
			r.GetMetrics("some key").IncInflightRequest()
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	go func() {
		defer wg.Done()
		stop := time.After(duration)
		for {
			r.GetMetrics("some key").DecInflightRequest()
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	wg.Wait()
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
		r := routing.NewEndpointRegistry(routing.RegistryOptions{})
		now := time.Now()

		for i := 1; i < mapSize; i++ {
			r.GetMetrics(fmt.Sprintf("foo-%d", i)).IncInflightRequest()
		}
		r.GetMetrics(key).IncInflightRequest()
		r.GetMetrics(key).SetDetectedTime(now)

		wg := sync.WaitGroup{}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				metrics := r.GetMetrics(key)
				for n := 0; n < b.N/goroutines; n++ {
					metrics.IncInflightRequest()
				}
			}()
		}
		wg.Wait()

		printTotalMutexWaitTime(b)
	})
}

func BenchmarkIncInflightRequests(b *testing.B) {
	goroutinesNums := []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64, 128, 256, 512, 768, 1024, 1536, 2048, 4096, 8192, 16384, 32768}
	for _, goroutines := range goroutinesNums {
		benchmarkIncInflightRequests(b, fmt.Sprintf("%d goroutines", goroutines), goroutines)
	}
}

func benchmarkGetInflightRequests(b *testing.B, name string, goroutines int) {
	const key string = "some key"
	const mapSize int = 10000

	b.Run(name, func(b *testing.B) {
		r := routing.NewEndpointRegistry(routing.RegistryOptions{})
		now := time.Now()
		for i := 1; i < mapSize; i++ {
			r.GetMetrics(fmt.Sprintf("foo-%d", i)).IncInflightRequest()
		}
		r.GetMetrics(key).IncInflightRequest()
		r.GetMetrics(key).SetDetectedTime(now)

		var dummy int
		wg := sync.WaitGroup{}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				metrics := r.GetMetrics(key)
				for n := 0; n < b.N/goroutines; n++ {
					dummy = metrics.GetInflightRequests()
				}
			}()
		}
		dummy++
		wg.Wait()

		printTotalMutexWaitTime(b)
	})
}

func BenchmarkGetInflightRequests(b *testing.B) {
	goroutinesNums := []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64, 128, 256, 512, 768, 1024, 1536, 2048, 4096, 8192, 16384, 32768}
	for _, goroutines := range goroutinesNums {
		benchmarkGetInflightRequests(b, fmt.Sprintf("%d goroutines", goroutines), goroutines)
	}
}

func benchmarkGetDetectedTime(b *testing.B, name string, goroutines int) {
	const key string = "some key"
	const mapSize int = 10000

	b.Run(name, func(b *testing.B) {
		r := routing.NewEndpointRegistry(routing.RegistryOptions{})
		now := time.Now()
		for i := 1; i < mapSize; i++ {
			r.GetMetrics(fmt.Sprintf("foo-%d", i)).IncInflightRequest()
		}
		r.GetMetrics(key).IncInflightRequest()
		r.GetMetrics(key).SetDetectedTime(now)

		var dummy time.Time
		wg := sync.WaitGroup{}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				metrics := r.GetMetrics(key)
				for n := 0; n < b.N/goroutines; n++ {
					dummy = metrics.GetDetectedTime()
				}
			}()
		}
		dummy = dummy.Add(time.Second)
		wg.Wait()

		printTotalMutexWaitTime(b)
	})
}

func BenchmarkGetDetectedTime(b *testing.B) {
	goroutinesNums := []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64, 128, 256, 512, 768, 1024, 1536, 2048, 4096, 8192, 16384, 32768}
	for _, goroutines := range goroutinesNums {
		benchmarkGetDetectedTime(b, fmt.Sprintf("%d goroutines", goroutines), goroutines)
	}
}
