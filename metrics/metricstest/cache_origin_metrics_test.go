package metricstest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheOriginMockMetrics(t *testing.T) {
	m := &MockMetrics{Prefix: "test."}
	m.IncCacheRouteReval("product", "started")
	m.IncCacheRouteReval("product", "started")
	m.IncCacheRouteReval("other", "failed")
	m.ObserveCacheOriginDuration("product", "2xx", 0.125)
	m.ObserveCacheOriginDuration("product", "2xx", 0.375)
	m.ObserveCacheOriginDuration("other", "error", 0.5)
	m.IncOriginCacheControl("max-age")
	m.IncOriginCacheControl("max-age")
	m.IncOriginCacheControl("other")
	m.WithCounters(func(got map[string]int64) {
		assert.Equal(t, map[string]int64{
			"test.cache_route_reval.product.started":  2,
			"test.cache_route_reval.other.failed":     1,
			"test.cache_origin_duration.product.2xx":  2,
			"test.cache_origin_duration.other.error":  1,
			"test.cache_origin_cache_control.max-age": 2,
			"test.cache_origin_cache_control.other":   1,
		}, got)
	})
}
