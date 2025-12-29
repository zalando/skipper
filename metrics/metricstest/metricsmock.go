// Package metricstest provides test infrastructure to test pacakges
// that depend on metrics package.
package metricstest

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type MockMetrics struct {
	Prefix string

	mu sync.Mutex

	// Metrics gathering
	counters      map[string]int64
	floatCounters map[string]float64
	gauges        map[string]float64
	measures      map[string][]time.Duration
	values        map[string][]float64
	Now           time.Time
}

//
// Public thread safe access to metrics
//

func (m *MockMetrics) WithCounters(f func(counters map[string]int64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counters == nil {
		m.counters = make(map[string]int64)
	}
	f(m.counters)
}

func (m *MockMetrics) WithFloatCounters(f func(floatCounters map[string]float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.floatCounters == nil {
		m.floatCounters = make(map[string]float64)
	}
	f(m.floatCounters)
}

func (m *MockMetrics) WithMeasures(f func(measures map[string][]time.Duration)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.measures == nil {
		m.measures = make(map[string][]time.Duration)
	}
	f(m.measures)
}

func (m *MockMetrics) WithValues(f func(measures map[string][]float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.values == nil {
		m.values = make(map[string][]float64)
	}
	f(m.values)
}

func (m *MockMetrics) WithGauges(f func(map[string]float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gauges == nil {
		m.gauges = make(map[string]float64)
	}
	f(m.gauges)
}

//
// Interface Metrics
//

func (m *MockMetrics) MeasureSince(key string, start time.Time) {
	key = m.Prefix + key
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measure, ok := m.measures[key]
		if !ok {
			measure = make([]time.Duration, 0)
		}
		measures[key] = append(measure, m.Now.Sub(start))
	})
}

func (m *MockMetrics) IncCounter(key string) {
	key = m.Prefix + key
	m.WithCounters(func(counters map[string]int64) {
		counter, ok := counters[key]
		if !ok {
			counter = 0
		}
		counters[key] = counter + 1
	})
}

func (m *MockMetrics) IncCounterBy(key string, value int64) {
	key = m.Prefix + key
	m.WithCounters(func(counters map[string]int64) {
		counter, ok := counters[key]
		if !ok {
			counter = 0
		}
		counters[key] = counter + value
	})
}

func (m *MockMetrics) IncFloatCounterBy(key string, value float64) {
	key = m.Prefix + key
	m.WithFloatCounters(func(floatCounters map[string]float64) {
		counter, ok := floatCounters[key]
		if !ok {
			counter = 0
		}
		floatCounters[key] = counter + value
	})
}

func (m *MockMetrics) MeasureRouteLookup(start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureFilterCreate(filterName string, start time.Time) {
	key := fmt.Sprintf("%sfilter.%s.create", m.Prefix, filterName)
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[key] = append(m.measures[key], time.Since(start))
	})
}

func (m *MockMetrics) MeasureFilterRequest(filterName string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureAllFiltersRequest(routeID string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureBackendRequestHeader(host string, size int) {
	headerSizeKey := fmt.Sprintf("%sbackend.%s.request_header_bytes", m.Prefix, host)
	m.WithValues(func(values map[string][]float64) {
		values[headerSizeKey] = append(values[headerSizeKey], float64(size))
	})
}

func (m *MockMetrics) MeasureBackend(routeID string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureBackendHost(routeBackendHost string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureFilterResponse(filterName string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureAllFiltersResponse(routeID string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureResponse(code int, method string, routeID string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureResponseSize(host string, size int64) {
	responseSizeKey := fmt.Sprintf("%sresponse.%s.size_bytes", m.Prefix, host)
	m.WithValues(func(measures map[string][]float64) {
		measures[responseSizeKey] = append(measures[responseSizeKey], float64(size))
	})
}

func (m *MockMetrics) MeasureProxy(requestDuration, responseDuration time.Duration) {
	totalDuration := requestDuration + responseDuration
	totalDurationKey := fmt.Sprintf("%sproxy.total.duration", m.Prefix)
	requestDurationKey := fmt.Sprintf("%sproxy.request.duration", m.Prefix)
	responseDurationKey := fmt.Sprintf("%sproxy.response.duration", m.Prefix)

	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[totalDurationKey] = append(measures[totalDurationKey], totalDuration)
		measures[requestDurationKey] = append(measures[requestDurationKey], requestDuration)
		measures[responseDurationKey] = append(measures[responseDurationKey], responseDuration)
	})
}

func (m *MockMetrics) MeasureServe(routeID, host, method string, code int, start time.Time) {
	// implement me
}

func (m *MockMetrics) IncRoutingFailures() {
	// implement me
}

func (m *MockMetrics) IncErrorsBackend(routeID string) {
	// implement me
}

func (m *MockMetrics) MeasureBackend5xx(t time.Time) {
	// implement me
}

func (m *MockMetrics) IncErrorsStreaming(routeID string) {
	// implement me
}

func (m *MockMetrics) RegisterHandler(path string, handler *http.ServeMux) {
	// implement me
}

func (m *MockMetrics) UpdateGauge(key string, value float64) {
	m.WithGauges(func(g map[string]float64) {
		g[key] = value
	})
}

func (m *MockMetrics) Gauge(key string) (v float64, ok bool) {
	m.WithGauges(func(g map[string]float64) {
		v, ok = g[key]
	})

	return
}

func (m *MockMetrics) SetInvalidRoute(routeID, reason string) {
	key := fmt.Sprintf("route.invalid.%s..%s", routeID, reason)
	m.UpdateGauge(key, 1)
}

func (*MockMetrics) Close() {}
