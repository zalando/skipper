package metrics

import "testing"

// Compile-time check that All implements PrometheusMetrics
func TestAllImplementsPrometheusMetrics(t *testing.T) {
	var _ PrometheusMetrics = (*All)(nil)
}

// Compile-time checks that the backends satisfy the Metrics interface
// (which now includes MeasureBackendZone).
func TestImplementsMetrics(t *testing.T) {
	var _ Metrics = (*All)(nil)
	var _ Metrics = (*Prometheus)(nil)
	var _ Metrics = (*CodaHale)(nil)
	var _ Metrics = (*OTel)(nil)
}
