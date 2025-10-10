package metrics

import "testing"

// Compile-time check that All implements PrometheusMetrics
func TestAllImplementsPrometheusMetrics(t *testing.T) {
	var _ PrometheusMetrics = (*All)(nil)
}
