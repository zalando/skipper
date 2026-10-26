// Package metricstest provides test infrastructure to test packages
// that depend on metrics package.
package metricstest

import (
	"fmt"
	"net/http"
	"strconv"
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

func (m *MockMetrics) MeasureBackendZone(zone string, start time.Time) {
	if zone != "" {
		key := fmt.Sprintf("%sbackendzone.%s", m.Prefix, zone)
		m.WithMeasures(func(measures map[string][]time.Duration) {
			measures[key] = append(measures[key], m.Now.Sub(start))
		})
	}
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
	d := time.Since(start)
	m.WithMeasures(func(measures map[string][]time.Duration) {
		measures[routeID] = append(measures[routeID], d)
		measures[host] = append(measures[host], d)
		measures[method] = append(measures[method], d)
		measures[strconv.Itoa(code)] = append(measures[strconv.Itoa(code)], d)
	})

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

func (m *MockMetrics) Counter(key string) (v int64, ok bool) {
	m.WithCounters(func(counters map[string]int64) {
		v, ok = counters[key]
	})
	return
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

// IncCacheResult records a labeled cache outcome (result x code_class),
// mirroring the real Prometheus skipper_cache_result_total{result,code_class}
// counter. Use CacheResultCount to read it back in tests.
func (m *MockMetrics) IncCacheResult(result, codeClass string) {
	m.IncCounter(cacheResultKey(result, codeClass))
}

// CacheResultCount returns the labeled cache-result counter recorded via
// IncCacheResult, if any.
func (m *MockMetrics) CacheResultCount(result, codeClass string) (v int64, ok bool) {
	return m.Counter(m.Prefix + cacheResultKey(result, codeClass))
}

func cacheResultKey(result, codeClass string) string {
	return fmt.Sprintf("cache_result.%s.%s", result, codeClass)
}

// IncStorageError records a labeled cache storage error (backend x op), mirroring the
// real Prometheus skipper_cache_storage_error_total{backend,op} counter. Also increments
// the legacy unlabeled cache.storage_error counter and the labeled
// cache_result.storage_error.n/a counter, matching the real Prometheus implementation.
// Use StorageErrorCount to read the labeled one back in tests.
func (m *MockMetrics) IncStorageError(backend, op string) {
	m.IncCounter(storageErrorKey(backend, op))
	m.IncCounter("cache.storage_error")
	m.IncCacheResult("storage_error", "n/a")
}

// StorageErrorCount returns the labeled cache-storage-error counter recorded via
// IncStorageError, if any.
func (m *MockMetrics) StorageErrorCount(backend, op string) (v int64, ok bool) {
	return m.Counter(m.Prefix + storageErrorKey(backend, op))
}

func storageErrorKey(backend, op string) string {
	return fmt.Sprintf("storage_error.%s.%s", backend, op)
}

// SetCacheLRUBytes records the per-route LRU bytes gauge, mirroring the real
// Prometheus skipper_cache_lru_bytes{route} gauge. Use CacheLRUBytes to read
// it back in tests.
func (m *MockMetrics) SetCacheLRUBytes(route string, bytes float64) {
	m.UpdateGauge(cacheLRUBytesKey(route), bytes)
}

// CacheLRUBytes returns the per-route LRU bytes gauge recorded via
// SetCacheLRUBytes, if any.
func (m *MockMetrics) CacheLRUBytes(route string) (v float64, ok bool) {
	return m.Gauge(cacheLRUBytesKey(route))
}

func cacheLRUBytesKey(route string) string {
	return fmt.Sprintf("cache_lru_bytes.%s", route)
}

// IncCacheRouteResult mirrors skipper_cache_route_result_total{route,result}.
func (m *MockMetrics) IncCacheRouteResult(route, result string) {
	m.IncCounter(fmt.Sprintf("cache_route_result.%s.%s", route, result))
}

// IncCacheRouteReval mirrors skipper_cache_route_reval_total{route,outcome}.
func (m *MockMetrics) IncCacheRouteReval(route, outcome string) {
	m.IncCounter(fmt.Sprintf("cache_route_reval.%s.%s", route, outcome))
}

// ObserveCacheOriginDuration mirrors skipper_cache_origin_duration_seconds{route,status_class,dispatch}
// by recording the observation count under a per-route/status-class/dispatch counter.
func (m *MockMetrics) ObserveCacheOriginDuration(route, statusClass, dispatch string, seconds float64) {
	m.IncCounter(fmt.Sprintf("cache_origin_duration.%s.%s.%s", route, statusClass, dispatch))
}

// IncOriginCacheControl mirrors skipper_cache_origin_cache_control_total{cache_control}.
func (m *MockMetrics) IncOriginCacheControl(directive string) {
	m.IncCounter(fmt.Sprintf("cache_origin_cache_control.%s", directive))
}

// SetCacheLRUEntries mirrors skipper_cache_lru_entries{route}.
func (m *MockMetrics) SetCacheLRUEntries(route string, entries float64) {
	m.UpdateGauge(fmt.Sprintf("cache_lru_entries.%s", route), entries)
}

// SetCacheLRUCapacity mirrors skipper_cache_lru_capacity_bytes.
func (m *MockMetrics) SetCacheLRUCapacity(bytes float64) {
	m.UpdateGauge("cache_lru_capacity_bytes", bytes)
}

func (*MockMetrics) Close() {}

func (*MockMetrics) String() string { return "mock" }
