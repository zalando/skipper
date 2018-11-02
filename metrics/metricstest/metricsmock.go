package metricstest

import (
	"net/http"
	"time"
)

type MockMetrics struct {
	Prefix string

	// Metrics gathering
	Counters map[string]int64
	Measures map[string][]time.Duration
}

func (m *MockMetrics) MeasureSince(key string, start time.Time) {
	key = m.Prefix + key
	if m.Measures == nil {
		m.Measures = make(map[string][]time.Duration)
	}
	duration := time.Since(start)
	measure, ok := m.Measures[key]
	if !ok {
		measure = make([]time.Duration, 1)
	}
	m.Measures[key] = append(measure, duration)
}

func (m *MockMetrics) IncCounter(key string) {
	key = m.Prefix + key
	if m.Counters == nil {
		m.Counters = make(map[string]int64)
	}
	counter, ok := m.Counters[key]
	if !ok {
		counter = 0
	}
	m.Counters[key] = counter + 1
}

func (m *MockMetrics) IncCounterBy(key string, value int64) {
	key = m.Prefix + key
	if m.Counters == nil {
		m.Counters = make(map[string]int64)
	}
	counter, ok := m.Counters[key]
	if !ok {
		counter = 0
	}
	m.Counters[key] = counter + value
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
