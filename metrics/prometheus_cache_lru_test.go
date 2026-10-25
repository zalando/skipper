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

func TestPrometheusCacheLRUGauges(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	const path = "/cache-lru"
	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	pm.SetCacheLRUCapacity(1024)
	pm.SetCacheLRUEntries("product__en", 3)
	pm.SetCacheLRUBytes("product__en", 512)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	out := string(body)

	assert.Contains(t, out, `skipper_cache_lru_capacity_bytes 1024`)
	assert.Contains(t, out, `skipper_cache_lru_entries{route="product__en"} 3`)
	assert.Contains(t, out, `skipper_cache_lru_bytes{route="product__en"} 512`)
}
