package metricstest

import (
	"fmt"
	"net/http"
	"strings"
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
	now := m.Now
	if now.IsZero() {
		now = time.Now()
	}

	key = m.Prefix + key
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[key] = append(measures[key], now.Sub(start))
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
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[metrics.KeyRouteLookup] = append(measures[metrics.KeyRouteLookup], time.Since(start))
	})
}

func (m *MockMetrics) MeasureFilterRequest(filterName string, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[filterName] = append(measures[filterName], time.Since(start))
	})
}

func (m *MockMetrics) MeasureAllFiltersRequest(routeID string, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[routeID] = append(measures[routeID], time.Since(start))
	})
}

func (*MockMetrics) MeasureBackend(routeId string, start time.Time) {
	panic("implement me")
}

func (*MockMetrics) MeasureBackendHost(routeBackendHost string, start time.Time) {
	panic("implement me")
}

func (m *MockMetrics) MeasureFilterResponse(filterName string, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[filterName] = append(measures[filterName], time.Since(start))
	})
}

func (m *MockMetrics) MeasureAllFiltersResponse(routeID string, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[routeID] = append(measures[routeID], time.Since(start))
	})
}

func (m *MockMetrics) MeasureResponse(code int, method string, routeID string, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		duration := time.Since(start)
		key := fmt.Sprintf(metrics.KeyResponseCombined, code, method)
		measures[key] = append(measures[key], duration)
		key = fmt.Sprintf(metrics.KeyResponse, code, method, routeID)
		measures[key] = append(measures[key], duration)
	})
}

func hostForKey(h string) string {
	h = strings.Replace(h, ".", "_", -1)
	h = strings.Replace(h, ":", "__", -1)
	return h
}

func (m *MockMetrics) MeasureServe(routeID, host, method string, code int, start time.Time) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		duration := time.Since(start)
		key := fmt.Sprintf(metrics.KeyServeRoute, routeID, method, code)
		measures[key] = append(measures[key], duration)
		key = fmt.Sprintf(metrics.KeyServeHost, hostForKey(host), method, code)
		measures[key] = append(measures[key], duration)
	})
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

func (m *MockMetrics) Timer(key string) (d []time.Duration, ok bool) {
	m.WithMeasures(func(measures map[string][]time.Duration) {
		d, ok = measures[key]
	})

	return
}

func (m *MockMetrics) Measure(key string) ([]time.Duration, bool) {
	return m.Timer(key)
}
