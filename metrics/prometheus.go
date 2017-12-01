package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
)

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
	serveHostM                 *prometheus.HistogramVec
	serveRouteM                *prometheus.HistogramVec
	proxyBackend5xxM           *prometheus.CounterVec
	proxyBackendErrorsM        *prometheus.CounterVec
	proxyStreamingErrorsM      *prometheus.CounterVec

	opts     Options
	registry *prometheus.Registry
}

// NewPrometheus returns a new Prometheus metric backend.
func NewPrometheus(opts Options) *Prometheus {

	routeLookup := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promRouteSubsystem,
		Name:      "lookup_duration_seconds",
		Help:      "Duration in seconds of a route lookup.",
	}, []string{})

	routeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promRouteSubsystem,
		Name:      "error_total",
		Help:      "The total of route lookup errors.",
	}, []string{})

	response := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promResponseSubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a response.",
	}, []string{"code", "method", "route"})

	filterRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
	}, []string{"filter"})

	filterAllRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_request_duration_seconds",
		Help:      "Duration in seconds of a filter request by all filters.",
	}, []string{"route"})

	filterAllCombinedRequest := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_request_duration_seconds",
		Help:      "Duration in seconds of a filter request combined by all filters.",
	}, []string{})

	proxyBackend := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promProxySubsystem,
		Name:      "duration_seconds",
		Help:      "Duration in seconds of a proxy backend.",
	}, []string{"route", "host"})

	proxyBackendCombined := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promProxySubsystem,
		Name:      "combined_duration_seconds",
		Help:      "Duration in seconds of a proxy backend combined.",
	}, []string{})

	filterResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "response_duration_seconds",
		Help:      "Duration in seconds of a filter request.",
	}, []string{"filter"})

	filterAllResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_response_duration_seconds",
		Help:      "Duration in seconds of a filter response by all filters.",
	}, []string{"route"})

	filterAllCombinedResponse := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promFilterSubsystem,
		Name:      "all_combined_response_duration_seconds",
		Help:      "Duration in seconds of a filter response combined by all filters.",
	}, []string{})

	serveRoute := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promServeSubsystem,
		Name:      "route_duration_seconds",
		Help:      "Duration in seconds of serving a route.",
	}, []string{"code", "method", "route"})

	serveHost := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promServeSubsystem,
		Name:      "host_duration_seconds",
		Help:      "Duration in seconds of serving a host.",
	}, []string{"code", "method", "host"})

	proxyBackend5xx := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promProxySubsystem,
		Name:      "5xx_total",
		Help:      "Total number of backend 5xx errors.",
	}, []string{})
	proxyBackendErrorsM := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promProxySubsystem,
		Name:      "error_total",
		Help:      "Total number of backend route errors.",
	}, []string{"route"})
	proxyStreamingErrorsM := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promStreamingSubsystem,
		Name:      "errors_total",
		Help:      "Total number of streaming route errors.",
	}, []string{"route"})

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
		serveHostM:                 serveHost,
		proxyBackend5xxM:           proxyBackend5xx,
		proxyBackendErrorsM:        proxyBackendErrorsM,
		proxyStreamingErrorsM:      proxyStreamingErrorsM,

		registry: prometheus.NewRegistry(),
	}

	// Register all metrics.
	p.registerMetrics()
	return p
}

// sinceS returns the seconds passed since the start time until now.
func (p *Prometheus) sinceS(start time.Time) float64 {
	return time.Now().Sub(start).Seconds()
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
	p.registry.MustRegister(p.serveHostM)
	p.registry.MustRegister(p.proxyBackend5xxM)
	p.registry.MustRegister(p.proxyBackendErrorsM)
	p.registry.MustRegister(p.proxyStreamingErrorsM)
}

func (p *Prometheus) RegisterHandler(path string, mux *http.ServeMux) {
	memHandler := promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
	mux.Handle(path, memHandler)
}

func (p *Prometheus) MeasureSince(key string, start time.Time) {
}

func (p *Prometheus) IncCounter(key string) {
}

func (p *Prometheus) MeasureRouteLookup(start time.Time) {
	t := p.sinceS(start)
	p.routeLookupM.WithLabelValues().Observe(t)
}

func (p *Prometheus) MeasureFilterRequest(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterRequestM.WithLabelValues(filterName).Observe(t)
}

func (p *Prometheus) MeasureAllFiltersRequest(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedRequestM.WithLabelValues().Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllRequestM.WithLabelValues(routeID).Observe(t)
	}
}

func (p *Prometheus) MeasureBackend(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.proxyBackendCombinedM.WithLabelValues().Observe(t)
	if p.opts.EnableRouteBackendMetrics {
		p.proxyBackendM.WithLabelValues(routeID, "").Observe(t)
	}
}

func (p *Prometheus) MeasureBackendHost(routeBackendHost string, start time.Time) {
	t := p.sinceS(start)
	if p.opts.EnableBackendHostMetrics {
		p.proxyBackendM.WithLabelValues("", routeBackendHost).Observe(t)
	}
}

func (p *Prometheus) MeasureFilterResponse(filterName string, start time.Time) {
	t := p.sinceS(start)
	p.filterResponseM.WithLabelValues(filterName).Observe(t)
}

func (p *Prometheus) MeasureAllFiltersResponse(routeID string, start time.Time) {
	t := p.sinceS(start)
	p.filterAllCombinedResponseM.WithLabelValues().Observe(t)
	if p.opts.EnableAllFiltersMetrics {
		p.filterAllResponseM.WithLabelValues(routeID).Observe(t)
	}
}

func (p *Prometheus) MeasureResponse(code int, method string, routeID string, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)
	if p.opts.EnableCombinedResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprintf("%d", code), method, "").Observe(t)
	}
	if p.opts.EnableRouteResponseMetrics {
		p.responseM.WithLabelValues(fmt.Sprintf("%d", code), method, routeID).Observe(t)
	}
}

func (p *Prometheus) MeasureServe(routeID, host, method string, code int, start time.Time) {
	method = measuredMethod(method)
	t := p.sinceS(start)

	if p.opts.EnableServeRouteMetrics {
		p.serveRouteM.WithLabelValues(fmt.Sprintf("%d", code), method, routeID).Observe(t)
	}

	if p.opts.EnableServeHostMetrics {
		p.serveHostM.WithLabelValues(fmt.Sprintf("%d", code), method, hostForKey(host))
	}
}

func (p *Prometheus) IncRoutingFailures() {
	p.routeErrorsM.WithLabelValues().Inc()
}

func (p *Prometheus) IncErrorsBackend(routeID string) {
	p.proxyBackendErrorsM.WithLabelValues(routeID).Inc()
}

func (p *Prometheus) MeasureBackend5xx(t time.Time) {
	p.proxyBackend5xxM.WithLabelValues().Inc()
}

func (p *Prometheus) IncErrorsStreaming(routeID string) {
	p.proxyStreamingErrorsM.WithLabelValues(routeID).Inc()
}
