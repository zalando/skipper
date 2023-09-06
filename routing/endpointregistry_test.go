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

	assert.Equal(t, time.Time{}, m.DetectedTime())
	assert.Equal(t, int64(0), m.InflightRequests())
}

func TestSetAndGet(t *testing.T) {
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

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
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

	r.IncInflightRequest("some key")
	mToChange := r.GetMetrics("some key")
	mConst := r.GetMetrics("another key")

	assert.Equal(t, int64(0), mConst.InflightRequests())
	assert.Equal(t, int64(1), mToChange.InflightRequests())
}

func TestDoRemovesOldEntries(t *testing.T) {
	beginTestTs := time.Now()
	r := routing.NewEndpointRegistry(routing.RegistryOptions{})

	routing.SetNow(r, func() time.Time {
		return beginTestTs
	})
	route := &routing.Route{
		LBEndpoints: []routing.LBEndpoint{
			{Host: "endpoint1.test:80"},
			{Host: "endpoint2.test:80"},
		},
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
		},
	}
	r.Do([]*routing.Route{route})

	mExist := r.GetMetrics("endpoint1.test:80")
	mExistYet := r.GetMetrics("endpoint2.test:80")
	assert.Equal(t, beginTestTs, mExist.DetectedTime())
	assert.Equal(t, beginTestTs, mExistYet.DetectedTime())

	r.IncInflightRequest("endpoint1.test:80")
	r.IncInflightRequest("endpoint2.test:80")

	routing.SetNow(r, func() time.Time {
		return beginTestTs.Add(routing.ExportLastSeenTimeout + time.Second)
	})
	route = &routing.Route{
		LBEndpoints: []routing.LBEndpoint{
			{Host: "endpoint1.test:80"},
		},
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
		},
	}
	route.BackendType = eskip.LBBackend
	r.Do([]*routing.Route{route})

	mExist = r.GetMetrics("endpoint1.test:80")
	mRemoved := r.GetMetrics("endpoint2.test:80")

	assert.Equal(t, beginTestTs, mExist.DetectedTime())
	assert.Equal(t, int64(1), mExist.InflightRequests())

	assert.Equal(t, time.Time{}, mRemoved.DetectedTime())
	assert.Equal(t, int64(0), mRemoved.InflightRequests())
}

// type createTestItem struct {
// 	name   string
// 	args   []interface{}
// 	expect interface{}
// 	fail   bool
// }

// func (test createTestItem) run(
// 	t *testing.T,
// 	init func() filters.Spec,
// 	box func(filters.Filter) interface{},
// ) {
// 	f, err := init().CreateFilter(test.args)
// 	if test.fail {
// 		if err == nil {
// 			t.Fatal("Failed to fail.")
// 		}

// 		return
// 	}

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	if box(f) != test.expect {
// 		t.Fatalf("Unexpected value, expected: %v, got: %v.", test.expect, box(f))
// 	}
// }

// func TestCreateFadeIn(t *testing.T) {
// 	for _, test := range []createTestItem{{
// 		name: "no args",
// 		fail: true,
// 	}, {
// 		name: "too many args",
// 		args: []interface{}{1, 2, 3},
// 		fail: true,
// 	}, {
// 		name: "wrong duration string",
// 		args: []interface{}{"foo"},
// 		fail: true,
// 	}, {
// 		name: "wrong exponent type",
// 		args: []interface{}{"3m", "foo"},
// 		fail: true,
// 	}, {
// 		name:   "duration as int",
// 		args:   []interface{}{1000},
// 		expect: routing.ExportFadeIn{duration: time.Second, exponent: 1},
// 	}, {
// 		name:   "duration as float",
// 		args:   []interface{}{float64(1000)},
// 		expect: routing.ExportFadeIn{duration: time.Second, exponent: 1},
// 	}, {
// 		name:   "duration as string",
// 		args:   []interface{}{"1s"},
// 		expect: routing.ExportFadeIn{duration: time.Second, exponent: 1},
// 	}, {
// 		name:   "duration as time.Duration",
// 		args:   []interface{}{time.Second},
// 		expect: fadeIn{duration: time.Second, exponent: 1},
// 	}, {
// 		name:   "exponent as int",
// 		args:   []interface{}{"3m", 2},
// 		expect: fadeIn{duration: 3 * time.Minute, exponent: 2},
// 	}, {
// 		name:   "exponent as float",
// 		args:   []interface{}{"3m", 2.0},
// 		expect: fadeIn{duration: 3 * time.Minute, exponent: 2},
// 	}} {
// 		t.Run(test.name, func(t *testing.T) {
// 			test.run(
// 				t,
// 				routing.NewFadeIn,
// 				func(f filters.Filter) interface{} { return f.(routing.ExportFadeIn) },
// 			)
// 		})
// 	}
// }

// func TestCreateEndpointCreated(t *testing.T) {
// 	now := time.Now()

// 	nows := func() string {
// 		b, err := now.MarshalText()
// 		if err != nil {
// 			t.Fatal(err)
// 		}

// 		return string(b)
// 	}

// 	// ensure same precision:
// 	now, err := time.Parse(time.RFC3339, nows())
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	for _, test := range []createTestItem{{
// 		name: "no args",
// 		fail: true,
// 	}, {
// 		name: "few args",
// 		args: []interface{}{"http://10.0.0.1:8080"},
// 		fail: true,
// 	}, {
// 		name: "too many args",
// 		args: []interface{}{"http://10.0.0.1:8080", now, "foo"},
// 		fail: true,
// 	}, {
// 		name: "address not string",
// 		args: []interface{}{42, now},
// 		fail: true,
// 	}, {
// 		name: "address not url",
// 		args: []interface{}{string(rune(' ' - 1)), now},
// 		fail: true,
// 	}, {
// 		name: "invalid host",
// 		args: []interface{}{"http://::1", now},
// 		fail: true,
// 	}, {
// 		name: "invalid time string",
// 		args: []interface{}{"http://10.0.0.1:8080", "foo"},
// 		fail: true,
// 	}, {
// 		name: "invalid time type",
// 		args: []interface{}{"http://10.0.0.1:8080", struct{}{}},
// 		fail: true,
// 	}, {
// 		name:   "future time",
// 		args:   []interface{}{"http://10.0.0.1:8080", now.Add(time.Hour)},
// 		expect: endpointCreated{which: "10.0.0.1:8080", when: time.Time{}},
// 	}, {
// 		name:   "auto 80",
// 		args:   []interface{}{"http://10.0.0.1", now},
// 		expect: endpointCreated{which: "10.0.0.1:80", when: now},
// 	}, {
// 		name:   "auto 443",
// 		args:   []interface{}{"https://10.0.0.1", now},
// 		expect: endpointCreated{which: "10.0.0.1:443", when: now},
// 	}, {
// 		name:   "time as int",
// 		args:   []interface{}{"http://10.0.0.1:8080", 42},
// 		expect: endpointCreated{which: "10.0.0.1:8080", when: time.Unix(42, 0)},
// 	}, {
// 		name:   "time as float",
// 		args:   []interface{}{"http://10.0.0.1:8080", 42.0},
// 		expect: endpointCreated{which: "10.0.0.1:8080", when: time.Unix(42, 0)},
// 	}, {
// 		name:   "time as string",
// 		args:   []interface{}{"http://10.0.0.1:8080", nows()},
// 		expect: endpointCreated{which: "10.0.0.1:8080", when: now},
// 	}, {
// 		name:   "time as time.Time",
// 		args:   []interface{}{"http://10.0.0.1:8080", now},
// 		expect: endpointCreated{which: "10.0.0.1:8080", when: now},
// 	}} {
// 		t.Run(test.name, func(t *testing.T) {
// 			test.run(
// 				t,
// 				NewEndpointCreated,
// 				func(f filters.Filter) interface{} { return f.(endpointCreated) },
// 			)
// 		})
// 	}
// }

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
		r := routing.NewEndpointRegistry(routing.RegistryOptions{})
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
