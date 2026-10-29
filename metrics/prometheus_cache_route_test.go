package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
)

func TestPrometheusCacheRouteResult(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	const path = "/cache-route-result"
	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	pm.IncCacheRouteResult("product", "hit")
	pm.IncCacheRouteResult("product", "miss")
	pm.IncCacheRouteResult("product", "stale")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	out := string(body)

	assert.Contains(t, out, `skipper_cache_route_result_total{result="hit",route="product"} 1`)
	assert.Contains(t, out, `skipper_cache_route_result_total{result="miss",route="product"} 1`)
	assert.Contains(t, out, `skipper_cache_route_result_total{result="stale",route="product"} 1`)
}

func TestPrometheusCacheFirstHitSeconds(t *testing.T) {
	reg := prometheus.NewRegistry()
	start := time.Now()
	pm := metrics.NewPrometheus(metrics.Options{PrometheusRegistry: reg})
	initialised := time.Now()
	firstHit := func() (float64, bool) {
		t.Helper()
		families, err := reg.Gather()
		require.NoError(t, err)
		for _, family := range families {
			if family.GetName() == "skipper_cache_first_hit_seconds" {
				require.Len(t, family.Metric, 1)
				return family.Metric[0].GetGauge().GetValue(), true
			}
		}
		return 0, false
	}

	_, exists := firstHit()
	require.False(t, exists, "no cache hit must not be reported as a zero-second hit")
	pm.IncCacheRouteResult("product", "miss")
	pm.IncCacheRouteResult("product", "stale")
	_, exists = firstHit()
	require.False(t, exists, "misses and stale responses do not establish the first hit")

	beforeHit := time.Now()
	pm.IncCacheRouteResult("product", "hit")
	elapsed := time.Since(start).Seconds()
	value, exists := firstHit()
	require.True(t, exists)
	assert.GreaterOrEqual(t, value, beforeHit.Sub(initialised).Seconds())
	assert.LessOrEqual(t, value, elapsed)

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pm.IncCacheRouteResult("other", "hit")
		}()
	}
	wg.Wait()
	later, exists := firstHit()
	require.True(t, exists)
	assert.Equal(t, value, later, "later hits must not reset the first-hit duration")

	otherReg := prometheus.NewRegistry()
	metrics.NewPrometheus(metrics.Options{PrometheusRegistry: otherReg})
	families, err := otherReg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		assert.NotEqual(t, "skipper_cache_first_hit_seconds", family.GetName())
	}
}

func BenchmarkPrometheusCacheRouteResult(b *testing.B) {
	pm := metrics.NewPrometheus(metrics.Options{})
	pm.IncCacheRouteResult("product", "hit")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		pm.IncCacheRouteResult("product", "hit")
	}
}

