package metricstest

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zalando/skipper/metrics"
)

type MockMetrics struct {
	Prefix string

	mu sync.Mutex

	// Metrics gathering
	counters      map[string]int64
	floatCounters map[string]float64
	gauges        map[string]float64
	measures      map[string][]time.Duration
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

func (m *MockMetrics) MeasureAllFiltersRequest(routeId string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureBackend(routeId string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureBackendHost(routeBackendHost string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureFilterResponse(filterName string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureAllFiltersResponse(routeId string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureResponse(code int, method string, routeId string, start time.Time) {
	// implement me
}

func (m *MockMetrics) MeasureSkipperLatency(key metrics.SkipperLatencyMetricKeys, skipperDuration time.Duration) {
	// implement me
}

func (m *MockMetrics) MeasureServe(routeId, host, method string, code int, start time.Time) {
	// implement me
}

func (m *MockMetrics) IncRoutingFailures() {
	// implement me
}

func (m *MockMetrics) IncErrorsBackend(routeId string) {
	// implement me
}

func (m *MockMetrics) MeasureBackend5xx(t time.Time) {
	// implement me
}

func (m *MockMetrics) IncErrorsStreaming(routeId string) {
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

func (m *MockMetrics) Close() {}
