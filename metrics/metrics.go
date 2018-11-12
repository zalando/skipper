package metrics

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"
)

const (
	defaultMetricsPath = "/metrics"
)

// Kind is the type a metrics expose backend can be.
type Kind int

const (
	UnkownKind   Kind = 0
	CodaHaleKind Kind = 1 << iota
	PrometheusKind
	AllKind = CodaHaleKind | PrometheusKind
)

func (k Kind) String() string {
	switch k {
	case CodaHaleKind:
		return "codahale"
	case PrometheusKind:
		return "prometheus"
	case AllKind:
		return "all"
	default:
		return "unknown"
	}
}

// ParseMetricsKind parses an string and returns the correct Metrics kind.
func ParseMetricsKind(t string) Kind {
	t = strings.ToLower(t)
	switch t {
	case "codahale":
		return CodaHaleKind
	case "prometheus":
		return PrometheusKind
	case "all":
		return AllKind
	default:
		return UnkownKind
	}
}

// Metrics is the generic interface that all the required backends
// should implement to be an skipper metrics compatible backend.
type Metrics interface {
	// Implements the `filter.Metrics` interface.
	MeasureSince(key string, start time.Time)
	IncCounter(key string)
	IncCounterBy(key string, value int64)
	IncFloatCounterBy(key string, value float64)
	// Additional methods
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
	RegisterHandler(path string, handler *http.ServeMux)
	UpdateGauge(key string, value float64)
}

// Options for initializing metrics collection.
type Options struct {
	// the metrics exposing format.
	Format Kind

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

	// HistogramBuckets defines buckets into which the observations are counted for
	// histogram metrics.
	HistogramBuckets []float64

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
	Default Metrics
	Void    Metrics
)

func init() {
	Void = NewVoid()
	Default = Void
}

// NewDefaultHandler returns a default metrics handler.
func NewDefaultHandler(o Options) http.Handler {
	var m Metrics

	switch o.Format {
	case AllKind:
		m = NewAll(o)
	case PrometheusKind:
		m = NewPrometheus(o)
	default:
		// CodaHale is the default backend always.
		m = NewCodaHale(o)
	}

	return NewHandler(o, m)
}

// NewHandler returns a collection of metrics handlers.
func NewHandler(o Options, m Metrics) http.Handler {

	mux := http.NewServeMux()
	if o.EnableProfile {
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}

	// Root path should return 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	Default = m

	// Fix trailing slashes and register routes.
	mPath := defaultMetricsPath
	mPath = strings.TrimRight(mPath, "/")
	m.RegisterHandler(mPath, mux)
	mPath = fmt.Sprintf("%s/", mPath)
	m.RegisterHandler(mPath, mux)

	return mux
}
