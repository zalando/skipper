package skipper

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	al "github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/apiusagemonitoring"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/innkeeper"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	pauth "github.com/zalando/skipper/predicates/auth"
	"github.com/zalando/skipper/predicates/cookie"
	"github.com/zalando/skipper/predicates/interval"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/swarm"
	"github.com/zalando/skipper/tracing"
)

const (
	defaultSourcePollTimeout   = 30 * time.Millisecond
	defaultRoutingUpdateBuffer = 1 << 5
)

const DefaultPluginDir = "./plugins"

// Options to start skipper.
type Options struct {
	// WaitForHealthcheckInterval sets the time that skipper waits
	// for the loadbalancer in front to become unhealthy. Defaults
	// to 0.
	WaitForHealthcheckInterval time.Duration

	// WhitelistedHealthcheckCIDR appends the whitelisted IP Range to the inernalIPS range for healthcheck purposes
	WhitelistedHealthCheckCIDR []string

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

	// If set this value is used as Bearer token for etcd OAuth authorization.
	EtcdOAuthToken string

	// If set this value is used as username for etcd basic authorization.
	EtcdUsername string

	// If set this value is used as password for etcd basic authorization.
	EtcdPassword string

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

	// KubernetesHTTPSRedirectCode overrides the default redirect code (308)
	// when used together with -kubernetes-https-redirect.
	KubernetesHTTPSRedirectCode int

	// KubernetesIngressClass is a regular expression, that will make
	// skipper load only the ingress resources that that have a matching
	// kubernetes.io/ingress.class annotation. For backwards compatibility,
	// the ingresses without an annotation, or an empty annotation, will
	// be loaded, too.
	KubernetesIngressClass string

	// PathMode controls the default interpretation of ingress paths in cases
	// when the ingress doesn't specify it with an annotation.
	KubernetesPathMode kubernetes.PathMode

	// KubernetesNamespace is used to switch between monitoring ingresses in the cluster-scope or limit
	// the ingresses to only those in the specified namespace. Defaults to "" which means monitor ingresses
	// in the cluster-scope.
	KubernetesNamespace string

	// KubernetesEnableEastWest enables cluster internal service to service communication, aka east-west traffic
	KubernetesEnableEastWest bool

	// KubernetesEastWestDomain sets the cluster internal domain used to create additional routes in skipper, defaults to skipper.cluster.local
	KubernetesEastWestDomain string

	// *DEPRECATED* API endpoint of the Innkeeper service, storing route definitions.
	InnkeeperUrl string

	// *DEPRECATED* Fixed token for innkeeper authentication. (Used mainly in
	// development environments.)
	InnkeeperAuthToken string

	// *DEPRECATED* Filters to be prepended to each route loaded from Innkeeper.
	InnkeeperPreRouteFilters string

	// *DEPRECATED* Filters to be appended to each route loaded from Innkeeper.
	InnkeeperPostRouteFilters string

	// *DEPRECATED* Skip TLS certificate check for Innkeeper connections.
	InnkeeperInsecure bool

	// *DEPRECATED* OAuth2 URL for Innkeeper authentication.
	OAuthUrl string

	// *DEPRECATED* Directory where oauth credentials are stored, with file names:
	// client.json and user.json.
	OAuthCredentialsDir string

	// *DEPRECATED* The whitespace separated list of OAuth2 scopes.
	OAuthScope string

	// File containing static route definitions.
	RoutesFile string

	// File containing route definitions with file watch enabled. (For the skipper
	// command this option is used when starting it with the -routes-file flag.)
	WatchRoutesFile string

	// InlineRoutes can define routes as eskip text.
	InlineRoutes string

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

	// Defines ReadTimeoutServer for server http connections.
	ReadTimeoutServer time.Duration

	// Defines ReadHeaderTimeout for server http connections.
	ReadHeaderTimeoutServer time.Duration

	// Defines WriteTimeout for server http connections.
	WriteTimeoutServer time.Duration

	// Defines IdleTimeout for server http connections.
	IdleTimeoutServer time.Duration

	// Defines MaxHeaderBytes for server http connections.
	MaxHeaderBytes int

	// Enable connection state metrics for server http connections.
	EnableConnMetricsServer bool

	// TimeoutBackend sets the TCP client connection timeout for
	// proxy http connections to the backend.
	TimeoutBackend time.Duration

	// ResponseHeaderTimeout sets the HTTP response timeout for
	// proxy http connections to the backend.
	ResponseHeaderTimeoutBackend time.Duration

	// ExpectContinueTimeoutBackend sets the HTTP timeout to expect a
	// response for status Code 100 for proxy http connections to
	// the backend.
	ExpectContinueTimeoutBackend time.Duration

	// KeepAliveBackend sets the TCP keepalive for proxy http
	// connections to the backend.
	KeepAliveBackend time.Duration

	// DualStackBackend sets if the proxy TCP connections to the
	// backend should be dual stack.
	DualStackBackend bool

	// TLSHandshakeTimeoutBackend sets the TLS handshake timeout
	// for proxy connections to the backend.
	TLSHandshakeTimeoutBackend time.Duration

	// MaxIdleConnsBackend sets MaxIdleConns, which limits the
	// number of idle connections to all backends, 0 means no
	// limit.
	MaxIdleConnsBackend int

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

	// WaitFirstRouteLoad prevents starting the listener before the first batch
	// of routes were applied.
	WaitFirstRouteLoad bool

	// SuppressRouteUpdateLogs indicates to log only summaries of the routing updates
	// instead of full details of the updated/deleted routes.
	SuppressRouteUpdateLogs bool

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

	// EnableCombinedResponseMetrics enables collecting response time
	// metrics combined for every route.
	EnableCombinedResponseMetrics bool

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

	// When set, makes the histograms use an exponentially decaying sample
	// instead of the default uniform one.
	MetricsUseExpDecaySample bool

	// Use custom buckets for prometheus histograms.
	HistogramMetricBuckets []float64

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

	// Filter only logs based on status codes, comma separated.
	AccessLogFilter string

	// Enables logs in JSON format
	AccessLogJSONEnabled bool

	// AccessLogStripQuery, when set, causes the query strings stripped
	// from the request URI in the access logs.
	AccessLogStripQuery bool

	DebugListener string

	// Path of certificate(s) when using TLS, mutiple may be given comma separated
	CertPathTLS string
	// Path of key(s) when using TLS, multiple may be given comma separated. For
	// multiple keys, the order must match the one given in CertPathTLS
	KeyPathTLS string

	// TLS Settings for Proxy Server
	ProxyTLS *tls.Config

	// Client TLS to connect to Backends
	ClientTLS *tls.Config

	// Flush interval for upgraded Proxy connections
	BackendFlushInterval time.Duration

	// Experimental feature to handle protocol Upgrades for Websockets, SPDY, etc.
	ExperimentalUpgrade bool

	// ExperimentalUpgradeAudit enables audit log of both the request line
	// and the response messages during web socket upgrades.
	ExperimentalUpgradeAudit bool

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

	// EnableRatelimiters enables the usage of the ratelimiter in the route definitions without initializing any
	// by default. It is a shortcut for setting the RatelimitSettings to:
	//
	// 	[]ratelimit.Settings{{Type: DisableRatelimit}}
	//
	EnableRatelimiters bool

	// RatelimitSettings contain global and host specific settings for the ratelimiters.
	RatelimitSettings []ratelimit.Settings

	// OpenTracing enables opentracing
	OpenTracing []string

	// OpenTracingInitialSpan can override the default initial, pre-routing, span name.
	// Default: "ingress".
	OpenTracingInitialSpan string

	// PluginDir defines the directory to load plugins from, DEPRECATED, use PluginDirs
	PluginDir string
	// PluginDirs defines the directories to load plugins from
	PluginDirs []string

	// FilterPlugins loads additional filters from modules. The first value in each []string
	// needs to be the plugin name (as on disk, without path, without ".so" suffix). The
	// following values are passed as arguments to the plugin while loading, see also
	// https://zalando.github.io/skipper/plugins/
	FilterPlugins [][]string

	// PredicatePlugins loads additional predicates from modules. See above for FilterPlugins
	// what the []string should contain.
	PredicatePlugins [][]string

	// DataClientPlugins loads additional data clients from modules. See above for FilterPlugins
	// what the []string should contain.
	DataClientPlugins [][]string

	// Plugins combine multiple types of the above plugin types in one plugin (where
	// necessary because of shared data between e.g. a filter and a data client).
	Plugins [][]string

	// DefaultHTTPStatus is the HTTP status used when no routes are found
	// for a request.
	DefaultHTTPStatus int

	// EnablePrometheusMetrics enables Prometheus format metrics.
	//
	// This option is *deprecated*. The recommended way to enable prometheus metrics is to
	// use the MetricsFlavours option.
	EnablePrometheusMetrics bool

	// MetricsFlavours sets the metrics storage and exposed format
	// of metrics endpoints.
	MetricsFlavours []string

	// LoadBalancerHealthCheckInterval enables and sets the
	// interval when to schedule health checks for dead or
	// unhealthy routes
	LoadBalancerHealthCheckInterval time.Duration

	// ReverseSourcePredicate enables the automatic use of IP
	// whitelisting in different places to use the reversed way of
	// identifying a client IP within the X-Forwarded-For
	// header. Amazon's ALB for example writes the client IP to
	// the last item of the string list of the X-Forwarded-For
	// header, in this case you want to set this to true.
	ReverseSourcePredicate bool

	// OAuthTokeninfoURL sets the the URL to be queried for
	// information for all auth.NewOAuthTokeninfo*() filters.
	OAuthTokeninfoURL string

	// OAuthTokeninfoTimeout sets timeout duration while calling oauth token service
	OAuthTokeninfoTimeout time.Duration

	// OAuthTokenintrospectionTimeout sets timeout duration while calling oauth tokenintrospection service
	OAuthTokenintrospectionTimeout time.Duration

	// OIDCSecretsFile path to the file containing key to encrypt OpenID token
	OIDCSecretsFile string

	// API Monitoring feature is active (feature toggle)
	ApiUsageMonitoringEnable                       bool
	ApiUsageMonitoringRealmKeys                    string
	ApiUsageMonitoringClientKeys                   string
	ApiUsageMonitoringDefaultClientTrackingPattern string
	ApiUsageMonitoringRealmsTrackingPattern        string

	// WebhookTimeout sets timeout duration while calling a custom webhook auth service
	WebhookTimeout time.Duration

	// MaxAuditBody sets the maximum read size of the body read by the audit log filter
	MaxAuditBody int

	// EnableSwarm enables skipper fleet communication, required by e.g.
	// the cluster ratelimiter
	EnableSwarm bool
	// redis based swarm
	SwarmRedisURLs         []string
	SwarmRedisReadTimeout  time.Duration
	SwarmRedisWriteTimeout time.Duration
	SwarmRedisPoolTimeout  time.Duration
	SwarmRedisMinIdleConns int
	SwarmRedisMaxIdleConns int
	// swim based swarm
	SwarmKubernetesNamespace          string
	SwarmKubernetesLabelSelectorKey   string
	SwarmKubernetesLabelSelectorValue string
	SwarmPort                         int
	SwarmMaxMessageBuffer             int
	SwarmLeaveTimeout                 time.Duration
	// swim based swarm for local testing
	SwarmStaticSelf  string // 127.0.0.1:9001
	SwarmStaticOther string // 127.0.0.1:9002,127.0.0.1:9003
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

	if o.WatchRoutesFile != "" {
		f := eskipfile.Watch(o.WatchRoutesFile)
		clients = append(clients, f)
	}

	if o.InlineRoutes != "" {
		ir, err := routestring.New(o.InlineRoutes)
		if err != nil {
			log.Error("error while parsing inline routes", err)
			return nil, err
		}

		clients = append(clients, ir)
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
			Endpoints:  o.EtcdUrls,
			Prefix:     o.EtcdPrefix,
			Timeout:    o.EtcdWaitTimeout,
			Insecure:   o.EtcdInsecure,
			OAuthToken: o.EtcdOAuthToken,
			Username:   o.EtcdUsername,
			Password:   o.EtcdPassword,
		})

		if err != nil {
			return nil, err
		}

		clients = append(clients, etcdClient)
	}

	if o.Kubernetes {
		kubernetesClient, err := kubernetes.New(kubernetes.Options{
			KubernetesInCluster:        o.KubernetesInCluster,
			KubernetesURL:              o.KubernetesURL,
			ProvideHealthcheck:         o.KubernetesHealthcheck,
			ProvideHTTPSRedirect:       o.KubernetesHTTPSRedirect,
			HTTPSRedirectCode:          o.KubernetesHTTPSRedirectCode,
			IngressClass:               o.KubernetesIngressClass,
			ReverseSourcePredicate:     o.ReverseSourcePredicate,
			WhitelistedHealthCheckCIDR: o.WhitelistedHealthCheckCIDR,
			PathMode:                   o.KubernetesPathMode,
			KubernetesNamespace:        o.KubernetesNamespace,
			KubernetesEnableEastWest:   o.KubernetesEnableEastWest,
			KubernetesEastWestDomain:   o.KubernetesEastWestDomain,
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
		AccessLogJSONEnabled: o.AccessLogJSONEnabled,
		AccessLogStripQuery:  o.AccessLogStripQuery,
	})

	return nil
}

func (o *Options) isHTTPS() bool {
	return (o.ProxyTLS != nil) || (o.CertPathTLS != "" && o.KeyPathTLS != "")
}

func listenAndServe(proxy http.Handler, o *Options) error {
	// create the access log handler
	log.Infof("proxy listener on %v", o.Address)

	srv := &http.Server{
		Addr:              o.Address,
		Handler:           proxy,
		ReadTimeout:       o.ReadTimeoutServer,
		ReadHeaderTimeout: o.ReadHeaderTimeoutServer,
		WriteTimeout:      o.WriteTimeoutServer,
		IdleTimeout:       o.IdleTimeoutServer,
		MaxHeaderBytes:    o.MaxHeaderBytes,
	}

	if o.EnableConnMetricsServer {
		m := metrics.Default
		srv.ConnState = func(conn net.Conn, state http.ConnState) {
			m.IncCounter(fmt.Sprintf("lb-conn-%s", state))
		}
	}

	if o.isHTTPS() {
		if o.ProxyTLS != nil {
			srv.TLSConfig = o.ProxyTLS
			o.CertPathTLS = ""
			o.KeyPathTLS = ""
		} else if strings.Index(o.CertPathTLS, ",") > 0 && strings.Index(o.KeyPathTLS, ",") > 0 {
			tlsCfg := &tls.Config{}
			crts := strings.Split(o.CertPathTLS, ",")
			keys := strings.Split(o.KeyPathTLS, ",")
			if len(crts) != len(keys) {
				log.Fatalf("number of certs does not match number of keys")
			}
			for i, crt := range crts {
				kp, err := tls.LoadX509KeyPair(crt, keys[i])
				if err != nil {
					log.Fatalf("Failed to load X509 keypair from %s/%s: %v", crt, keys[i], err)
				}
				tlsCfg.Certificates = append(tlsCfg.Certificates, kp)
			}
			tlsCfg.BuildNameToCertificate()
			o.CertPathTLS = ""
			o.KeyPathTLS = ""
			srv.TLSConfig = tlsCfg
		}
		return srv.ListenAndServeTLS(o.CertPathTLS, o.KeyPathTLS)
	}
	log.Infof("TLS settings not found, defaulting to HTTP")

	idleConnsCH := make(chan struct{})
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

		<-sigs

		log.Infof("Got shutdown signal, wait %v for health check", o.WaitForHealthcheckInterval)
		time.Sleep(o.WaitForHealthcheckInterval)

		log.Info("Start shutdown")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Errorf("Failed to graceful shutdown: %v", err)
		}
		close(idleConnsCH)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Errorf("Failed to start to ListenAndServe: %v", err)
		return err
	}

	<-idleConnsCH
	log.Infof("done.")
	return nil
}

// Run skipper.
func Run(o Options) error {
	// init log
	err := initLog(o)
	if err != nil {
		return err
	}

	// *DEPRECATED* create authentication for Innkeeper
	inkeeperAuth := innkeeper.CreateInnkeeperAuthentication(innkeeper.AuthOptions{
		InnkeeperAuthToken:  o.InnkeeperAuthToken,
		OAuthCredentialsDir: o.OAuthCredentialsDir,
		OAuthUrl:            o.OAuthUrl,
		OAuthScope:          o.OAuthScope})

	var lbInstance *loadbalancer.LB
	if o.LoadBalancerHealthCheckInterval != 0 {
		lbInstance = loadbalancer.New(o.LoadBalancerHealthCheckInterval)
	}

	if err := o.findAndLoadPlugins(); err != nil {
		return err
	}

	// *DEPRECATED* innkeeper - create data clients
	dataClients, err := createDataClients(o, inkeeperAuth)
	if err != nil {
		return err
	}

	// append custom data clients
	dataClients = append(dataClients, o.CustomDataClients...)

	if len(dataClients) == 0 {
		log.Warning("no route source specified")
	}

	if o.OAuthTokeninfoURL != "" {
		o.CustomFilters = append(o.CustomFilters,
			auth.NewOAuthTokeninfoAllScope(o.OAuthTokeninfoURL, o.OAuthTokeninfoTimeout),
			auth.NewOAuthTokeninfoAnyScope(o.OAuthTokeninfoURL, o.OAuthTokeninfoTimeout),
			auth.NewOAuthTokeninfoAllKV(o.OAuthTokeninfoURL, o.OAuthTokeninfoTimeout),
			auth.NewOAuthTokeninfoAnyKV(o.OAuthTokeninfoURL, o.OAuthTokeninfoTimeout))
	}

	o.CustomFilters = append(o.CustomFilters,
		logfilter.NewAuditLog(o.MaxAuditBody),
		auth.NewOAuthTokenintrospectionAnyClaims(o.OAuthTokenintrospectionTimeout),
		auth.NewOAuthTokenintrospectionAllClaims(o.OAuthTokenintrospectionTimeout),
		auth.NewOAuthTokenintrospectionAnyKV(o.OAuthTokenintrospectionTimeout),
		auth.NewOAuthTokenintrospectionAllKV(o.OAuthTokenintrospectionTimeout),
		auth.NewWebhook(o.WebhookTimeout),
		auth.NewOAuthOidcUserInfos(o.OIDCSecretsFile),
		auth.NewOAuthOidcAnyClaims(o.OIDCSecretsFile),
		auth.NewOAuthOidcAllClaims(o.OIDCSecretsFile),
		apiusagemonitoring.NewApiUsageMonitoring(
			o.ApiUsageMonitoringEnable,
			o.ApiUsageMonitoringRealmKeys,
			o.ApiUsageMonitoringClientKeys,
			o.ApiUsageMonitoringRealmsTrackingPattern,
		),
	)

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
		source.NewFromLast(),
		interval.NewBetween(),
		interval.NewBefore(),
		interval.NewAfter(),
		cookie.New(),
		query.New(),
		traffic.New(),
		pauth.NewJWTPayloadAllKV(),
		pauth.NewJWTPayloadAnyKV(),
	)

	// create a routing engine
	routing := routing.New(routing.Options{
		FilterRegistry:  registry,
		MatchingOptions: mo,
		PollTimeout:     o.SourcePollTimeout,
		DataClients:     dataClients,
		Predicates:      o.CustomPredicates,
		UpdateBuffer:    updateBuffer,
		SuppressLogs:    o.SuppressRouteUpdateLogs,
		PostProcessors: []routing.PostProcessor{
			loadbalancer.HealthcheckPostProcessor{LB: lbInstance},
			loadbalancer.NewAlgorithmProvider(),
		},
		SignalFirstLoad: o.WaitFirstRouteLoad,
	})
	defer routing.Close()

	proxyFlags := proxy.Flags(o.ProxyOptions) | o.ProxyFlags
	proxyParams := proxy.Params{
		Routing:                  routing,
		Flags:                    proxyFlags,
		PriorityRoutes:           o.PriorityRoutes,
		IdleConnectionsPerHost:   o.IdleConnectionsPerHost,
		CloseIdleConnsPeriod:     o.CloseIdleConnsPeriod,
		FlushInterval:            o.BackendFlushInterval,
		ExperimentalUpgrade:      o.ExperimentalUpgrade,
		ExperimentalUpgradeAudit: o.ExperimentalUpgradeAudit,
		MaxLoopbacks:             o.MaxLoopbacks,
		DefaultHTTPStatus:        o.DefaultHTTPStatus,
		LoadBalancer:             lbInstance,
		Timeout:                  o.TimeoutBackend,
		ResponseHeaderTimeout:    o.ResponseHeaderTimeoutBackend,
		ExpectContinueTimeout:    o.ExpectContinueTimeoutBackend,
		KeepAlive:                o.KeepAliveBackend,
		DualStack:                o.DualStackBackend,
		TLSHandshakeTimeout:      o.TLSHandshakeTimeoutBackend,
		MaxIdleConns:             o.MaxIdleConnsBackend,
		AccessLogFilter:          parseAccessLogFilter(o.AccessLogDisabled, o.AccessLogFilter),
		ClientTLS:                o.ClientTLS,
	}

	var swarmer ratelimit.Swarmer
	var swops *swarm.Options
	var redisOptions *ratelimit.RedisOptions
	if o.EnableSwarm {
		if len(o.SwarmRedisURLs) > 0 {
			log.Infof("Redis based swarm with %d shards", len(o.SwarmRedisURLs))
			redisOptions = &ratelimit.RedisOptions{
				Addrs:        o.SwarmRedisURLs,
				ReadTimeout:  o.SwarmRedisReadTimeout,
				WriteTimeout: o.SwarmRedisWriteTimeout,
				PoolTimeout:  o.SwarmRedisPoolTimeout,
				MinIdleConns: o.SwarmRedisMinIdleConns,
				MaxIdleConns: o.SwarmRedisMaxIdleConns,
			}
		} else {
			log.Infof("Start swim based swarm")
			swops = &swarm.Options{
				SwarmPort:        uint16(o.SwarmPort),
				MaxMessageBuffer: o.SwarmMaxMessageBuffer,
				LeaveTimeout:     o.SwarmLeaveTimeout,
				Debug:            log.GetLevel() == log.DebugLevel,
			}

			if o.Kubernetes {
				swops.KubernetesOptions = &swarm.KubernetesOptions{
					KubernetesInCluster:  o.KubernetesInCluster,
					KubernetesAPIBaseURL: o.KubernetesURL,
					Namespace:            o.SwarmKubernetesNamespace,
					LabelSelectorKey:     o.SwarmKubernetesLabelSelectorKey,
					LabelSelectorValue:   o.SwarmKubernetesLabelSelectorValue,
				}
			}

			if o.SwarmStaticSelf != "" {
				self, err := swarm.NewStaticNodeInfo(o.SwarmStaticSelf, o.SwarmStaticSelf)
				if err != nil {
					log.Fatalf("Failed to get static NodeInfo: %v", err)
				}
				other := []*swarm.NodeInfo{self}

				for _, addr := range strings.Split(o.SwarmStaticOther, ",") {
					ni, err := swarm.NewStaticNodeInfo(addr, addr)
					if err != nil {
						log.Fatalf("Failed to get static NodeInfo: %v", err)
					}
					other = append(other, ni)
				}

				swops.StaticSwarm = swarm.NewStaticSwarm(self, other)
			}

			theSwarm, err := swarm.NewSwarm(swops)
			if err != nil {
				log.Errorf("failed to init swarm with options %+v: %v", swops, err)
			}
			defer theSwarm.Leave()
			swarmer = theSwarm
		}
	}

	if o.EnableRatelimiters || len(o.RatelimitSettings) > 0 {
		log.Infof("enabled ratelimiters %v: %v", o.EnableRatelimiters, o.RatelimitSettings)
		reg := ratelimit.NewSwarmRegistry(swarmer, redisOptions, o.RatelimitSettings...)
		defer reg.Close()
		proxyParams.RateLimiters = reg
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

		if o.EnablePrometheusMetrics {
			o.MetricsFlavours = append(o.MetricsFlavours, "prometheus")
		}
		metricsKind := metrics.UnkownKind
		for _, s := range o.MetricsFlavours {
			switch s {
			case "codahale":
				metricsKind |= metrics.CodaHaleKind
			case "prometheus":
				metricsKind |= metrics.PrometheusKind
			}
		}
		// set default if unset
		if metricsKind == metrics.UnkownKind {
			metricsKind = metrics.CodaHaleKind
		}
		log.Infof("Expose metrics in %s format", metricsKind)

		metricsHandler := metrics.NewDefaultHandler(metrics.Options{
			Format:                             metricsKind,
			Prefix:                             o.MetricsPrefix,
			EnableDebugGcMetrics:               o.EnableDebugGcMetrics,
			EnableRuntimeMetrics:               o.EnableRuntimeMetrics,
			EnableServeRouteMetrics:            o.EnableServeRouteMetrics,
			EnableServeHostMetrics:             o.EnableServeHostMetrics,
			EnableBackendHostMetrics:           o.EnableBackendHostMetrics,
			EnableProfile:                      o.EnableProfile,
			EnableAllFiltersMetrics:            o.EnableAllFiltersMetrics,
			EnableCombinedResponseMetrics:      o.EnableCombinedResponseMetrics,
			EnableRouteResponseMetrics:         o.EnableRouteResponseMetrics,
			EnableRouteBackendErrorsCounters:   o.EnableRouteBackendErrorsCounters,
			EnableRouteStreamingErrorsCounters: o.EnableRouteStreamingErrorsCounters,
			EnableRouteBackendMetrics:          o.EnableRouteBackendMetrics,
			UseExpDecaySample:                  o.MetricsUseExpDecaySample,
			HistogramBuckets:                   o.HistogramMetricBuckets,
			DisableCompatibilityDefaults:       o.DisableMetricsCompatibilityDefaults,
		})
		mux.Handle("/metrics", metricsHandler)
		mux.Handle("/metrics/", metricsHandler)
		mux.Handle("/debug/pprof", metricsHandler)
		mux.Handle("/debug/pprof/", metricsHandler)

		log.Infof("support listener on %s", supportListener)
		go func() {
			if err := http.ListenAndServe(supportListener, mux); err != nil {
				log.Errorf("Failed to start supportListener on %s: %v", supportListener, err)
			}
		}()
	} else {
		log.Infoln("Metrics are disabled")
	}

	o.PluginDirs = append(o.PluginDirs, o.PluginDir)
	if len(o.OpenTracing) > 0 {
		tracer, err := tracing.InitTracer(o.OpenTracing)
		if err != nil {
			return err
		}
		proxyParams.OpenTracer = tracer
		proxyParams.OpenTracingInitialSpan = o.OpenTracingInitialSpan
	} else {
		// always have a tracer available, so filter authors can rely on the
		// existence of a tracer
		proxyParams.OpenTracer, _ = tracing.LoadTracingPlugin(o.PluginDirs, []string{"noop"})
	}

	if proxyParams.OpenTracingInitialSpan != "" {
		proxyParams.OpenTracingInitialSpan = o.OpenTracingInitialSpan
	}

	// create the proxy
	proxy := proxy.WithParams(proxyParams)
	defer proxy.Close()

	// wait for the first route configuration to be loaded if enabled:
	<-routing.FirstLoad()

	return listenAndServe(proxy, &o)
}

func parseAccessLogFilter(accessLogDisabled bool, filter string) al.AccessLogFilter {
	if accessLogDisabled {
		return al.AccessLogFilter{Enable: false, Prefixes: nil}
	}
	if filter != "" {
		accessFilter := strings.Split(filter, ",")
		prefixes := make([]int, 0)
		for _, v := range accessFilter {
			statusCode, err := strconv.Atoi(v)
			if err == nil {
				prefixes = append(prefixes, statusCode)
			}
		}
		return al.AccessLogFilter{Enable: true, Prefixes: prefixes}
	}
	return al.AccessLogFilter{Enable: false, Prefixes: nil}
}
