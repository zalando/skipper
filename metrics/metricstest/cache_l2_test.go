package metricstest

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCacheL2Observer(t *testing.T) {
	m := &MockMetrics{Prefix: "test."}
	_, ok := m.CacheL2OperationCount("get", "response", "success")
	require.False(t, ok)
	_, ok = m.CacheL2ProcessingErrorCount("get", "response", "decode")
	require.False(t, ok)
	m.IncStorageError("l2", "get")

	const workers, iterations = 10, 100
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				m.IncCacheL2Operation("get", "response", "success")
				m.IncCacheL2ProcessingError("get", "response", "decode")
			}
		}()
	}
	wg.Wait()

	count, ok := m.CacheL2OperationCount("get", "response", "success")
	require.True(t, ok)
	require.EqualValues(t, workers*iterations, count)
	count, ok = m.CacheL2ProcessingErrorCount("get", "response", "decode")
	require.True(t, ok)
	require.EqualValues(t, workers*iterations, count)
	m.WithCounters(func(counters map[string]int64) {
		require.Equal(t, map[string]int64{
			"test.storage_error.l2.get":                          1,
			"test.cache.storage_error":                           1,
			"test.cache_result.storage_error.n/a":                1,
			"test.cache_l2_operation.get.response.success":       workers * iterations,
			"test.cache_l2_processing_error.get.response.decode": workers * iterations,
		}, counters)
	})
}
