package metricstest

import (
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

func (*MockMetrics) MeasureRouteLookup(start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureFilterRequest(filterName string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureAllFiltersRequest(routeId string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureBackend(routeId string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureBackendHost(routeBackendHost string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureFilterResponse(filterName string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureAllFiltersResponse(routeId string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureResponse(code int, method string, routeId string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureServe(routeId, host, method string, code int, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) IncRoutingFailures() {
	panic("implement me")
}

func (*MockMetrics) IncErrorsBackend(routeId string) {
	panic("implement me")
}

func (*MockMetrics) MeasureBackend5xx(t time.Time) {
	panic("implement me")
}

func (*MockMetrics) IncErrorsStreaming(routeId string) {
	panic("implement me")
}

func (*MockMetrics) RegisterHandler(path string, handler *http.ServeMux) {
	panic("implement me")
}

func (*MockMetrics) UpdateGauge(key string, value float64) {
	panic("implement me")
}
