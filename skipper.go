package skipper

import (
	"io"
	"net/http"
	"os"
	"path"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/innkeeper"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/predicates/cookie"
	"github.com/zalando/skipper/predicates/interval"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

const (
	defaultSourcePollTimeout   = 30 * time.Millisecond
	defaultRoutingUpdateBuffer = 1 << 5
)

// Options to start skipper.
type Options struct {

	// Network address that skipper should listen on.
	Address string

	// List of custom filter specifications.
	CustomFilters []filters.Spec

	// Urls of nodes in an etcd cluster, storing route definitions.
	EtcdUrls []string

	// Path prefix for skipper related data in the etcd storage.
	EtcdPrefix string

	// Timeout used for a single request when querying for updates
	// in etcd. This is independent of, and an addition to,
	// SourcePollTimeout. When not set, the internally defined 1s
	// is used.
	EtcdWaitTimeout time.Duration

	// Skip TLS certificate check for etcd connections.
	EtcdInsecure bool

	// If set enables skipper to generate based on ingress resources in kubernetes cluster
	Kubernetes bool

	// If set makes skipper authenticate with the kubernetes API server with service account assigned to the
	// skipper POD.
	// If omitted skipper will rely on kubectl proxy to authenticate with API server
	KubernetesInCluster bool

	// Kubernetes API base URL. Only makes sense if KubernetesInCluster is set to false. If omitted and
	// skipper is not running in-cluster, the default API URL will be used.
	KubernetesURL string

	// KubernetesHealthcheck, when Kubernetes ingress is set, indicates
	// whether an automatic healthcheck route should be generated. The
	// generated route will report healthyness when the Kubernetes API
	// calls are successful. The healthcheck endpoint is accessible from
	// internal IPs, with the path /kube-system/healthz.
	KubernetesHealthcheck bool

	// KubernetesHTTPSRedirect, when Kubernetes ingress is set, indicates
	// whether an automatic redirect route should be generated to redirect
	// HTTP requests to their HTTPS equivalent. The generated route will
	// match requests with the X-Forwarded-Proto and X-Forwarded-Port,
	// expected to be set by the load-balancer.
	KubernetesHTTPSRedirect bool

	// KubernetesIngressClass is a regular expression, that will make
	// skipper load only the ingress resources that that have a matching
	// kubernetes.io/ingress.class annotation. For backwards compatibility,
	// the ingresses without an annotation, or an empty annotation, will
	// be loaded, too.
	KubernetesIngressClass string

	// API endpoint of the Innkeeper service, storing route definitions.
	InnkeeperUrl string

	// Fixed token for innkeeper authentication. (Used mainly in
	// development environments.)
	InnkeeperAuthToken string

	// Filters to be prepended to each route loaded from Innkeeper.
	InnkeeperPreRouteFilters string

	// Filters to be appended to each route loaded from Innkeeper.
	InnkeeperPostRouteFilters string

	// Skip TLS certificate check for Innkeeper connections.
	InnkeeperInsecure bool

	// OAuth2 URL for Innkeeper authentication.
	OAuthUrl string

	// Directory where oauth credentials are stored, with file names:
	// client.json and user.json.
	OAuthCredentialsDir string

	// The whitespace separated list of OAuth2 scopes.
	OAuthScope string

	// File containing static route definitions.
	RoutesFile string

	// Polling timeout of the routing data sources.
	SourcePollTimeout time.Duration

	// Deprecated. See ProxyFlags. When used together with ProxyFlags,
	// the values will be combined with |.
	ProxyOptions proxy.Options

	// Flags controlling the proxy behavior.
	ProxyFlags proxy.Flags

	// Tells the proxy maximum how many idle connections can it keep
	// alive.
	IdleConnectionsPerHost int

	// Defines the time period of how often the idle connections maintained
	// by the proxy are closed.
	CloseIdleConnsPeriod time.Duration

	// Flag indicating to ignore trailing slashes in paths during route
	// lookup.
	IgnoreTrailingSlash bool

	// Priority routes that are matched against the requests before
	// the standard routes from the data clients.
	PriorityRoutes []proxy.PriorityRoute

	// Specifications of custom, user defined predicates.
	CustomPredicates []routing.PredicateSpec

	// Custom data clients to be used together with the default etcd and Innkeeper.
	CustomDataClients []routing.DataClient

	// Dev mode. Currently this flag disables prioritization of the
	// consumer side over the feeding side during the routing updates to
	// populate the updated routes faster.
	DevMode bool

	// Network address for the support endpoints
	SupportListener string

	// Deprecated: Network address for the /metrics endpoint
	MetricsListener string

	// Skipper provides a set of metrics with different keys which are exposed via HTTP in JSON
	// You can customize those key names with your own prefix
	MetricsPrefix string

	// EnableProfile exposes profiling information on /profile of the
	// metrics listener.
	EnableProfile bool

	// Flag that enables reporting of the Go garbage collector statistics exported in debug.GCStats
	EnableDebugGcMetrics bool

	// Flag that enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats
	EnableRuntimeMetrics bool

	// If set, detailed response time metrics will be collected
	// for each route, additionally grouped by status and method.
	EnableServeRouteMetrics bool

	// If set, detailed response time metrics will be collected
	// for each host, additionally grouped by status and method.
	EnableServeHostMetrics bool

	// If set, detailed response time metrics will be collected
	// for each backend host
	EnableBackendHostMetrics bool

	// EnableAllFiltersMetrics enables collecting combined filter
	// metrics per each route. Without the DisableMetricsCompatibilityDefaults,
	// it is enabled by default.
	EnableAllFiltersMetrics bool

	// EnableRouteResponseMetrics enables collecting response time
	// metrics per each route. Without the DisableMetricsCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteResponseMetrics bool

	// EnableRouteBackendErrorsCounters enables counters for backend
	// errors per each route. Without the DisableMetricsCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteBackendErrorsCounters bool

	// EnableRouteStreamingErrorsCounters enables counters for streaming
	// errors per each route. Without the DisableMetricsCompatibilityDefaults,
	// it is enabled by default.
	EnableRouteStreamingErrorsCounters bool

	// EnableRouteBackendMetrics enables backend response time metrics
	// per each route. Without the DisableMetricsCompatibilityDefaults, it is
	// enabled by default.
	EnableRouteBackendMetrics bool

	// The following options, for backwards compatibility, are true
	// by default: EnableAllFiltersMetrics, EnableRouteResponseMetrics,
	// EnableRouteBackendErrorsCounters, EnableRouteStreamingErrorsCounters,
	// EnableRouteBackendMetrics. With this compatibility flag, the default
	// for these options can be set to false.
	DisableMetricsCompatibilityDefaults bool

	// Output file for the application log. Default value: /dev/stderr.
	//
	// When /dev/stderr or /dev/stdout is passed in, it will be resolved
	// to os.Stderr or os.Stdout.
	//
	// Warning: passing an arbitrary file will try to open it append
	// on start and use it, or fail on start, but the current
	// implementation doesn't support any more proper handling
	// of temporary failures or log-rolling.
	ApplicationLogOutput string

	// Application log prefix. Default value: "[APP]".
	ApplicationLogPrefix string

	// Output file for the access log. Default value: /dev/stderr.
	//
	// When /dev/stderr or /dev/stdout is passed in, it will be resolved
	// to os.Stderr or os.Stdout.
	//
	// Warning: passing an arbitrary file will try to open for append
	// it on start and use it, or fail on start, but the current
	// implementation doesn't support any more proper handling
	// of temporary failures or log-rolling.
	AccessLogOutput string

	// Disables the access log.
	AccessLogDisabled bool

	// Enables logs in JSON format
	AccessLogJSONEnabled bool

	DebugListener string

	//Path of certificate when using TLS
	CertPathTLS string
	//Path of key when using TLS
	KeyPathTLS string

	// Flush interval for upgraded Proxy connections
	BackendFlushInterval time.Duration

	// Experimental feature to handle protocol Upgrades for Websockets, SPDY, etc.
	ExperimentalUpgrade bool

	// MaxLoopbacks defines the maximum number of loops that the proxy can execute when the routing table
	// contains loop backends (<loopback>).
	MaxLoopbacks int

	// EnableBreakers enables the usage of the breakers in the route definitions without initializing any
	// by default. It is a shortcut for setting the BreakerSettings to:
	//
	// 	[]circuit.BreakerSettings{{Type: BreakerDisabled}}
	//
	EnableBreakers bool

	// BreakerSettings contain global and host specific settings for the circuit breakers.
	BreakerSettings []circuit.BreakerSettings

	// OpenTracing enables opentracing
	OpenTracing []string

	// PluginDir defines the dir to load plugins from
	PluginDir string

	// DefaultHTTPStatus is the HTTP status used when no routes are found
	// for a request.
	DefaultHTTPStatus int
}

func createDataClients(o Options, auth innkeeper.Authentication) ([]routing.DataClient, error) {
	var clients []routing.DataClient

	if o.RoutesFile != "" {
		f, err := eskipfile.Open(o.RoutesFile)
		if err != nil {
			log.Error("error while opening eskip file", err)
			return nil, err
		}

		clients = append(clients, f)
	}

	if o.InnkeeperUrl != "" {
		ic, err := innkeeper.New(innkeeper.Options{
			Address:          o.InnkeeperUrl,
			Insecure:         o.InnkeeperInsecure,
			Authentication:   auth,
			PreRouteFilters:  o.InnkeeperPreRouteFilters,
			PostRouteFilters: o.InnkeeperPostRouteFilters,
		})

		if err != nil {
			log.Error("error while initializing Innkeeper client", err)
			return nil, err
		}

		clients = append(clients, ic)
	}

	if len(o.EtcdUrls) > 0 {
		etcdClient, err := etcd.New(etcd.Options{
			Endpoints: o.EtcdUrls,
			Prefix:    o.EtcdPrefix,
			Timeout:   o.EtcdWaitTimeout,
			Insecure:  o.EtcdInsecure,
		})

		if err != nil {
			return nil, err
		}

		clients = append(clients, etcdClient)
	}

	if o.Kubernetes {
		kubernetesClient, err := kubernetes.New(kubernetes.Options{
			KubernetesInCluster:  o.KubernetesInCluster,
			KubernetesURL:        o.KubernetesURL,
			ProvideHealthcheck:   o.KubernetesHealthcheck,
			ProvideHTTPSRedirect: o.KubernetesHTTPSRedirect,
			IngressClass:         o.KubernetesIngressClass,
		})
		if err != nil {
			return nil, err
		}
		clients = append(clients, kubernetesClient)
	}

	return clients, nil
}

func getLogOutput(name string) (io.Writer, error) {
	name = path.Clean(name)

	if name == "/dev/stdout" {
		return os.Stdout, nil
	}

	if name == "/dev/stderr" {
		return os.Stderr, nil
	}

	return os.OpenFile(name, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
}

func initLog(o Options) error {
	var (
		logOutput       io.Writer
		accessLogOutput io.Writer
		err             error
	)

	if o.ApplicationLogOutput != "" {
		logOutput, err = getLogOutput(o.ApplicationLogOutput)
		if err != nil {
			return err
		}
	}

	if !o.AccessLogDisabled && o.AccessLogOutput != "" {
		accessLogOutput, err = getLogOutput(o.AccessLogOutput)
		if err != nil {
			return err
		}
	}

	logging.Init(logging.Options{
		ApplicationLogPrefix: o.ApplicationLogPrefix,
		ApplicationLogOutput: logOutput,
		AccessLogOutput:      accessLogOutput,
		AccessLogDisabled:    o.AccessLogDisabled,
		AccessLogJSONEnabled: o.AccessLogJSONEnabled})

	return nil
}

func (o *Options) isHTTPS() bool {
	return o.CertPathTLS != "" && o.KeyPathTLS != ""
}

func listenAndServe(proxy http.Handler, o *Options) error {
	// create the access log handler
	loggingHandler := logging.NewHandler(proxy)
	log.Infof("proxy listener on %v", o.Address)
	if o.isHTTPS() {
		return http.ListenAndServeTLS(o.Address, o.CertPathTLS, o.KeyPathTLS, loggingHandler)
	}
	log.Infof("certPathTLS or keyPathTLS not found, defaulting to HTTP")
	return http.ListenAndServe(o.Address, loggingHandler)
}

// Run skipper.
func Run(o Options) error {
	// init log
	err := initLog(o)
	if err != nil {
		return err
	}

	// create authentication for Innkeeper
	auth := innkeeper.CreateInnkeeperAuthentication(innkeeper.AuthOptions{
		InnkeeperAuthToken:  o.InnkeeperAuthToken,
		OAuthCredentialsDir: o.OAuthCredentialsDir,
		OAuthUrl:            o.OAuthUrl,
		OAuthScope:          o.OAuthScope})

	// create data clients
	dataClients, err := createDataClients(o, auth)
	if err != nil {
		return err
	}

	// append custom data clients
	dataClients = append(dataClients, o.CustomDataClients...)

	if len(dataClients) == 0 {
		log.Warning("no route source specified")
	}

	// create a filter registry with the available filter specs registered,
	// and register the custom filters
	registry := builtin.MakeRegistry()
	for _, f := range o.CustomFilters {
		registry.Register(f)
	}

	// create routing
	// create the proxy instance
	var mo routing.MatchingOptions
	if o.IgnoreTrailingSlash {
		mo = routing.IgnoreTrailingSlash
	}

	// ensure a non-zero poll timeout
	if o.SourcePollTimeout <= 0 {
		o.SourcePollTimeout = defaultSourcePollTimeout
	}

	// check for dev mode, and set update buffer of the routes
	updateBuffer := defaultRoutingUpdateBuffer
	if o.DevMode {
		updateBuffer = 0
	}

	// include bundled custom predicates
	o.CustomPredicates = append(o.CustomPredicates,
		source.New(),
		interval.NewBetween(),
		interval.NewBefore(),
		interval.NewAfter(),
		cookie.New(),
		query.New(),
		traffic.New())

	// create a routing engine
	routing := routing.New(routing.Options{
		FilterRegistry:  registry,
		MatchingOptions: mo,
		PollTimeout:     o.SourcePollTimeout,
		DataClients:     dataClients,
		Predicates:      o.CustomPredicates,
		UpdateBuffer:    updateBuffer})
	defer routing.Close()

	proxyFlags := proxy.Flags(o.ProxyOptions) | o.ProxyFlags
	proxyParams := proxy.Params{
		Routing:                routing,
		Flags:                  proxyFlags,
		PriorityRoutes:         o.PriorityRoutes,
		IdleConnectionsPerHost: o.IdleConnectionsPerHost,
		CloseIdleConnsPeriod:   o.CloseIdleConnsPeriod,
		FlushInterval:          o.BackendFlushInterval,
		ExperimentalUpgrade:    o.ExperimentalUpgrade,
		MaxLoopbacks:           o.MaxLoopbacks,
		DefaultHTTPStatus:      o.DefaultHTTPStatus,
	}

	if o.EnableBreakers || len(o.BreakerSettings) > 0 {
		proxyParams.CircuitBreakers = circuit.NewRegistry(o.BreakerSettings...)
	}

	if o.DebugListener != "" {
		do := proxyParams
		do.Flags |= proxy.Debug
		dbg := proxy.WithParams(do)
		log.Infof("debug listener on %v", o.DebugListener)
		go func() { http.ListenAndServe(o.DebugListener, dbg) }()
	}

	// init support endpoints
	supportListener := o.SupportListener

	// Backward compatibility
	if supportListener == "" {
		supportListener = o.MetricsListener
	}

	if supportListener != "" {
		mux := http.NewServeMux()
		mux.Handle("/routes", routing)
		mux.Handle("/routes/", routing)
		metricsHandler := metrics.NewHandler(metrics.Options{
			Prefix:                             o.MetricsPrefix,
			EnableDebugGcMetrics:               o.EnableDebugGcMetrics,
			EnableRuntimeMetrics:               o.EnableRuntimeMetrics,
			EnableServeRouteMetrics:            o.EnableServeRouteMetrics,
			EnableServeHostMetrics:             o.EnableServeHostMetrics,
			EnableBackendHostMetrics:           o.EnableBackendHostMetrics,
			EnableProfile:                      o.EnableProfile,
			EnableAllFiltersMetrics:            o.EnableAllFiltersMetrics,
			EnableRouteResponseMetrics:         o.EnableRouteResponseMetrics,
			EnableRouteBackendErrorsCounters:   o.EnableRouteBackendErrorsCounters,
			EnableRouteStreamingErrorsCounters: o.EnableRouteStreamingErrorsCounters,
			EnableRouteBackendMetrics:          o.EnableRouteBackendMetrics,
			DisableCompatibilityDefaults:       o.DisableMetricsCompatibilityDefaults,
		})
		mux.Handle("/metrics", metricsHandler)
		mux.Handle("/metrics/", metricsHandler)
		mux.Handle("/debug/pprof", metricsHandler)
		mux.Handle("/debug/pprof/", metricsHandler)

		log.Infof("support listener on %s", supportListener)
		go http.ListenAndServe(supportListener, mux)
	} else {
		log.Infoln("Metrics are disabled")
	}

	if len(o.OpenTracing) > 0 {
		tracer, err := tracing.LoadPlugin(o.PluginDir, o.OpenTracing)
		if err != nil {
			return err
		}
		proxyParams.OpenTracer = tracer
	} else {
		// always have a tracer available, so filter authors can rely on the
		// existance of a tracer
		proxyParams.OpenTracer, _ = tracing.LoadPlugin(o.PluginDir, []string{"noop"})
	}

	// create the proxy
	proxy := proxy.WithParams(proxyParams)
	defer proxy.Close()

	return listenAndServe(proxy, &o)
}
