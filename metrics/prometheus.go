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
	"google.golang.org/protobuf/proto"
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

// Prometheus implements the prometheus metrics backend.
type Prometheus struct {
	// Metrics.
	routeLookupM               *prometheus.HistogramVec
	routeErrorsM               *prometheus.CounterVec
	responseM                  *prometheus.HistogramVec
	filterCreateM              *prometheus.HistogramVec
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
	}, []string{})

	routeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promRouteSubsystem,
		Name:      "error_total",
		Help:      "The total of route lookup errors.",
	}, []string{})

	response := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promResponseSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a response.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"code", "method", "route"})

	filterCreate := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "create_duration_seconds",
		Help:      "Duration in seconds of filter creation.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"})

	filterRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"})

	filterAllRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_request_duration_seconds",
		Help:      "Duration in seconds of a filter request by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route"})

	filterAllCombinedRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_request_duration_seconds",
		Help:      "Duration in seconds of a filter request combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{})

	proxyBackend := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a proxy backend.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route", "host"})

	proxyBackendCombined := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "combined_duration_seconds",
		Help:      "Duration in seconds of a proxy backend combined.",
		Buckets:   opts.HistogramBuckets,
	}, []string{})

	filterResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "response_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"filter"})

	filterAllResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_response_duration_seconds",
		Help:      "Duration in seconds of a filter response by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"route"})

	filterAllCombinedResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_response_duration_seconds",
		Help:      "Duration in seconds of a filter response combined by all filters.",
		Buckets:   opts.HistogramBuckets,
	}, []string{})

	metrics := []string{}
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
	}, []string{"code", "method", "route"})

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
	}, []string{"code", "method", "host"})

	proxyBackend5xx := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "5xx_duration_seconds",
		Help:      "Duration in seconds of backend 5xx.",
		Buckets:   opts.HistogramBuckets,
	}, []string{})
	proxyBackendErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promProxySubsystem,
		Name:      "error_total",
		Help:      "Total number of backend route errors.",
	}, []string{"route"})
	proxyStreamingErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promStreamingSubsystem,
		Name:      "error_total",
		Help:      "Total number of streaming route errors.",
	}, []string{"route"})

	customCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "total",
		Help:      "Total number of custom metrics.",
	}, []string{"key"})
	customGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "gauges",
		Help:      "Gauges number of custom metrics.",
	}, []string{"key"})
	customHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: promCustomSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of custom metrics.",
		Buckets:   opts.HistogramBuckets,
	}, []string{"key"})

	p := &Prometheus{
		routeLookupM:               routeLookup,
		routeErrorsM:               routeErrors,
		responseM:                  response,
		filterCreateM:              filterCreate,
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

// MeasureBackend satisfies Metrics interface.
func (p *Prometheus) MeasureBackend(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.proxyBackendCombinedM.WithLabelValues().Observe(t)
	if p.opts.EnableRouteBackendMetrics {
		p.proxyBackendM.WithLabelValues(routeID, "").Observe(t)
	}
}

// MeasureBackendHost satisfies Metrics interface.
func (p *Prometheus) MeasureBackendHost(routeBackendHost string, start time.Time) {
	t := p.sinceS(start)
	if p.opts.EnableBackendHostMetrics {
		p.proxyBackendM.WithLabelValues("", routeBackendHost).Observe(t)
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
	p.proxyBackendErrorsM.WithLabelValues(routeID).Inc()
}

// MeasureBackend5xx satisfies Metrics interface.
func (p *Prometheus) MeasureBackend5xx(start time.Time) {
	t := p.sinceS(start)
	p.proxyBackend5xxM.WithLabelValues().Observe(t)
}

// IncErrorsStreaming satisfies Metrics interface.
func (p *Prometheus) IncErrorsStreaming(routeID string) {
	p.proxyStreamingErrorsM.WithLabelValues(routeID).Inc()
}

func (p *Prometheus) Close() {}

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
					Name:  proto.String("start"),
					Value: proto.String(fmt.Sprintf("%d", metric.Counter.CreatedTimestamp.AsTime().UnixNano())),
				})
			}
		}
	}
	return metricFamilies, err
}
