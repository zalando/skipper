package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
)

func TestPrometheusCacheRequest(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	mux := http.NewServeMux()
	pm.RegisterHandler("/metrics", mux)
	for _, tier := range []string{"l1", "l2", "none"} {
		pm.IncCacheRequest("fresh", tier)
		pm.IncCacheRequest("stale", tier)
	}
	for _, result := range []string{"origin", "coalesced", "bypass", "cache_only_miss", "error"} {
		pm.IncCacheRequest(result, "none")
	}
	pm.IncCacheRequest("fresh", "l1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	out := rec.Body.String()
	assert.Contains(t, out, "# TYPE skipper_cache_request_total counter")
	assert.Contains(t, out, `skipper_cache_request_total{result="fresh",tier="l1"} 2`)
	for _, tier := range []string{"l1", "l2", "none"} {
		assert.Contains(t, out, `skipper_cache_request_total{result="stale",tier="`+tier+`"} 1`)
	}
	for _, result := range []string{"origin", "coalesced", "bypass", "cache_only_miss", "error"} {
		assert.Contains(t, out, `skipper_cache_request_total{result="`+result+`",tier="none"} 1`)
	}
}
