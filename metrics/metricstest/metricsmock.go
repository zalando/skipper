package metricstest

import (
	"github.com/zalando/skipper/metrics"
	"net/http"
	"time"
)

type MockMetrics struct {
	strictMock bool

	// Functions external implementation
	MeasureSinceFn func(key string, start time.Time)
	IncCounterFn   func(key string)
	IncCounterByFn func(key string, value int64)

	// Metrics gathering
	counters map[string]int64
	measures map[string][]int64
}

var _ metrics.Metrics = new(MockMetrics)

func (m *MockMetrics) MeasureSince(key string, start time.Time) {
	if m.MeasureSinceFn == nil {
		if m.strictMock {
			panic("mock me")
		}
	} else {
		m.MeasureSinceFn(key, start)
	}


}

func (m *MockMetrics) IncCounter(key string) {
	if m.IncCounterFn == nil {
		if m.strictMock {
			panic("mock me")
		}
	} else {
		m.IncCounterFn(key)
	}

	counter, ok := m.counters[key]
	if !ok {
		counter = 0
	}
	m.counters[key] = counter + 1
}

func (m *MockMetrics) IncCounterBy(key string, value int64) {
	if m.IncCounterByFn == nil {
		if m.strictMock {
			panic("mock me")
		}
	} else {
		m.IncCounterByFn(key, value)
	}

	counter, ok := m.counters[key]
	if !ok {
		counter = 0
	}
	m.counters[key] = counter + value
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
