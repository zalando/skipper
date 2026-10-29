package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
)

func TestPrometheusCacheUnobservedSeries(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	rec := httptest.NewRecorder()
	pm.CreateHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	for _, name := range []string{
		"skipper_cache_route_result_total",
		"skipper_cache_route_reval_total",
		"skipper_cache_origin_duration_seconds",
		"skipper_cache_origin_cache_control_total",
		"skipper_cache_lru_entries",
		"skipper_cache_first_hit_seconds",
	} {
		assert.NotContains(t, rec.Body.String(), name, "unobserved series must be absent")
	}
}

func TestPrometheusCacheOriginNativeHistogram(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "classic"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			pm := metrics.NewPrometheus(metrics.Options{
				PrometheusRegistry:               reg,
				EnablePrometheusNativeHistograms: native,
			})
			pm.ObserveCacheOriginDuration("product", "2xx", "direct", 0.125)
			families, err := reg.Gather()
			require.NoError(t, err)
			for _, family := range families {
				if family.GetName() != "skipper_cache_origin_duration_seconds" {
					continue
				}
				require.Len(t, family.Metric, 1)
				histogram := family.Metric[0].GetHistogram()
				require.NotNil(t, histogram)
				assert.NotEmpty(t, histogram.GetBucket())
				assert.Equal(t, uint64(1), histogram.GetSampleCount())
				assert.Equal(t, 0.125, histogram.GetSampleSum())
				assert.Equal(t, native, histogram.Schema != nil)
				return
			}
			t.Fatal("origin duration histogram is missing")
		})
	}
}

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
	pm.ObserveCacheOriginDuration("product", "2xx", "direct", 0.125)
	pm.ObserveCacheOriginDuration("product", "2xx", "direct", 0.375)
	pm.ObserveCacheOriginDuration("other", "error", "pipeline", 0.5)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	out := rec.Body.String()
	for _, sample := range []string{
		"# TYPE skipper_cache_origin_duration_seconds histogram",
		`skipper_cache_origin_duration_seconds_count{dispatch="direct",route="product",status_class="2xx"} 2`,
		`skipper_cache_origin_duration_seconds_sum{dispatch="direct",route="product",status_class="2xx"} 0.5`,
		`skipper_cache_origin_duration_seconds_bucket{dispatch="direct",route="product",status_class="2xx",le="0.25"} 1`,
		`skipper_cache_origin_duration_seconds_bucket{dispatch="direct",route="product",status_class="2xx",le="0.5"} 2`,
		`skipper_cache_origin_duration_seconds_bucket{dispatch="direct",route="product",status_class="2xx",le="+Inf"} 2`,
		`skipper_cache_origin_duration_seconds_count{dispatch="pipeline",route="other",status_class="error"} 1`,
		`skipper_cache_origin_duration_seconds_sum{dispatch="pipeline",route="other",status_class="error"} 0.5`,
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
