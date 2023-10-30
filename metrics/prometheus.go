package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	promNamespace          = "skipper"
	promRouteSubsystem     = "route"
	promFilterSubsystem    = "filter"
	promProxySubsystem     = "backend"
	promStreamingSubsystem = "streaming"
	promResponseSubsystem  = "response"
	promServeSubsystem     = "serve"
	promCustomSubsystem    = "custom"
)

var version string

// Prometheus implements the prometheus metrics backend.
type Prometheus struct {
	// Metrics.
	routeLookupM               *prometheus.HistogramVec
	routeErrorsM               *prometheus.CounterVec
	responseM                  *prometheus.HistogramVec
	filterRequestM             *prometheus.HistogramVec
	filterAllRequestM          *prometheus.HistogramVec
	filterAllCombinedRequestM  *prometheus.HistogramVec
	proxyBackendM              *prometheus.HistogramVec
	proxyBackendCombinedM      *prometheus.HistogramVec
	filterResponseM            *prometheus.HistogramVec
	filterAllResponseM         *prometheus.HistogramVec
	filterAllCombinedResponseM *prometheus.HistogramVec
	serveRouteM                *prometheus.HistogramVec
	serveRouteCounterM         *prometheus.CounterVec
	serveHostM                 *prometheus.HistogramVec
	serveHostCounterM          *prometheus.CounterVec
	proxyBackend5xxM           *prometheus.HistogramVec
	proxyBackendErrorsM        *prometheus.CounterVec
	proxyStreamingErrorsM      *prometheus.CounterVec
	customHistogramM           *prometheus.HistogramVec
	customCounterM             *prometheus.CounterVec
	customGaugeM               *prometheus.GaugeVec

	opts     Options
	registry *prometheus.Registry
	handler  http.Handler
}

// NewPrometheus returns a new Prometheus metric backend.
func NewPrometheus(opts Options) *Prometheus {
	opts = applyCompatibilityDefaults(opts)

	namespace := promNamespace
	if opts.Prefix != "" {
		namespace = strings.TrimSuffix(opts.Prefix, ".")
	}

	routeLookup := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "lookup_duration_seconds",
		Help:      "Duration in seconds of a route lookup.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"version"})

	routeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "error_total",
		Help:      "The total of route lookup errors.",
	}, []string{"version"})

	response := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promResponseSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a response.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"code", "method", "route"})

	filterRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter", "version"})

	filterAllRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_request_duration_seconds",
		Help:      "Duration in seconds of a filter request by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route", "version"})

	filterAllCombinedRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_request_duration_seconds",
		Help:      "Duration in seconds of a filter request combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"version"})

	proxyBackend := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a proxy backend.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route", "host", "version"})

	proxyBackendCombined := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "combined_duration_seconds",
		Help:      "Duration in seconds of a proxy backend combined.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"version"})

	filterResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "response_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter", "version"})

	filterAllResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_response_duration_seconds",
		Help:      "Duration in seconds of a filter response by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route", "version"})

	filterAllCombinedResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_response_duration_seconds",
		Help:      "Duration in seconds of a filter response combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"version"})

	metrics := []string{"version"}
	if opts.EnableServeStatusCodeMetric {
		metrics = append(metrics, "code")
	}
	if opts.EnableServeMethodMetric {
		metrics = append(metrics, "method")
	}
	serveRoute := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "route_duration_seconds",
		Help:      "Duration in seconds of serving a route.",
		Buckets:   opts.HistogramBuckets,
	}, append(metrics, "route"))
	serveRouteCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "route_count",
		Help:      "Total number of requests of serving a route.",
	}, []string{"code", "method", "route", "version"})

	serveHost := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "host_duration_seconds",
		Help:      "Duration in seconds of serving a host.",
		Buckets:   opts.HistogramBuckets,
	}, append(metrics, "host"))

	serveHostCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promServeSubsystem,
		Name:      "host_count",
		Help:      "Total number of requests of serving a host.",
	}, []string{"code", "method", "host", "version"})

	proxyBackend5xx := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "5xx_duration_seconds",
		Help:      "Duration in seconds of backend 5xx.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"version"})
	proxyBackendErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "error_total",
		Help:      "Total number of backend route errors.",
	}, []string{"route", "version"})
	proxyStreamingErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promStreamingSubsystem,
		Name:      "error_total",
		Help:      "Total number of streaming route errors.",
	}, []string{"route", "version"})

	customCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "total",
		Help:      "Total number of custom metrics.",
	}, []string{"key", "version"})
	customGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "gauges",
		Help:      "Gauges number of custom metrics.",
	}, []string{"key", "version"})
	customHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of custom metrics.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"key", "version"})

	p := &Prometheus{
		routeLookupM:               routeLookup,
		routeErrorsM:               routeErrors,
		responseM:                  response,
		filterRequestM:             filterRequest,
		filterAllRequestM:          filterAllRequest,
		filterAllCombinedRequestM:  filterAllCombinedRequest,
		proxyBackendM:              proxyBackend,
		proxyBackendCombinedM:      proxyBackendCombined,
		filterResponseM:            filterResponse,
		filterAllResponseM:         filterAllResponse,
		filterAllCombinedResponseM: filterAllCombinedResponse,
		serveRouteM:                serveRoute,
		serveRouteCounterM:         serveRouteCounter,
		serveHostM:                 serveHost,
		serveHostCounterM:          serveHostCounter,
		proxyBackend5xxM:           proxyBackend5xx,
		proxyBackendErrorsM:        proxyBackendErrors,
		proxyStreamingErrorsM:      proxyStreamingErrors,
		customCounterM:             customCounter,
		customGaugeM:               customGauge,
		customHistogramM:           customHistogram,

		registry: opts.PrometheusRegistry,
		opts:     opts,
	}

	if p.registry == nil {
		p.registry = prometheus.NewRegistry()
	}

	// Register all metrics.
	p.registerMetrics()
	return p
}

// sinceS returns the seconds passed since the start time until now.
func (p *Prometheus) sinceS(start time.Time) float64 {
	return time.Since(start).Seconds()
}

func (p *Prometheus) registerMetrics() {
	p.registry.MustRegister(p.routeLookupM)
	p.registry.MustRegister(p.responseM)
	p.registry.MustRegister(p.routeErrorsM)
	p.registry.MustRegister(p.filterRequestM)
	p.registry.MustRegister(p.filterAllRequestM)
	p.registry.MustRegister(p.filterAllCombinedRequestM)
	p.registry.MustRegister(p.proxyBackendM)
	p.registry.MustRegister(p.proxyBackendCombinedM)
	p.registry.MustRegister(p.filterResponseM)
	p.registry.MustRegister(p.filterAllResponseM)
	p.registry.MustRegister(p.filterAllCombinedResponseM)
	p.registry.MustRegister(p.serveRouteM)
	p.registry.MustRegister(p.serveRouteCounterM)
	p.registry.MustRegister(p.serveHostM)
	p.registry.MustRegister(p.serveHostCounterM)
	p.registry.MustRegister(p.proxyBackend5xxM)
	p.registry.MustRegister(p.proxyBackendErrorsM)
	p.registry.MustRegister(p.proxyStreamingErrorsM)
	p.registry.MustRegister(p.customCounterM)
	p.registry.MustRegister(p.customHistogramM)
	p.registry.MustRegister(p.customGaugeM)

	// Register prometheus runtime collectors if required.
	if p.opts.EnableRuntimeMetrics {
		p.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		p.registry.MustRegister(collectors.NewGoCollector())
	}
}

func (p *Prometheus) CreateHandler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
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
	p.customHistogramM.WithLabelValues(key, version).Observe(t)
}

// IncCounter satisfies Metrics interface.
func (p *Prometheus) IncCounter(key string) {
	p.customCounterM.WithLabelValues(key, version).Inc()
}

// IncCounterBy satisfies Metrics interface.
func (p *Prometheus) IncCounterBy(key string, value int64) {
	f := float64(value)
	p.customCounterM.WithLabelValues(key, version).Add(f)
}

// IncFloatCounterBy satisfies Metrics interface.
func (p *Prometheus) IncFloatCounterBy(key string, value float64) {
	p.customCounterM.WithLabelValues(key, version).Add(value)
}

// UpdateGauge satisfies Metrics interface.
func (p *Prometheus) UpdateGauge(key string, v float64) {
	p.customGaugeM.WithLabelValues(key, version).Set(v)
}

// MeasureRouteLookup satisfies Metrics interface.
func (p *Prometheus) MeasureRouteLookup(start time.Time) {
	t := p.sinceS(start)
	p.routeLookupM.WithLabelValues(version).Observe(t)
}

// MeasureFilterRequest satisfies Metrics interface.
func (p *Prometheus) MeasureFilterRequest(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterRequestM.WithLabelValues(filterName, version).Observe(t)
}

// MeasureAllFiltersRequest satisfies Metrics interface.
func (p *Prometheus) MeasureAllFiltersRequest(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedRequestM.WithLabelValues(version).Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllRequestM.WithLabelValues(routeID, version).Observe(t)
	}
}

// MeasureBackend satisfies Metrics interface.
func (p *Prometheus) MeasureBackend(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.proxyBackendCombinedM.WithLabelValues(version).Observe(t)
	if p.opts.EnableRouteBackendMetrics {
		p.proxyBackendM.WithLabelValues(routeID, "", version).Observe(t)
	}
}

// MeasureBackendHost satisfies Metrics interface.
func (p *Prometheus) MeasureBackendHost(routeBackendHost string, start time.Time) {
	t := p.sinceS(start)
	if p.opts.EnableBackendHostMetrics {
		p.proxyBackendM.WithLabelValues("", routeBackendHost, version).Observe(t)
	}
}

// MeasureFilterResponse satisfies Metrics interface.
func (p *Prometheus) MeasureFilterResponse(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterResponseM.WithLabelValues(filterName, version).Observe(t)
}

// MeasureAllFiltersResponse satisfies Metrics interface.
func (p *Prometheus) MeasureAllFiltersResponse(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedResponseM.WithLabelValues(version).Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllResponseM.WithLabelValues(routeID, version).Observe(t)
	}
}

// MeasureResponse satisfies Metrics interface.
func (p *Prometheus) MeasureResponse(code int, method string, routeID string, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)
	if p.opts.EnableCombinedResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprint(code), method, "", version).Observe(t)
	}
	if p.opts.EnableRouteResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprint(code), method, routeID, version).Observe(t)
	}
}

// MeasureServe satisfies Metrics interface.
func (p *Prometheus) MeasureServe(routeID, host, method string, code int, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)

	if p.opts.EnableServeRouteMetrics || p.opts.EnableServeHostMetrics {
		metrics := []string{version}
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
		p.serveRouteCounterM.WithLabelValues(fmt.Sprint(code), method, routeID, version).Inc()
	}

	if p.opts.EnableServeHostCounter {
		p.serveHostCounterM.WithLabelValues(fmt.Sprint(code), method, hostForKey(host), version).Inc()
	}
}

// IncRoutingFailures satisfies Metrics interface.
func (p *Prometheus) IncRoutingFailures() {
	p.routeErrorsM.WithLabelValues(version).Inc()
}

// IncErrorsBackend satisfies Metrics interface.
func (p *Prometheus) IncErrorsBackend(routeID string) {
	p.proxyBackendErrorsM.WithLabelValues(routeID, version).Inc()
}

// MeasureBackend5xx satisfies Metrics interface.
func (p *Prometheus) MeasureBackend5xx(start time.Time) {
	t := p.sinceS(start)
	p.proxyBackend5xxM.WithLabelValues(version).Observe(t)
}

// IncErrorsStreaming satisfies Metrics interface.
func (p *Prometheus) IncErrorsStreaming(routeID string) {
	p.proxyStreamingErrorsM.WithLabelValues(routeID, version).Inc()
}

func (p *Prometheus) Close() {}
