package metricstest

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

func TestMockMetrics(t *testing.T) {
	m := &MockMetrics{}

	t.Run("test-measure-since", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			key := "test-measure-since"
			start := time.Now()
			time.Sleep(2 * time.Second)
			m.MeasureSince(key, start)

			if a, ok := m.measures[key]; !ok {
				t.Fatalf("Failed to find measure %q", key)
			} else {
				if len(a) != 1 {
					t.Fatalf("Failed to have one measurement, got: %d", len(a))
				}
			}
		})
	})

	t.Run("test-inc-counter", func(t *testing.T) {
		key := "test-inc-counter"
		m.IncCounter(key)
		if i, ok := m.counters[key]; !ok {
			t.Fatalf("Failed to find measure %q", key)
		} else {
			if i != 1 {
				t.Fatalf("Failed to get the right value after inc: %d", i)
			}
		}

		m.IncCounterBy(key, 2)
		if i, ok := m.counters[key]; !ok {
			t.Fatalf("Failed to find measure %q", key)
		} else {
			if i != 3 {
				t.Fatalf("Failed to get the right value after inc: %d", i)
			}
		}
	})

	t.Run("test-inc-counter-by new key", func(t *testing.T) {
		key := "test-inc-counter-by"

		m.IncCounterBy(key, 2)
		if i, ok := m.counters[key]; !ok {
			t.Fatalf("Failed to find measure %q", key)
		} else {
			if i != 2 {
				t.Fatalf("Failed to get the right value after inc: %d", i)
			}
		}
	})

	t.Run("test-inc-float-counter-by new key", func(t *testing.T) {
		key := "test-inc-float-counter-by"

		m.IncFloatCounterBy(key, 2.1)
		if f, ok := m.floatCounters[key]; !ok {
			t.Fatalf("Failed to find measure %q", key)
		} else {
			if f != 2.1 {
				t.Fatalf("Failed to get the right value after inc: %0.2f", f)
			}
		}

		m.IncFloatCounterBy(key, 2.9)
		if f, ok := m.floatCounters[key]; !ok {
			t.Fatalf("Failed to find measure %q", key)
		} else {
			if f != 5 {
				t.Fatalf("Failed to get the right value after inc: %0.2f", f)
			}
		}
	})

	t.Run("test-measure-filter-create", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			filterName := "latency"
			key := fmt.Sprintf("filter.%s.create", filterName)

			now := time.Now()
			time.Sleep(time.Second)

			m.MeasureFilterCreate(filterName, now)

			if a, ok := m.measures[key]; !ok {
				t.Fatalf("Failed to find value %q for filter %q: %v", key, filterName, m.values)
			} else if len(a) != 1 {
				t.Fatalf("Failed to get the right number of values: %d", len(a))
			} else {
				f := a[0]
				if f != time.Second {
					t.Fatalf("Failed to get the right value after inc: %s", f)
				}
			}
		})
	})

	t.Run("test-measure-backend-request-header", func(t *testing.T) {
		host := "test.example"
		key := fmt.Sprintf("backend.%s.request_header_bytes", host)

		m.MeasureBackendRequestHeader(host, 2)

		if a, ok := m.values[key]; !ok {
			t.Fatalf("Failed to find value %q for host %q", key, host)
		} else if len(a) != 1 {
			t.Fatalf("Failed to get the right number of values: %d", len(a))
		} else {
			f := a[0]
			if f != 2 {
				t.Fatalf("Failed to get the right value after inc: %0.2f", f)
			}
		}
	})

	t.Run("test-measure-response-size", func(t *testing.T) {
		host := "response.test"
		key := fmt.Sprintf("response.%s.size_bytes", host)

		m.MeasureResponseSize(host, 1024)

		if a, ok := m.values[key]; !ok {
			t.Fatalf("Failed to find value %q for host %q", key, host)
		} else if len(a) != 1 {
			t.Fatalf("Failed to get the right number of values: %d", len(a))
		} else {
			f := a[0]
			if f != 1024 {
				t.Fatalf("Failed to get the right value after inc: %0.2f", f)
			}
		}
	})

	t.Run("test-measure-proxy", func(t *testing.T) {
		m.MeasureProxy(10*time.Millisecond, 30*time.Millisecond)

		h := map[string]time.Duration{
			"proxy.total.duration":    40 * time.Millisecond,
			"proxy.request.duration":  10 * time.Millisecond,
			"proxy.response.duration": 30 * time.Millisecond,
		}

		for key, v := range h {
			if a, ok := m.measures[key]; !ok {
				t.Fatalf("Failed to find value %q", key)
			} else if len(a) != 1 {
				t.Fatalf("Failed to get the right number of values: %d", len(a))
			} else {
				d := a[0]
				if d != v {
					t.Fatalf("Failed to get the right value after inc: %s", d)
				}
			}
		}
	})

	t.Run("test-measure-backend-request-header", func(t *testing.T) {
		routeID := "my-route"
		reason := "foo"
		key := fmt.Sprintf("route.invalid.%s..%s", routeID, reason)

		m.SetInvalidRoute(routeID, reason)

		if f, ok := m.gauges[key]; !ok {
			t.Fatalf("Failed to find value %q for routeID %q", key, routeID)
		} else if f != 1 {
			t.Fatalf("Failed to get the right value after inc: %0.2f", f)
		}

		m.DeleteInvalidRoute(routeID)
		if f, ok := m.gauges[key]; !ok {
			t.Fatalf("Failed to find value %q for routeID %q", key, routeID)
		} else if f != 0 {
			t.Fatalf("Failed to get the right value after inc: %0.2f", f)
		}
	})

	t.Run("test-gauge", func(t *testing.T) {
		key := "my-gauge"

		m.UpdateGauge(key, 5.4)

		if f, ok := m.Gauge(key); !ok {
			t.Fatalf("Failed to find value %q", key)
		} else if f != 5.4 {
			t.Fatalf("Failed to get the right value after inc: %0.2f", f)
		}
	})

}
