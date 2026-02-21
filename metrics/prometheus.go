package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

const (
	promNamespace          = "skipper"
	promRouteSubsystem     = "route"
	promFilterSubsystem    = "filter"
	promBackendSubsystem   = "backend"
	promStreamingSubsystem = "streaming"
	promProxySubsystem     = "proxy"
	promResponseSubsystem  = "response"
	promServeSubsystem     = "serve"
	promCustomSubsystem    = "custom"
)

const (
	KiB = 1024
	MiB = 1024 * KiB
	GiB = 1024 * MiB
)

// DefaultRequestSizeBuckets are chosen to cover typical max request header sizes:
//   - 64 KiB for [AWS ELB](https://docs.aws.amazon.com/elasticloadbalancing/latest/userguide/how-elastic-load-balancing-works.html#http-header-limits)
//   - 16 KiB for [NodeJS](https://nodejs.org/api/cli.html#cli_max_http_header_size_size)
//   - 8 KiB for [Nginx](https://nginx.org/en/docs/http/ngx_http_core_module.html#large_client_header_buffers)
//   - 8 KiB for [Spring Boot](https://docs.spring.io/spring-boot/appendix/application-properties/index.html#application-properties.server.server.max-http-request-header-size)
var DefaultRequestSizeBuckets = []float64{4 * KiB, 8 * KiB, 16 * KiB, 64 * KiB}

// DefaultResponseSizeBuckets are chosen to cover 2^(10*n) sizes up to 1 GiB and halves of those.
var DefaultResponseSizeBuckets = []float64{1, 512, 1 * KiB, 512 * KiB, 1 * MiB, 512 * MiB, 1 * GiB}

// Prometheus implements the prometheus metrics backend.
type Prometheus struct {
	// Metrics.
	routeLookupM               *prometheus.HistogramVec
	routeErrorsM               *prometheus.CounterVec
	responseM                  *prometheus.HistogramVec
	responseSizeM              *prometheus.HistogramVec
	filterCreateM              *prometheus.HistogramVec
	filterRequestM             *prometheus.HistogramVec
	filterAllRequestM          *prometheus.HistogramVec
	filterAllCombinedRequestM  *prometheus.HistogramVec
	backendRequestHeadersM     *prometheus.HistogramVec
	backendM                   *prometheus.HistogramVec
	backendCombinedM           *prometheus.HistogramVec
	filterResponseM            *prometheus.HistogramVec
	filterAllResponseM         *prometheus.HistogramVec
	filterAllCombinedResponseM *prometheus.HistogramVec
	serveRouteM                *prometheus.HistogramVec
	serveRouteCounterM         *prometheus.CounterVec
	serveHostM                 *prometheus.HistogramVec
	serveHostCounterM          *prometheus.CounterVec
	proxyTotalM                *prometheus.HistogramVec
	proxyRequestM              *prometheus.HistogramVec
	proxyResponseM             *prometheus.HistogramVec
	backend5xxM                *prometheus.HistogramVec
	backendErrorsM             *prometheus.CounterVec
	proxyStreamingErrorsM      *prometheus.CounterVec
	customHistogramM           *prometheus.HistogramVec
	customCounterM             *prometheus.CounterVec
	customGaugeM               *prometheus.GaugeVec
	invalidRouteM              *prometheus.GaugeVec

	opts      Options
	registry  *prometheus.Registry
	handler   http.Handler
	namespace string
}

// NewPrometheus returns a new Prometheus metric backend.
func NewPrometheus(opts Options) *Prometheus {
	opts = applyCompatibilityDefaults(opts)

	p := &Prometheus{
		registry: opts.PrometheusRegistry,
		opts:     opts,
	}

	if p.registry == nil {
		p.registry = prometheus.NewRegistry()
	}

	responseSizeBuckets := DefaultResponseSizeBuckets
	if len(opts.ResponseSizeBuckets) > 1 {
		responseSizeBuckets = opts.ResponseSizeBuckets
	}

	requestSizeBuckets := DefaultRequestSizeBuckets
	if len(opts.RequestSizeBuckets) > 1 {
		requestSizeBuckets = opts.RequestSizeBuckets
	}

	namespace := promNamespace
	if opts.Prefix != "" {
		namespace = strings.TrimSuffix(opts.Prefix, ".")
	}
	p.namespace = namespace

	p.routeLookupM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "lookup_duration_seconds",
		Help:      "Duration in seconds of a route lookup.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.routeErrorsM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "error_total",
		Help:      "The total of route lookup errors.",
	}, []string{}))

	p.responseM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promResponseSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a response.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"code", "method", "route"}))

	p.responseSizeM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promResponseSubsystem,
		Name:      "size_bytes",
		Help:      "Size of response in bytes.",
		Buckets:   responseSizeBuckets,
	}, []string{"host"}))

	p.filterCreateM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "create_duration_seconds",
		Help:      "Duration in seconds of filter creation.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"}))

	p.filterRequestM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"}))

	p.filterAllRequestM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_request_duration_seconds",
		Help:      "Duration in seconds of a filter request by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route"}))

	p.filterAllCombinedRequestM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_request_duration_seconds",
		Help:      "Duration in seconds of a filter request combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.backendRequestHeadersM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promBackendSubsystem,
		Name:      "request_header_bytes",
		Help:      "Size of a backend request header.",
		Buckets:   requestSizeBuckets,
	}, []string{"host"}))

	p.backendM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promBackendSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a proxy backend.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route", "host"}))

	p.backendCombinedM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promBackendSubsystem,
		Name:      "combined_duration_seconds",
		Help:      "Duration in seconds of a proxy backend combined.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.filterResponseM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "response_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"}))

	p.filterAllResponseM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_response_duration_seconds",
		Help:      "Duration in seconds of a filter response by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route"}))

	p.filterAllCombinedResponseM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_response_duration_seconds",
		Help:      "Duration in seconds of a filter response combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	metrics := []string{}
	if opts.EnableServeStatusCodeMetric {
		metrics = append(metrics, "code")
	}
	if opts.EnableServeMethodMetric {
		metrics = append(metrics, "method")
	}
	p.serveRouteM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "route_duration_seconds",
		Help:      "Duration in seconds of serving a route.",
		Buckets:   opts.HistogramBuckets,
	}, append(metrics, "route")))
	p.serveRouteCounterM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "route_count",
		Help:      "Total number of requests of serving a route.",
	}, []string{"code", "method", "route"}))

	p.serveHostM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "host_duration_seconds",
		Help:      "Duration in seconds of serving a host.",
		Buckets:   opts.HistogramBuckets,
	}, append(metrics, "host")))
	p.serveHostCounterM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "host_count",
		Help:      "Total number of requests of serving a host.",
	}, []string{"code", "method", "host"}))

	p.proxyTotalM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "total_duration_seconds",
		Help:      "Total duration in seconds of skipper latency.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.proxyRequestM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of skipper latency for request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.proxyResponseM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "response_duration_seconds",
		Help:      "Duration in seconds of skipper latency for response.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))

	p.backend5xxM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promBackendSubsystem,
		Name:      "5xx_duration_seconds",
		Help:      "Duration in seconds of backend 5xx.",
		Buckets:   opts.HistogramBuckets,
	}, []string{}))
	p.backendErrorsM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promBackendSubsystem,
		Name:      "error_total",
		Help:      "Total number of backend route errors.",
	}, []string{"route"}))

	p.proxyStreamingErrorsM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promStreamingSubsystem,
		Name:      "error_total",
		Help:      "Total number of streaming route errors.",
	}, []string{"route"}))

	p.customCounterM = register(p, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "total",
		Help:      "Total number of custom metrics.",
	}, []string{"key"}))
	p.customGaugeM = register(p, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "gauges",
		Help:      "Gauges number of custom metrics.",
	}, []string{"key"}))
	p.customHistogramM = register(p, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of custom metrics.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"key"}))

	p.invalidRouteM = register(p, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "invalid",
		Help:      "Invalid route by route ID and name.",
	}, []string{"route_id", "reason"}))

	// Register prometheus runtime collectors if required.
	if opts.EnableRuntimeMetrics {
		register(p, collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		register(p, collectors.NewGoCollector())
	}

	return p
}

// sinceS returns the seconds passed since the start time until now.
func (p *Prometheus) sinceS(start time.Time) float64 {
	return time.Since(start).Seconds()
}

func register[T prometheus.Collector](p *Prometheus, cs T) T {
	p.registry.MustRegister(cs)
	return cs
}

func (p *Prometheus) CreateHandler() http.Handler {
	var gatherer prometheus.Gatherer = p.registry
	if p.opts.EnablePrometheusStartLabel {
		gatherer = withStartLabelGatherer{p.registry}
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

func (p *Prometheus) getHandler() http.Handler {
	if p.handler != nil {
		return p.handler
	}

	p.handler = p.CreateHandler()
	return p.handler
}

// RegisterHandler satisfies Metrics interface.
func (p *Prometheus) RegisterHandler(path string, mux *http.ServeMux) {
	promHandler := p.getHandler()
	mux.Handle(path, promHandler)
}

// MeasureSince satisfies Metrics interface.
func (p *Prometheus) MeasureSince(key string, start time.Time) {
	t := p.sinceS(start)
	p.customHistogramM.WithLabelValues(key).Observe(t)
}

// IncCounter satisfies Metrics interface.
func (p *Prometheus) IncCounter(key string) {
	p.customCounterM.WithLabelValues(key).Inc()
}

// IncCounterBy satisfies Metrics interface.
func (p *Prometheus) IncCounterBy(key string, value int64) {
	f := float64(value)
	p.customCounterM.WithLabelValues(key).Add(f)
}

// IncFloatCounterBy satisfies Metrics interface.
func (p *Prometheus) IncFloatCounterBy(key string, value float64) {
	p.customCounterM.WithLabelValues(key).Add(value)
}

// UpdateGauge satisfies Metrics interface.
func (p *Prometheus) UpdateGauge(key string, v float64) {
	p.customGaugeM.WithLabelValues(key).Set(v)
}

// MeasureRouteLookup satisfies Metrics interface.
func (p *Prometheus) MeasureRouteLookup(start time.Time) {
	t := p.sinceS(start)
	p.routeLookupM.WithLabelValues().Observe(t)
}

func (p *Prometheus) MeasureFilterCreate(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterCreateM.WithLabelValues(filterName).Observe(t)
}

// MeasureFilterRequest satisfies Metrics interface.
func (p *Prometheus) MeasureFilterRequest(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterRequestM.WithLabelValues(filterName).Observe(t)
}

// MeasureAllFiltersRequest satisfies Metrics interface.
func (p *Prometheus) MeasureAllFiltersRequest(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedRequestM.WithLabelValues().Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllRequestM.WithLabelValues(routeID).Observe(t)
	}
}

func (p *Prometheus) MeasureBackendRequestHeader(host string, size int) {
	p.backendRequestHeadersM.WithLabelValues(hostForKey(host)).Observe(float64(size))
}

// MeasureBackend satisfies Metrics interface.
func (p *Prometheus) MeasureBackend(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.backendCombinedM.WithLabelValues().Observe(t)
	if p.opts.EnableRouteBackendMetrics {
		p.backendM.WithLabelValues(routeID, "").Observe(t)
	}
}

// MeasureBackendHost satisfies Metrics interface.
func (p *Prometheus) MeasureBackendHost(routeBackendHost string, start time.Time) {
	t := p.sinceS(start)
	if p.opts.EnableBackendHostMetrics {
		p.backendM.WithLabelValues("", routeBackendHost).Observe(t)
	}
}

// MeasureFilterResponse satisfies Metrics interface.
func (p *Prometheus) MeasureFilterResponse(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterResponseM.WithLabelValues(filterName).Observe(t)
}

// MeasureAllFiltersResponse satisfies Metrics interface.
func (p *Prometheus) MeasureAllFiltersResponse(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedResponseM.WithLabelValues().Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllResponseM.WithLabelValues(routeID).Observe(t)
	}
}

// MeasureResponse satisfies Metrics interface.
func (p *Prometheus) MeasureResponse(code int, method string, routeID string, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)
	if p.opts.EnableCombinedResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprint(code), method, "").Observe(t)
	}
	if p.opts.EnableRouteResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprint(code), method, routeID).Observe(t)
	}
}

func (p *Prometheus) MeasureResponseSize(host string, size int64) {
	p.responseSizeM.WithLabelValues(hostForKey(host)).Observe(float64(size))
}

func (p *Prometheus) MeasureProxy(requestDuration, responseDuration time.Duration) {
	skipperDuration := requestDuration + responseDuration
	p.proxyTotalM.WithLabelValues().Observe(skipperDuration.Seconds())
	if p.opts.EnableProxyRequestMetrics {
		p.proxyRequestM.WithLabelValues().Observe(requestDuration.Seconds())
	}
	if p.opts.EnableProxyResponseMetrics {
		p.proxyResponseM.WithLabelValues().Observe(responseDuration.Seconds())
	}
}

// MeasureServe satisfies Metrics interface.
func (p *Prometheus) MeasureServe(routeID, host, method string, code int, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)

	if p.opts.EnableServeRouteMetrics || p.opts.EnableServeHostMetrics {
		metrics := []string{}
		if p.opts.EnableServeStatusCodeMetric {
			metrics = append(metrics, fmt.Sprint(code))
		}
		if p.opts.EnableServeMethodMetric {
			metrics = append(metrics, method)
		}
		if p.opts.EnableServeRouteMetrics {
			p.serveRouteM.WithLabelValues(append(metrics, routeID)...).Observe(t)
		}
		if p.opts.EnableServeHostMetrics {
			p.serveHostM.WithLabelValues(append(metrics, hostForKey(host))...).Observe(t)
		}
	}

	if p.opts.EnableServeRouteCounter {
		p.serveRouteCounterM.WithLabelValues(fmt.Sprint(code), method, routeID).Inc()
	}

	if p.opts.EnableServeHostCounter {
		p.serveHostCounterM.WithLabelValues(fmt.Sprint(code), method, hostForKey(host)).Inc()
	}
}

// IncRoutingFailures satisfies Metrics interface.
func (p *Prometheus) IncRoutingFailures() {
	p.routeErrorsM.WithLabelValues().Inc()
}

// IncErrorsBackend satisfies Metrics interface.
func (p *Prometheus) IncErrorsBackend(routeID string) {
	p.backendErrorsM.WithLabelValues(routeID).Inc()
}

// MeasureBackend5xx satisfies Metrics interface.
func (p *Prometheus) MeasureBackend5xx(start time.Time) {
	t := p.sinceS(start)
	p.backend5xxM.WithLabelValues().Observe(t)
}

// IncErrorsStreaming satisfies Metrics interface.
func (p *Prometheus) IncErrorsStreaming(routeID string) {
	p.proxyStreamingErrorsM.WithLabelValues(routeID).Inc()
}

// SetInvalidRoute satisfies Metrics interface.
func (p *Prometheus) SetInvalidRoute(routeId, reason string) {
	p.invalidRouteM.WithLabelValues(routeId, reason).Set(1)
}

func (p *Prometheus) Close() {}

// ScopedPrometheusRegisterer implements the PrometheusMetrics interface
func (p *Prometheus) ScopedPrometheusRegisterer(subsystem string) prometheus.Registerer {
	return prometheus.WrapRegistererWithPrefix(p.namespace+"_"+subsystem+"_", p.registry)
}

// withStartLabelGatherer adds a "start" label to all counters with
// the value of counter creation timestamp as unix nanoseconds.
type withStartLabelGatherer struct {
	*prometheus.Registry
}

func (g withStartLabelGatherer) Gather() ([]*dto.MetricFamily, error) {
	metricFamilies, err := g.Registry.Gather()
	for _, metricFamily := range metricFamilies {
		if metricFamily.GetType() == dto.MetricType_COUNTER {
			for _, metric := range metricFamily.Metric {
				metric.Label = append(metric.Label, &dto.LabelPair{
					Name:  new("start"),
					Value: new(fmt.Sprintf("%d", metric.Counter.CreatedTimestamp.AsTime().UnixNano())),
				})
			}
		}
	}
	return metricFamilies, err
}
