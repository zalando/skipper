package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	out := string(body)

	assert.Contains(t, out, `skipper_cache_route_result_total{result="hit",route="product"} 1`)
	assert.Contains(t, out, `skipper_cache_route_result_total{result="miss",route="product"} 1`)
}

func TestPrometheusCacheFirstHitSeconds(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	const path = "/cache-first-hit"
	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	pm.IncCacheRouteResult("product", "miss")
	pm.IncCacheRouteResult("product", "hit")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Regexp(t, `skipper_cache_first_hit_seconds [0-9.e+-]+\n`, string(body))
}
