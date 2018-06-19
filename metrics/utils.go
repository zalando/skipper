package metrics

import (
	"strings"

	metrics "github.com/rcrowley/go-metrics"
)

func newUniformSample() metrics.Sample {
	return metrics.NewUniformSample(defaultUniformReservoirSize)
}

func newExpDecaySample() metrics.Sample {
	return metrics.NewExpDecaySample(defaultExpDecayReservoirSize, defaultExpDecayAlpha)
}

func createTimer(sample metrics.Sample) metrics.Timer {
	return metrics.NewCustomTimer(metrics.NewHistogram(sample), metrics.NewMeter())
}

func hostForKey(h string) string {
	h = strings.Replace(h, ".", "_", -1)
	h = strings.Replace(h, ":", "__", -1)
	return h
}

func measuredMethod(m string) string {
	switch m {
	case "OPTIONS",
		"GET",
		"HEAD",
		"POST",
		"PUT",
		"DELETE",
		"TRACE",
		"CONNECT":
		return m
	default:
		return "_unknownmethod_"
	}
}

func applyCompatibilityDefaults(o Options) Options {
	if o.DisableCompatibilityDefaults {
		return o
	}

	o.EnableAllFiltersMetrics = true
	o.EnableRouteResponseMetrics = true
	o.EnableRouteBackendErrorsCounters = true
	o.EnableRouteStreamingErrorsCounters = true
	o.EnableRouteBackendMetrics = true

	return o
}
