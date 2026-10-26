package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
)

func TestPrometheusCacheRouteReval(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	mux := http.NewServeMux()
	pm.RegisterHandler("/metrics", mux)
	for _, outcome := range []string{"started", "success", "failed", "dropped"} {
		pm.IncCacheRouteReval("product", outcome)
	}
	pm.IncCacheRouteReval("product", "started")
	pm.IncCacheRouteReval("other", "failed")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	out := rec.Body.String()
	assert.Contains(t, out, "# TYPE skipper_cache_route_reval_total counter")
	for _, sample := range []string{
		`skipper_cache_route_reval_total{outcome="started",route="product"} 2`,
		`skipper_cache_route_reval_total{outcome="success",route="product"} 1`,
		`skipper_cache_route_reval_total{outcome="failed",route="product"} 1`,
		`skipper_cache_route_reval_total{outcome="dropped",route="product"} 1`,
		`skipper_cache_route_reval_total{outcome="failed",route="other"} 1`,
	} {
		assert.Contains(t, out, sample)
	}
}

func TestPrometheusCacheOriginDurationRouteStatus(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{HistogramBuckets: []float64{0.25, 0.5}})
	mux := http.NewServeMux()
	pm.RegisterHandler("/metrics", mux)
	pm.ObserveCacheOriginDuration("product", "2xx", 0.125)
	pm.ObserveCacheOriginDuration("product", "2xx", 0.375)
	pm.ObserveCacheOriginDuration("other", "error", 0.5)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	out := rec.Body.String()
	for _, sample := range []string{
		"# TYPE skipper_cache_origin_duration_seconds histogram",
		`skipper_cache_origin_duration_seconds_count{route="product",status_class="2xx"} 2`,
		`skipper_cache_origin_duration_seconds_sum{route="product",status_class="2xx"} 0.5`,
		`skipper_cache_origin_duration_seconds_bucket{route="product",status_class="2xx",le="0.25"} 1`,
		`skipper_cache_origin_duration_seconds_bucket{route="product",status_class="2xx",le="0.5"} 2`,
		`skipper_cache_origin_duration_seconds_bucket{route="product",status_class="2xx",le="+Inf"} 2`,
		`skipper_cache_origin_duration_seconds_count{route="other",status_class="error"} 1`,
		`skipper_cache_origin_duration_seconds_sum{route="other",status_class="error"} 0.5`,
	} {
		assert.Contains(t, out, sample)
	}
}

func TestPrometheusCacheOriginCacheControl(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	mux := http.NewServeMux()
	pm.RegisterHandler("/metrics", mux)
	labels := []string{"public", "private", "no-store", "no-cache", "max-age", "s-maxage", "stale-while-revalidate", "stale-if-error", "other"}
	for _, label := range labels {
		pm.IncOriginCacheControl(label)
	}
	pm.IncOriginCacheControl("max-age")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	out := rec.Body.String()
	assert.Contains(t, out, "# TYPE skipper_cache_origin_cache_control_total counter")
	for _, label := range labels {
		count := "1"
		if label == "max-age" {
			count = "2"
		}
		assert.Contains(t, out, `skipper_cache_origin_cache_control_total{cache_control="`+label+`"} `+count)
	}
}
