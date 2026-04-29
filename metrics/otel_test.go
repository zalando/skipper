package metrics

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Compile-time check that OTel implements Metrics.
var _ Metrics = (*OTel)(nil)

// failOnNthMeter wraps noop.Meter and returns an error on the Nth instrument call.
type failOnNthMeter struct {
	noop.Meter
	failAt  int
	callNum int
}

func (m *failOnNthMeter) next() error {
	m.callNum++
	if m.callNum == m.failAt {
		return errors.New("injected meter error")
	}
	return nil
}

func (m *failOnNthMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if err := m.next(); err != nil {
		return nil, err
	}
	return m.Meter.Float64Histogram(name, opts...)
}

func (m *failOnNthMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if err := m.next(); err != nil {
		return nil, err
	}
	return m.Meter.Int64Counter(name, opts...)
}

func (m *failOnNthMeter) Float64Counter(name string, opts ...metric.Float64CounterOption) (metric.Float64Counter, error) {
	if err := m.next(); err != nil {
		return nil, err
	}
	return m.Meter.Float64Counter(name, opts...)
}

func (m *failOnNthMeter) Float64Gauge(name string, opts ...metric.Float64GaugeOption) (metric.Float64Gauge, error) {
	if err := m.next(); err != nil {
		return nil, err
	}
	return m.Meter.Float64Gauge(name, opts...)
}

func newTestOTel(t *testing.T, opts Options) *OTel {
	t.Helper()
	o, err := newOTelWithReader(opts, sdkmetric.NewManualReader())
	if err != nil {
		t.Fatalf("newOTelWithReader: %v", err)
	}
	t.Cleanup(o.Close)
	return o
}

// TestOTelNewOTel exercises the public constructor (no real OTLP endpoint needed —
// the exporter is created successfully even without a configured endpoint).
func TestOTelNewOTel(t *testing.T) {
	o, err := NewOTel(Options{DisableCompatibilityDefaults: true})
	if err != nil {
		t.Fatalf("NewOTel: %v", err)
	}
	o.Close()
}

// TestOTelInstrumentErrors verifies that newOTelWithReaderAndMeter returns an error
// when each individual instrument creation fails, and that it also shuts down
// the provider (exercises the error path in newOTelWithReaderAndMeter itself).
// The instruments are created in this order inside newOTelInstruments:
//  1. Float64Histogram: routeLookupM
//  2. Int64Counter:     routeErrorsM
//  3. Float64Histogram: responseM
//  4. Float64Histogram: responseSizeM
//  5. Float64Histogram: filterCreateM
//  6. Float64Histogram: filterRequestM
//  7. Float64Histogram: filterAllRequestM
//  8. Float64Histogram: filterAllCombinedRequestM
//  9. Float64Histogram: backendRequestHeadersM
//
// 10. Float64Histogram: backendM
// 11. Float64Histogram: backendCombinedM
// 12. Float64Histogram: filterResponseM
// 13. Float64Histogram: filterAllResponseM
// 14. Float64Histogram: filterAllCombinedResponseM
// 15. Float64Histogram: serveRouteM
// 16. Int64Counter:     serveRouteCounterM
// 17. Float64Histogram: serveHostM
// 18. Int64Counter:     serveHostCounterM
// 19. Float64Histogram: proxyTotalM
// 20. Float64Histogram: proxyRequestM
// 21. Float64Histogram: proxyResponseM
// 22. Float64Histogram: backend5xxM
// 23. Int64Counter:     backendErrorsM
// 24. Int64Counter:     proxyStreamingErrorsM
// 25. Float64Histogram: customHistogramM
// 26. Float64Counter:   customCounterM
// 27. Float64Gauge:     customGaugeM
// 28. Float64Gauge:     invalidRouteM
func TestOTelInstrumentErrors(t *testing.T) {
	// 28 instruments total — test each one failing.
	for failAt := 1; failAt <= 28; failAt++ {
		t.Run("", func(t *testing.T) {
			m := &failOnNthMeter{failAt: failAt}
			// Use newOTelWithReaderAndMeter so we also cover the
			// provider.Shutdown error path in that function.
			_, err := newOTelWithReaderAndMeter(Options{DisableCompatibilityDefaults: true}, sdkmetric.NewManualReader(), m)
			if err == nil {
				t.Fatalf("failAt=%d: expected error, got nil", failAt)
			}
		})
	}
}

func TestOTelMeasureSince(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureSince("custom.key", time.Now().Add(-time.Millisecond))
}

func TestOTelIncCounter(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.IncCounter("req.total")
}

func TestOTelIncCounterBy(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.IncCounterBy("req.bytes", 42)
}

func TestOTelIncFloatCounterBy(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.IncFloatCounterBy("req.cost", 1.5)
}

func TestOTelUpdateGauge(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.UpdateGauge("pool.size", 8)
}

func TestOTelMeasureRouteLookup(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureRouteLookup(time.Now().Add(-time.Millisecond))
}

func TestOTelMeasureFilterCreate(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureFilterCreate("rateLimit", time.Now().Add(-time.Millisecond))
}

func TestOTelMeasureFilterRequest(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureFilterRequest("cors", time.Now().Add(-time.Millisecond))
}

func TestOTelMeasureAllFiltersRequest(t *testing.T) {
	for _, enableAll := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableAllFiltersMetrics: enableAll, DisableCompatibilityDefaults: true})
		o.MeasureAllFiltersRequest("route1", time.Now().Add(-time.Millisecond))
	}
}

func TestOTelMeasureBackendRequestHeader(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureBackendRequestHeader("example.com:443", 512)
}

func TestOTelMeasureBackend(t *testing.T) {
	for _, enable := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableRouteBackendMetrics: enable, DisableCompatibilityDefaults: true})
		o.MeasureBackend("route1", time.Now().Add(-time.Millisecond))
	}
}

func TestOTelMeasureBackendHost(t *testing.T) {
	for _, enable := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableBackendHostMetrics: enable, DisableCompatibilityDefaults: true})
		o.MeasureBackendHost("10.0.0.1:8080", time.Now().Add(-time.Millisecond))
	}
}

func TestOTelMeasureFilterResponse(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureFilterResponse("compress", time.Now().Add(-time.Millisecond))
}

func TestOTelMeasureAllFiltersResponse(t *testing.T) {
	for _, enableAll := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableAllFiltersMetrics: enableAll, DisableCompatibilityDefaults: true})
		o.MeasureAllFiltersResponse("route1", time.Now().Add(-time.Millisecond))
	}
}

func TestOTelMeasureResponse(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "combined", opts: Options{EnableCombinedResponseMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "route", opts: Options{EnableRouteResponseMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "both", opts: Options{EnableCombinedResponseMetrics: true, EnableRouteResponseMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "neither", opts: Options{DisableCompatibilityDefaults: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newTestOTel(t, tt.opts)
			o.MeasureResponse(200, "GET", "route1", time.Now().Add(-time.Millisecond))
		})
	}
}

func TestOTelMeasureResponseSize(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureResponseSize("example.com", 1024)
}

func TestOTelMeasureProxy(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "total_only", opts: Options{DisableCompatibilityDefaults: true}},
		{name: "with_request", opts: Options{EnableProxyRequestMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "with_response", opts: Options{EnableProxyResponseMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "all", opts: Options{EnableProxyRequestMetrics: true, EnableProxyResponseMetrics: true, DisableCompatibilityDefaults: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newTestOTel(t, tt.opts)
			o.MeasureProxy(2*time.Millisecond, 3*time.Millisecond)
		})
	}
}

func TestOTelMeasureServe(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "none", opts: Options{DisableCompatibilityDefaults: true}},
		{name: "route_hist", opts: Options{EnableServeRouteMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "host_hist", opts: Options{EnableServeHostMetrics: true, DisableCompatibilityDefaults: true}},
		{name: "route_counter", opts: Options{EnableServeRouteCounter: true, DisableCompatibilityDefaults: true}},
		{name: "host_counter", opts: Options{EnableServeHostCounter: true, DisableCompatibilityDefaults: true}},
		{name: "all_labels", opts: Options{
			EnableServeRouteMetrics:      true,
			EnableServeHostMetrics:       true,
			EnableServeRouteCounter:      true,
			EnableServeHostCounter:       true,
			EnableServeStatusCodeMetric:  true,
			EnableServeMethodMetric:      true,
			DisableCompatibilityDefaults: true,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newTestOTel(t, tt.opts)
			o.MeasureServe("route1", "example.com:443", "GET", 200, time.Now().Add(-time.Millisecond))
		})
	}
}

func TestOTelIncRoutingFailures(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.IncRoutingFailures()
}

func TestOTelIncErrorsBackend(t *testing.T) {
	for _, enable := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableRouteBackendErrorsCounters: enable, DisableCompatibilityDefaults: true})
		o.IncErrorsBackend("route1")
	}
}

func TestOTelMeasureBackend5xx(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.MeasureBackend5xx(time.Now().Add(-time.Millisecond))
}

func TestOTelIncErrorsStreaming(t *testing.T) {
	for _, enable := range []bool{false, true} {
		o := newTestOTel(t, Options{EnableRouteStreamingErrorsCounters: enable, DisableCompatibilityDefaults: true})
		o.IncErrorsStreaming("route1")
	}
}

func TestOTelSetInvalidRoute(t *testing.T) {
	o := newTestOTel(t, Options{})
	o.SetInvalidRoute("route-broken", "missing backend")
}

func TestOTelRegisterHandler(t *testing.T) {
	o := newTestOTel(t, Options{})
	mux := http.NewServeMux()
	o.RegisterHandler("/metrics", mux)
	// no handler registered — the mux pattern list stays empty
}

func TestOTelClose(t *testing.T) {
	o, err := newOTelWithReader(Options{}, sdkmetric.NewManualReader())
	if err != nil {
		t.Fatalf("newOTelWithReader: %v", err)
	}
	o.Close()
}

func TestOTelCustomBuckets(t *testing.T) {
	// Exercises the custom-bucket branches in newOTelWithReader and otelHistogramViews.
	opts := Options{
		HistogramBuckets:             []float64{0.001, 0.01, 0.1, 1},
		ResponseSizeBuckets:          []float64{512, 4096, 65536},
		RequestSizeBuckets:           []float64{1024, 8192},
		DisableCompatibilityDefaults: true,
	}
	o := newTestOTel(t, opts)
	o.MeasureRouteLookup(time.Now().Add(-time.Millisecond))
	o.MeasureResponseSize("host", 1024)
	o.MeasureBackendRequestHeader("host", 512)
}

func TestOTelPrefix(t *testing.T) {
	// Exercises the opts.Prefix branch in newOTelWithReader.
	o := newTestOTel(t, Options{Prefix: "myapp.", DisableCompatibilityDefaults: true})
	o.MeasureRouteLookup(time.Now().Add(-time.Millisecond))
}

func TestOTelHistogramViewsEmptyBuckets(t *testing.T) {
	// otelHistogramViews with empty durationBuckets skips the duration view loop
	// but still appends the two size views.
	views := otelHistogramViews("skipper", nil, DefaultResponseSizeBuckets, DefaultRequestSizeBuckets)
	if len(views) != 2 {
		t.Fatalf("expected 2 views (size only), got %d", len(views))
	}
}
