package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otlpmetrichttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// OTel implements the Metrics interface using OpenTelemetry.
// Metrics are pushed to an OTLP endpoint configured via environment variables
// (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_METRICS_ENDPOINT, etc.).
// RegisterHandler is a no-op; there is no scrape endpoint.
type OTel struct {
	routeLookupM               metric.Float64Histogram
	routeErrorsM               metric.Int64Counter
	responseM                  metric.Float64Histogram
	responseSizeM              metric.Float64Histogram
	filterCreateM              metric.Float64Histogram
	filterRequestM             metric.Float64Histogram
	filterAllRequestM          metric.Float64Histogram
	filterAllCombinedRequestM  metric.Float64Histogram
	backendRequestHeadersM     metric.Float64Histogram
	backendM                   metric.Float64Histogram
	backendCombinedM           metric.Float64Histogram
	filterResponseM            metric.Float64Histogram
	filterAllResponseM         metric.Float64Histogram
	filterAllCombinedResponseM metric.Float64Histogram
	serveRouteM                metric.Float64Histogram
	serveRouteCounterM         metric.Int64Counter
	serveHostM                 metric.Float64Histogram
	serveHostCounterM          metric.Int64Counter
	proxyTotalM                metric.Float64Histogram
	proxyRequestM              metric.Float64Histogram
	proxyResponseM             metric.Float64Histogram
	backend5xxM                metric.Float64Histogram
	backendErrorsM             metric.Int64Counter
	proxyStreamingErrorsM      metric.Int64Counter
	customHistogramM           metric.Float64Histogram
	customCounterM             metric.Float64Counter
	customGaugeM               metric.Float64Gauge
	invalidRouteM              metric.Float64Gauge

	opts     Options
	provider *sdkmetric.MeterProvider
}

// NewOTel returns a new OTel metrics backend.
// The OTLP endpoint is configured via OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_METRICS_ENDPOINT environment variables.
func NewOTel(opts Options) (*OTel, error) {
	exporter, err := otlpmetrichttp.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("metrics: OTel OTLP/HTTP exporter: %w", err)
	}
	return newOTelWithReader(opts, sdkmetric.NewPeriodicReader(exporter))
}

func newOTelWithReader(opts Options, reader sdkmetric.Reader) (*OTel, error) {
	return newOTelWithReaderAndMeter(opts, reader, nil)
}

// newOTelWithReaderAndMeter is the internal constructor. When meterOverride is nil,
// the meter is obtained from the created MeterProvider. Tests may pass a failing
// meter to exercise error paths.
func newOTelWithReaderAndMeter(opts Options, reader sdkmetric.Reader, meterOverride metric.Meter) (*OTel, error) {
	opts = applyCompatibilityDefaults(opts)

	namespace := promNamespace
	if opts.Prefix != "" {
		namespace = strings.TrimSuffix(opts.Prefix, ".")
	}

	histogramBuckets := opts.HistogramBuckets

	responseSizeBuckets := DefaultResponseSizeBuckets
	if len(opts.ResponseSizeBuckets) > 1 {
		responseSizeBuckets = opts.ResponseSizeBuckets
	}

	requestSizeBuckets := DefaultRequestSizeBuckets
	if len(opts.RequestSizeBuckets) > 1 {
		requestSizeBuckets = opts.RequestSizeBuckets
	}

	views := otelHistogramViews(namespace, histogramBuckets, responseSizeBuckets, requestSizeBuckets)

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(views...),
	)

	m := meterOverride
	if m == nil {
		m = provider.Meter(namespace)
	}

	o, err := newOTelInstruments(opts, namespace, provider, m)
	if err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, err
	}
	return o, nil
}

func newOTelInstruments(opts Options, namespace string, provider *sdkmetric.MeterProvider, meter metric.Meter) (*OTel, error) {
	var err error

	o := &OTel{opts: opts, provider: provider}

	if o.routeLookupM, err = meter.Float64Histogram(
		namespace+"_route_lookup_duration_seconds",
		metric.WithDescription("Duration in seconds of a route lookup."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.routeErrorsM, err = meter.Int64Counter(
		namespace+"_route_error_total",
		metric.WithDescription("The total of route lookup errors."),
	); err != nil {
		return nil, err
	}

	if o.responseM, err = meter.Float64Histogram(
		namespace+"_response_duration_seconds",
		metric.WithDescription("Duration in seconds of a response."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.responseSizeM, err = meter.Float64Histogram(
		namespace+"_response_size_bytes",
		metric.WithDescription("Size of response in bytes."),
		metric.WithUnit("By"),
	); err != nil {
		return nil, err
	}

	if o.filterCreateM, err = meter.Float64Histogram(
		namespace+"_filter_create_duration_seconds",
		metric.WithDescription("Duration in seconds of filter creation."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterRequestM, err = meter.Float64Histogram(
		namespace+"_filter_request_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter request."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterAllRequestM, err = meter.Float64Histogram(
		namespace+"_filter_all_request_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter request by all filters."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterAllCombinedRequestM, err = meter.Float64Histogram(
		namespace+"_filter_all_combined_request_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter request combined by all filters."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.backendRequestHeadersM, err = meter.Float64Histogram(
		namespace+"_backend_request_header_bytes",
		metric.WithDescription("Size of a backend request header."),
		metric.WithUnit("By"),
	); err != nil {
		return nil, err
	}

	if o.backendM, err = meter.Float64Histogram(
		namespace+"_backend_duration_seconds",
		metric.WithDescription("Duration in seconds of a proxy backend."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.backendCombinedM, err = meter.Float64Histogram(
		namespace+"_backend_combined_duration_seconds",
		metric.WithDescription("Duration in seconds of a proxy backend combined."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterResponseM, err = meter.Float64Histogram(
		namespace+"_filter_response_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter response."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterAllResponseM, err = meter.Float64Histogram(
		namespace+"_filter_all_response_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter response by all filters."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.filterAllCombinedResponseM, err = meter.Float64Histogram(
		namespace+"_filter_all_combined_response_duration_seconds",
		metric.WithDescription("Duration in seconds of a filter response combined by all filters."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.serveRouteM, err = meter.Float64Histogram(
		namespace+"_serve_route_duration_seconds",
		metric.WithDescription("Duration in seconds of serving a route."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.serveRouteCounterM, err = meter.Int64Counter(
		namespace+"_serve_route_count",
		metric.WithDescription("Total number of requests of serving a route."),
	); err != nil {
		return nil, err
	}

	if o.serveHostM, err = meter.Float64Histogram(
		namespace+"_serve_host_duration_seconds",
		metric.WithDescription("Duration in seconds of serving a host."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.serveHostCounterM, err = meter.Int64Counter(
		namespace+"_serve_host_count",
		metric.WithDescription("Total number of requests of serving a host."),
	); err != nil {
		return nil, err
	}

	if o.proxyTotalM, err = meter.Float64Histogram(
		namespace+"_proxy_total_duration_seconds",
		metric.WithDescription("Total duration in seconds of skipper latency."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.proxyRequestM, err = meter.Float64Histogram(
		namespace+"_proxy_request_duration_seconds",
		metric.WithDescription("Duration in seconds of skipper latency for request."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.proxyResponseM, err = meter.Float64Histogram(
		namespace+"_proxy_response_duration_seconds",
		metric.WithDescription("Duration in seconds of skipper latency for response."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.backend5xxM, err = meter.Float64Histogram(
		namespace+"_backend_5xx_duration_seconds",
		metric.WithDescription("Duration in seconds of backend 5xx."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.backendErrorsM, err = meter.Int64Counter(
		namespace+"_backend_error_total",
		metric.WithDescription("Total number of backend route errors."),
	); err != nil {
		return nil, err
	}

	if o.proxyStreamingErrorsM, err = meter.Int64Counter(
		namespace+"_streaming_error_total",
		metric.WithDescription("Total number of streaming route errors."),
	); err != nil {
		return nil, err
	}

	if o.customHistogramM, err = meter.Float64Histogram(
		namespace+"_custom_duration_seconds",
		metric.WithDescription("Duration in seconds of custom metrics."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	if o.customCounterM, err = meter.Float64Counter(
		namespace+"_custom_total",
		metric.WithDescription("Total number of custom metrics."),
	); err != nil {
		return nil, err
	}

	if o.customGaugeM, err = meter.Float64Gauge(
		namespace+"_custom_gauges",
		metric.WithDescription("Gauges number of custom metrics."),
	); err != nil {
		return nil, err
	}

	if o.invalidRouteM, err = meter.Float64Gauge(
		namespace+"_route_invalid",
		metric.WithDescription("Invalid route by route ID and name."),
	); err != nil {
		return nil, err
	}

	return o, nil
}

// otelHistogramViews returns sdkmetric views that configure explicit bucket boundaries per instrument.
func otelHistogramViews(namespace string, durationBuckets, responseSizeBuckets, requestSizeBuckets []float64) []sdkmetric.View {
	durationNames := []string{
		namespace + "_route_lookup_duration_seconds",
		namespace + "_response_duration_seconds",
		namespace + "_filter_create_duration_seconds",
		namespace + "_filter_request_duration_seconds",
		namespace + "_filter_all_request_duration_seconds",
		namespace + "_filter_all_combined_request_duration_seconds",
		namespace + "_backend_duration_seconds",
		namespace + "_backend_combined_duration_seconds",
		namespace + "_filter_response_duration_seconds",
		namespace + "_filter_all_response_duration_seconds",
		namespace + "_filter_all_combined_response_duration_seconds",
		namespace + "_serve_route_duration_seconds",
		namespace + "_serve_host_duration_seconds",
		namespace + "_proxy_total_duration_seconds",
		namespace + "_proxy_request_duration_seconds",
		namespace + "_proxy_response_duration_seconds",
		namespace + "_backend_5xx_duration_seconds",
		namespace + "_custom_duration_seconds",
	}

	views := make([]sdkmetric.View, 0, len(durationNames)+2)

	if len(durationBuckets) > 0 {
		for _, name := range durationNames {
			views = append(views, sdkmetric.NewView(
				sdkmetric.Instrument{Name: name},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: durationBuckets,
					},
				},
			))
		}
	}

	views = append(views, sdkmetric.NewView(
		sdkmetric.Instrument{Name: namespace + "_response_size_bytes"},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: responseSizeBuckets,
			},
		},
	))

	views = append(views, sdkmetric.NewView(
		sdkmetric.Instrument{Name: namespace + "_backend_request_header_bytes"},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: requestSizeBuckets,
			},
		},
	))

	return views
}

func (o *OTel) sinceS(start time.Time) float64 {
	return time.Since(start).Seconds()
}

// MeasureSince satisfies Metrics interface.
func (o *OTel) MeasureSince(key string, start time.Time) {
	o.customHistogramM.Record(context.Background(), o.sinceS(start), metric.WithAttributes(attribute.String("key", key)))
}

// IncCounter satisfies Metrics interface.
func (o *OTel) IncCounter(key string) {
	o.customCounterM.Add(context.Background(), 1, metric.WithAttributes(attribute.String("key", key)))
}

// IncCounterBy satisfies Metrics interface.
func (o *OTel) IncCounterBy(key string, value int64) {
	o.customCounterM.Add(context.Background(), float64(value), metric.WithAttributes(attribute.String("key", key)))
}

// IncFloatCounterBy satisfies Metrics interface.
func (o *OTel) IncFloatCounterBy(key string, value float64) {
	o.customCounterM.Add(context.Background(), value, metric.WithAttributes(attribute.String("key", key)))
}

// UpdateGauge satisfies Metrics interface.
func (o *OTel) UpdateGauge(key string, value float64) {
	o.customGaugeM.Record(context.Background(), value, metric.WithAttributes(attribute.String("key", key)))
}

// MeasureRouteLookup satisfies Metrics interface.
func (o *OTel) MeasureRouteLookup(start time.Time) {
	o.routeLookupM.Record(context.Background(), o.sinceS(start))
}

// MeasureFilterCreate satisfies Metrics interface.
func (o *OTel) MeasureFilterCreate(filterName string, start time.Time) {
	o.filterCreateM.Record(context.Background(), o.sinceS(start), metric.WithAttributes(attribute.String("filter", filterName)))
}

// MeasureFilterRequest satisfies Metrics interface.
func (o *OTel) MeasureFilterRequest(filterName string, start time.Time) {
	o.filterRequestM.Record(context.Background(), o.sinceS(start), metric.WithAttributes(attribute.String("filter", filterName)))
}

// MeasureAllFiltersRequest satisfies Metrics interface.
func (o *OTel) MeasureAllFiltersRequest(routeId string, start time.Time) {
	t := o.sinceS(start)
	o.filterAllCombinedRequestM.Record(context.Background(), t)
	if o.opts.EnableAllFiltersMetrics {
		o.filterAllRequestM.Record(context.Background(), t, metric.WithAttributes(attribute.String("route", routeId)))
	}
}

// MeasureBackendRequestHeader satisfies Metrics interface.
func (o *OTel) MeasureBackendRequestHeader(host string, size int) {
	o.backendRequestHeadersM.Record(context.Background(), float64(size), metric.WithAttributes(attribute.String("host", hostForKey(host))))
}

// MeasureBackend satisfies Metrics interface.
func (o *OTel) MeasureBackend(routeId string, start time.Time) {
	t := o.sinceS(start)
	o.backendCombinedM.Record(context.Background(), t)
	if o.opts.EnableRouteBackendMetrics {
		o.backendM.Record(context.Background(), t,
			metric.WithAttributes(attribute.String("route", routeId), attribute.String("host", "")))
	}
}

// MeasureBackendHost satisfies Metrics interface.
func (o *OTel) MeasureBackendHost(routeBackendHost string, start time.Time) {
	if o.opts.EnableBackendHostMetrics {
		o.backendM.Record(context.Background(), o.sinceS(start),
			metric.WithAttributes(attribute.String("route", ""), attribute.String("host", routeBackendHost)))
	}
}

// MeasureFilterResponse satisfies Metrics interface.
func (o *OTel) MeasureFilterResponse(filterName string, start time.Time) {
	o.filterResponseM.Record(context.Background(), o.sinceS(start), metric.WithAttributes(attribute.String("filter", filterName)))
}

// MeasureAllFiltersResponse satisfies Metrics interface.
func (o *OTel) MeasureAllFiltersResponse(routeId string, start time.Time) {
	t := o.sinceS(start)
	o.filterAllCombinedResponseM.Record(context.Background(), t)
	if o.opts.EnableAllFiltersMetrics {
		o.filterAllResponseM.Record(context.Background(), t, metric.WithAttributes(attribute.String("route", routeId)))
	}
}

// MeasureResponse satisfies Metrics interface.
func (o *OTel) MeasureResponse(code int, method string, routeId string, start time.Time) {
	method = measuredMethod(method)
	t := o.sinceS(start)
	if o.opts.EnableCombinedResponseMetrics {
		o.responseM.Record(context.Background(), t, metric.WithAttributes(
			attribute.String("code", fmt.Sprint(code)),
			attribute.String("method", method),
			attribute.String("route", ""),
		))
	}
	if o.opts.EnableRouteResponseMetrics {
		o.responseM.Record(context.Background(), t, metric.WithAttributes(
			attribute.String("code", fmt.Sprint(code)),
			attribute.String("method", method),
			attribute.String("route", routeId),
		))
	}
}

// MeasureResponseSize satisfies Metrics interface.
func (o *OTel) MeasureResponseSize(host string, size int64) {
	o.responseSizeM.Record(context.Background(), float64(size), metric.WithAttributes(attribute.String("host", hostForKey(host))))
}

// MeasureProxy satisfies Metrics interface.
func (o *OTel) MeasureProxy(requestDuration, responseDuration time.Duration) {
	total := requestDuration + responseDuration
	o.proxyTotalM.Record(context.Background(), total.Seconds())
	if o.opts.EnableProxyRequestMetrics {
		o.proxyRequestM.Record(context.Background(), requestDuration.Seconds())
	}
	if o.opts.EnableProxyResponseMetrics {
		o.proxyResponseM.Record(context.Background(), responseDuration.Seconds())
	}
}

// MeasureServe satisfies Metrics interface.
func (o *OTel) MeasureServe(routeId, host, method string, code int, start time.Time) {
	method = measuredMethod(method)
	t := o.sinceS(start)

	if o.opts.EnableServeRouteMetrics || o.opts.EnableServeHostMetrics {
		attrs := make([]attribute.KeyValue, 0, 3)
		if o.opts.EnableServeStatusCodeMetric {
			attrs = append(attrs, attribute.String("code", fmt.Sprint(code)))
		}
		if o.opts.EnableServeMethodMetric {
			attrs = append(attrs, attribute.String("method", method))
		}
		if o.opts.EnableServeRouteMetrics {
			o.serveRouteM.Record(context.Background(), t,
				metric.WithAttributes(append(attrs, attribute.String("route", routeId))...))
		}
		if o.opts.EnableServeHostMetrics {
			o.serveHostM.Record(context.Background(), t,
				metric.WithAttributes(append(attrs, attribute.String("host", hostForKey(host)))...))
		}
	}

	if o.opts.EnableServeRouteCounter {
		o.serveRouteCounterM.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("code", fmt.Sprint(code)),
			attribute.String("method", method),
			attribute.String("route", routeId),
		))
	}

	if o.opts.EnableServeHostCounter {
		o.serveHostCounterM.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("code", fmt.Sprint(code)),
			attribute.String("method", method),
			attribute.String("host", hostForKey(host)),
		))
	}
}

// IncRoutingFailures satisfies Metrics interface.
func (o *OTel) IncRoutingFailures() {
	o.routeErrorsM.Add(context.Background(), 1)
}

// IncErrorsBackend satisfies Metrics interface.
func (o *OTel) IncErrorsBackend(routeId string) {
	if o.opts.EnableRouteBackendErrorsCounters {
		o.backendErrorsM.Add(context.Background(), 1, metric.WithAttributes(attribute.String("route", routeId)))
	}
}

// MeasureBackend5xx satisfies Metrics interface.
func (o *OTel) MeasureBackend5xx(start time.Time) {
	o.backend5xxM.Record(context.Background(), o.sinceS(start))
}

// IncErrorsStreaming satisfies Metrics interface.
func (o *OTel) IncErrorsStreaming(routeId string) {
	if o.opts.EnableRouteStreamingErrorsCounters {
		o.proxyStreamingErrorsM.Add(context.Background(), 1, metric.WithAttributes(attribute.String("route", routeId)))
	}
}

// SetInvalidRoute satisfies Metrics interface.
func (o *OTel) SetInvalidRoute(routeId, reason string) {
	o.invalidRouteM.Record(context.Background(), 1, metric.WithAttributes(
		attribute.String("route_id", routeId),
		attribute.String("reason", reason),
	))
}

// RegisterHandler satisfies Metrics interface.
// This is a no-op for the OTel backend; metrics are pushed to an OTLP collector
// configured via OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_METRICS_ENDPOINT.
func (o *OTel) RegisterHandler(_ string, _ *http.ServeMux) {}

// Close satisfies Metrics interface.
func (o *OTel) Close() {
	_ = o.provider.Shutdown(context.Background())
}

func (o *OTel) String() string {
	return "otel"
}
