package metrics

import (
	"net/http"
	"net/http/pprof"
	"time"
)

// Metrics is the generic interface that all the required backends
// should implement to be an skipper metrics compatible backend.
type Metrics interface {
	MeasureSince(key string, start time.Time)
	IncCounter(key string)
	MeasureRouteLookup(start time.Time)
	MeasureFilterRequest(filterName string, start time.Time)
	MeasureAllFiltersRequest(routeId string, start time.Time)
	MeasureBackend(routeId string, start time.Time)
	MeasureBackendHost(routeBackendHost string, start time.Time)
	MeasureFilterResponse(filterName string, start time.Time)
	MeasureAllFiltersResponse(routeId string, start time.Time)
	MeasureResponse(code int, method string, routeId string, start time.Time)
	MeasureServe(routeId, host, method string, code int, start time.Time)
	IncRoutingFailures()
	IncErrorsBackend(routeId string)
	MeasureBackend5xx(t time.Time)
	IncErrorsStreaming(routeId string)
}

// Options for initializing metrics collection.
type Options struct {
	// Common prefix for the keys of the different
	// collected metrics.
	Prefix string

	// If set, garbage collector metrics are collected
	// in addition to the http traffic metrics.
	EnableDebugGcMetrics bool

	// If set, Go runtime metrics are collected in
	// addition to the http traffic metrics.
	EnableRuntimeMetrics bool

	// If set, detailed total response time metrics will be collected
	// for each route, additionally grouped by status and method.
	EnableServeRouteMetrics bool

	// If set, detailed total response time metrics will be collected
	// for each host, additionally grouped by status and method.
	EnableServeHostMetrics bool

	// If set, detailed response time metrics will be collected
	// for each backend host
	EnableBackendHostMetrics bool

	// EnableAllFiltersMetrics enables collecting combined filter
	// metrics per each route. Without the DisableCompatibilityDefaults,
	// it is enabled by default.
	EnableAllFiltersMetrics bool

	// EnableCombinedResponseMetrics enables collecting response time
	// metrics combined for every route.
	EnableCombinedResponseMetrics bool

	// EnableRouteResponseMetrics enables collecting response time
	// metrics per each route. Without the DisableCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteResponseMetrics bool

	// EnableRouteBackendErrorsCounters enables counters for backend
	// errors per each route. Without the DisableCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteBackendErrorsCounters bool

	// EnableRouteStreamingErrorsCounters enables counters for streaming
	// errors per each route. Without the DisableCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteStreamingErrorsCounters bool

	// EnableRouteBackendMetrics enables backend response time metrics
	// per each route. Without the DisableCompatibilityDefaults, it is
	// enabled by default.
	EnableRouteBackendMetrics bool

	// UseExpDecaySample, when set, makes the histograms use an exponentially
	// decaying sample instead of the default uniform one.
	UseExpDecaySample bool

	// The following options, for backwards compatibility, are true
	// by default: EnableAllFiltersMetrics, EnableRouteResponseMetrics,
	// EnableRouteBackendErrorsCounters, EnableRouteStreamingErrorsCounters,
	// EnableRouteBackendMetrics. With this compatibility flag, the default
	// for these options can be set to false.
	DisableCompatibilityDefaults bool

	// EnableProfile exposes profiling information on /pprof of the
	// metrics listener.
	EnableProfile bool
}

var (
	Default *CodaHale
	Void    *CodaHale
)

func init() {
	Void = NewVoid()
	Default = Void
}

// NewHandler returns a collection of metrics handlers.
func NewHandler(o Options) http.Handler {
	Default = NewCodaHale(o)

	handler := &codaHaleMetricsHandler{registry: Default.reg, options: o}
	if o.EnableProfile {
		mux := http.NewServeMux()
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		handler.profile = mux
	}

	return handler
}
