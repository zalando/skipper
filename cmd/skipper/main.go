/*
This command provides an executable version of skipper with the default
set of filters.

For the list of command line options, run:

    skipper -help

For details about the usage and extensibility of skipper, please see the
documentation of the root skipper package.

To see which built-in filters are available, see the skipper/filters
package documentation.
*/
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/proxy"
)

const (
	// generic:
	defaultAddress                         = ":9090"
	defaultEtcdPrefix                      = "/skipper"
	defaultSourcePollTimeout               = int64(3000)
	defaultSupportListener                 = ":9911"
	defaultBackendFlushInterval            = 20 * time.Millisecond
	defaultExperimentalUpgrade             = false
	defaultLoadBalancerHealthCheckInterval = 0 // disabled
	defaultMaxAuditBody                    = 1024

	// metrics, logging:
	defaultMetricsListener      = ":9911" // deprecated
	defaultMetricsPrefix        = "skipper."
	defaultRuntimeMetrics       = true
	defaultApplicationLogPrefix = "[APP]"
	defaultApplicationLogLevel  = "INFO"

	// connections, timeouts:
	defaultReadTimeoutServer          = 5 * time.Minute
	defaultReadHeaderTimeoutServer    = 60 * time.Second
	defaultWriteTimeoutServer         = 60 * time.Second
	defaultIdleTimeoutServer          = 60 * time.Second
	defaultTimeoutBackend             = 60 * time.Second
	defaultKeepaliveBackend           = 30 * time.Second
	defaultTLSHandshakeTimeoutBackend = 60 * time.Second
	defaultMaxIdleConnsBackend        = 0

	// generic:
	addressUsage                         = "network address that skipper should listen on"
	ignoreTrailingSlashUsage             = "flag indicating to ignore trailing slashes in paths when routing"
	insecureUsage                        = "flag indicating to ignore the verification of the TLS certificates of the backend services"
	proxyPreserveHostUsage               = "flag indicating to preserve the incoming request 'Host' header in the outgoing requests"
	devModeUsage                         = "enables developer time behavior, like ubuffered routing updates"
	supportListenerUsage                 = "network address used for exposing the /metrics endpoint. An empty value disables support endpoint."
	debugEndpointUsage                   = "when this address is set, skipper starts an additional listener returning the original and transformed requests"
	certPathTLSUsage                     = "the path on the local filesystem to the certificate file(s) (including any intermediates), multiple may be given comma separated"
	keyPathTLSUsage                      = "the path on the local filesystem to the certificate's private key file(s), multiple keys may be given comma separated - the order must match the certs"
	versionUsage                         = "print Skipper version"
	maxLoopbacksUsage                    = "maximum number of loopbacks for an incoming request, set to -1 to disable loopbacks"
	defaultHTTPStatusUsage               = "default HTTP status used when no route is found for a request"
	pluginDirUsage                       = "set the directory to load plugins from, default is ./"
	loadBalancerHealthCheckIntervalUsage = "use to set the health checker interval to check healthiness of former dead or unhealthy routes"
	reverseSourcePredicateUsage          = "reverse the order of finding the client IP from X-Forwarded-For header"
	enableHopHeadersRemovalUsage         = "enables removal of Hop-Headers according to RFC-2616"
	maxAuditBodyUsage                    = "sets the max body to read to log inthe audit log body"

	// logging, metrics, tracing:
	enablePrometheusMetricsUsage    = "siwtch to Prometheus metrics format to expose metrics. *Deprecated*: use metrics-flavour"
	opentracingUsage                = "list of arguments for opentracing (space separated), first argument is the tracer implementation"
	opentracingIngressSpanNameUsage = "set the name of the initial, pre-routing, tracing span"
	metricsListenerUsage            = "network address used for exposing the /metrics endpoint. An empty value disables metrics iff support listener is also empty."
	metricsPrefixUsage              = "allows setting a custom path prefix for metrics export"
	enableProfileUsage              = "enable profile information on the metrics endpoint with path /pprof"
	debugGcMetricsUsage             = "enables reporting of the Go garbage collector statistics exported in debug.GCStats"
	runtimeMetricsUsage             = "enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats"
	serveRouteMetricsUsage          = "enables reporting total serve time metrics for each route"
	serveHostMetricsUsage           = "enables reporting total serve time metrics for each host"
	backendHostMetricsUsage         = "enables reporting total serve time metrics for each backend"
	allFiltersMetricsUsage          = "enables reporting combined filter metrics for each route"
	combinedResponseMetricsUsage    = "enables reporting combined response time metrics"
	routeResponseMetricsUsage       = "enables reporting response time metrics for each route"
	routeBackendErrorCountersUsage  = "enables counting backend errors for each route"
	routeStreamErrorCountersUsage   = "enables counting streaming errors for each route"
	routeBackendMetricsUsage        = "enables reporting backend response time metrics for each route"
	metricsUseExpDecaySampleUsage   = "use exponentially decaying sample in metrics"
	disableMetricsCompatsUsage      = "disables the default true value for all-filters-metrics, route-response-metrics, route-backend-errorCounters and route-stream-error-counters"
	applicationLogUsage             = "output file for the application log. When not set, /dev/stderr is used"
	applicationLogLevelUsage        = "log level for application logs, possible values: PANIC, FATAL, ERROR, WARN, INFO, DEBUG"
	applicationLogPrefixUsage       = "prefix for each log entry"
	accessLogUsage                  = "output file for the access log, When not set, /dev/stderr is used"
	accessLogDisabledUsage          = "when this flag is set, no access log is printed"
	accessLogJSONEnabledUsage       = "when this flag is set, log in JSON format is used"
	accessLogStripQueryUsage        = "when this flag is set, the access log strips the query strings from the access log"
	suppressRouteUpdateLogsUsage    = "print only summaries on route updates/deletes"

	// route sources:
	etcdUrlsUsage                  = "urls of nodes in an etcd cluster, storing route definitions"
	etcdPrefixUsage                = "path prefix for skipper related data in etcd"
	innkeeperURLUsage              = "API endpoint of the Innkeeper service, storing route definitions"
	innkeeperAuthTokenUsage        = "fixed token for innkeeper authentication"
	innkeeperPreRouteFiltersUsage  = "filters to be prepended to each route loaded from Innkeeper"
	innkeeperPostRouteFiltersUsage = "filters to be appended to each route loaded from Innkeeper"
	routesFileUsage                = "file containing route definitions"
	inlineRoutesUsage              = "inline routes in eskip format"
	sourcePollTimeoutUsage         = "polling timeout of the routing data sources, in milliseconds"

	// Kubernetes:
	kubernetesUsage                 = "enables skipper to generate routes for ingress resources in kubernetes cluster"
	kubernetesInClusterUsage        = "specify if skipper is running inside kubernetes cluster"
	kubernetesURLUsage              = "kubernetes API base URL for the ingress data client; requires kubectl proxy running; omit if kubernetes-in-cluster is set to true"
	kubernetesHealthcheckUsage      = "automatic healthcheck route for internal IPs with path /kube-system/healthz; valid only with kubernetes"
	kubernetesHTTPSRedirectUsage    = "automatic HTTP->HTTPS redirect route; valid only with kubernetes"
	kubernetesIngressClassUsage     = "ingress class regular expression used to filter ingress resources for kubernetes"
	whitelistedHealthCheckCIDRUsage = "sets the iprange/CIDRS to be whitelisted during healthcheck"
	kubernetesPathModeUsage         = "controls the default interpretation of Kubernetes ingress paths: kubernetes-ingress/path-regexp/path-prefix"

	// OAuth2:
	oauthURLUsage            = "OAuth2 URL for Innkeeper authentication"
	oauthCredentialsDirUsage = "directory where oauth credentials are stored: client.json and user.json"
	oauthScopeUsage          = "the whitespace separated list of oauth scopes"
	oauth2TokeninfoURLUsage  = "sets the default tokeninfo URL to query information about an incoming OAuth2 token in oauth2Tokeninfo filters"

	// connections, timeouts:
	idleConnsPerHostUsage           = "maximum idle connections per backend host"
	closeIdleConnsPeriodUsage       = "period of closing all idle connections in seconds or as a duration string. Not closing when less than 0"
	backendFlushIntervalUsage       = "flush interval for upgraded proxy connections"
	experimentalUpgradeUsage        = "enable experimental feature to handle upgrade protocol requests"
	readTimeoutServerUsage          = "set ReadTimeout for http server connections"
	readHeaderTimeoutServerUsage    = "set ReadHeaderTimeout for http server connections"
	writeTimeoutServerUsage         = "set WriteTimeout for http server connections"
	idleTimeoutServerUsage          = "set IdleTimeout for http server connections"
	maxHeaderBytesUsage             = "set MaxHeaderBytes for http server connections"
	enableConnMetricsServerUsage    = "enables connection metrics for http server connections"
	timeoutBackendUsage             = "sets the TCP client connection timeout for backend connections"
	keepaliveBackendUsage           = "sets the keepalive for backend connections"
	enableDualstackBackendUsage     = "enables DualStack for backend connections"
	tlsHandshakeTimeoutBackendUsage = "sets the TLS handshake timeout for backend connections"
	maxIdleConnsBackendUsage        = "sets the maximum idle connections for all backend connections"
)

var (
	version string
	commit  string

	// generic:
	address                         string
	ignoreTrailingSlash             bool
	insecure                        bool
	proxyPreserveHost               bool
	devMode                         bool
	supportListener                 string
	debugListener                   string
	certPathTLS                     string
	keyPathTLS                      string
	printVersion                    bool
	maxLoopbacks                    int
	defaultHTTPStatus               int
	pluginDir                       string
	loadBalancerHealthCheckInterval time.Duration
	reverseSourcePredicate          bool
	removeHopHeaders                bool
	maxAuditBody                    int
	enableBreakers                  bool
	breakers                        breakerFlags
	enableRatelimiters              bool
	ratelimits                      ratelimitFlags
	metricsFlavour                  metricsFlags
	filterPlugins                   pluginFlags
	predicatePlugins                pluginFlags
	dataclientPlugins               pluginFlags
	multiPlugins                    pluginFlags

	// logging, metrics, tracing:
	enablePrometheusMetrics   bool
	openTracing               string
	openTracingInitialSpan    string
	metricsListener           string
	metricsPrefix             string
	enableProfile             bool
	debugGcMetrics            bool
	runtimeMetrics            bool
	serveRouteMetrics         bool
	serveHostMetrics          bool
	backendHostMetrics        bool
	allFiltersMetrics         bool
	combinedResponseMetrics   bool
	routeResponseMetrics      bool
	routeBackendErrorCounters bool
	routeStreamErrorCounters  bool
	routeBackendMetrics       bool
	metricsUseExpDecaySample  bool
	disableMetricsCompat      bool
	applicationLog            string
	applicationLogLevel       string
	applicationLogPrefix      string
	accessLog                 string
	accessLogDisabled         bool
	accessLogJSONEnabled      bool
	accessLogStripQuery       bool
	suppressRouteUpdateLogs   bool

	// route sources:
	etcdUrls                  string
	etcdPrefix                string
	innkeeperURL              string
	innkeeperAuthToken        string
	innkeeperPreRouteFilters  string
	innkeeperPostRouteFilters string
	routesFile                string
	inlineRoutes              string
	sourcePollTimeout         int64

	// Kubernetes:
	kubernetesIngress          bool
	kubernetesInCluster        bool
	kubernetesURL              string
	kubernetesHealthcheck      bool
	kubernetesHTTPSRedirect    bool
	kubernetesIngressClass     string
	whitelistedHealthCheckCIDR string
	kubernetesPathModeString   string

	// OAuth2:
	oauthURL            string
	oauthScope          string
	oauthCredentialsDir string
	oauth2TokeninfoURL  string

	// connections, timeouts:
	idleConnsPerHost           int
	closeIdleConnsPeriod       string
	backendFlushInterval       time.Duration
	experimentalUpgrade        bool
	readTimeoutServer          time.Duration
	readHeaderTimeoutServer    time.Duration
	writeTimeoutServer         time.Duration
	idleTimeoutServer          time.Duration
	maxHeaderBytes             int
	enableConnMetricsServer    bool
	timeoutBackend             time.Duration
	keepaliveBackend           time.Duration
	enableDualstackBackend     bool
	tlsHandshakeTimeoutBackend time.Duration
	maxIdleConnsBackend        int
)

func init() {

	// generic:
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.BoolVar(&ignoreTrailingSlash, "ignore-trailing-slash", false, ignoreTrailingSlashUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.BoolVar(&proxyPreserveHost, "proxy-preserve-host", false, proxyPreserveHostUsage)
	flag.BoolVar(&devMode, "dev-mode", false, devModeUsage)
	flag.StringVar(&supportListener, "support-listener", defaultSupportListener, supportListenerUsage)
	flag.StringVar(&debugListener, "debug-listener", "", debugEndpointUsage)
	flag.StringVar(&certPathTLS, "tls-cert", "", certPathTLSUsage)
	flag.StringVar(&keyPathTLS, "tls-key", "", keyPathTLSUsage)
	flag.BoolVar(&printVersion, "version", false, versionUsage)
	flag.IntVar(&maxLoopbacks, "max-loopbacks", proxy.DefaultMaxLoopbacks, maxLoopbacksUsage)
	flag.IntVar(&defaultHTTPStatus, "default-http-status", http.StatusNotFound, defaultHTTPStatusUsage)
	flag.StringVar(&pluginDir, "plugindir", "", pluginDirUsage)
	flag.DurationVar(&loadBalancerHealthCheckInterval, "lb-healthcheck-interval", defaultLoadBalancerHealthCheckInterval, loadBalancerHealthCheckIntervalUsage)
	flag.BoolVar(&reverseSourcePredicate, "reverse-source-predicate", false, reverseSourcePredicateUsage)
	flag.BoolVar(&removeHopHeaders, "remove-hop-headers", false, enableHopHeadersRemovalUsage)
	flag.IntVar(&maxAuditBody, "max-audit-body", defaultMaxAuditBody, maxAuditBodyUsage)
	flag.BoolVar(&enableBreakers, "enable-breakers", false, enableBreakersUsage)
	flag.Var(&breakers, "breaker", breakerUsage)
	flag.BoolVar(&enableRatelimiters, "enable-ratelimits", false, enableRatelimitUsage)
	flag.Var(&ratelimits, "ratelimits", ratelimitUsage)
	flag.Var(&metricsFlavour, "metrics-flavour", metricsFlavourUsage)
	flag.Var(&filterPlugins, "filter-plugin", filterPluginUsage)
	flag.Var(&predicatePlugins, "predicate-plugin", predicatePluginUsage)
	flag.Var(&dataclientPlugins, "dataclient-plugin", dataclientPluginUsage)
	flag.Var(&multiPlugins, "multi-plugin", multiPluginUsage)

	// logging, metrics, tracing:
	flag.BoolVar(&enablePrometheusMetrics, "enable-prometheus-metrics", false, enablePrometheusMetricsUsage)
	flag.StringVar(&openTracing, "opentracing", "noop", opentracingUsage)
	flag.StringVar(&openTracingInitialSpan, "opentracing-initial-span", "ingress", opentracingIngressSpanNameUsage)
	flag.StringVar(&metricsListener, "metrics-listener", defaultMetricsListener, metricsListenerUsage)
	flag.StringVar(&metricsPrefix, "metrics-prefix", defaultMetricsPrefix, metricsPrefixUsage)
	flag.BoolVar(&enableProfile, "enable-profile", false, enableProfileUsage)
	flag.BoolVar(&debugGcMetrics, "debug-gc-metrics", false, debugGcMetricsUsage)
	flag.BoolVar(&runtimeMetrics, "runtime-metrics", defaultRuntimeMetrics, runtimeMetricsUsage)
	flag.BoolVar(&serveRouteMetrics, "serve-route-metrics", false, serveRouteMetricsUsage)
	flag.BoolVar(&serveHostMetrics, "serve-host-metrics", false, serveHostMetricsUsage)
	flag.BoolVar(&backendHostMetrics, "backend-host-metrics", false, backendHostMetricsUsage)
	flag.BoolVar(&allFiltersMetrics, "all-filters-metrics", false, allFiltersMetricsUsage)
	flag.BoolVar(&combinedResponseMetrics, "combined-response-metrics", false, combinedResponseMetricsUsage)
	flag.BoolVar(&routeResponseMetrics, "route-response-metrics", false, routeResponseMetricsUsage)
	flag.BoolVar(&routeBackendErrorCounters, "route-backend-error-counters", false, routeBackendErrorCountersUsage)
	flag.BoolVar(&routeStreamErrorCounters, "route-stream-error-counters", false, routeStreamErrorCountersUsage)
	flag.BoolVar(&routeBackendMetrics, "route-backend-metrics", false, routeBackendMetricsUsage)
	flag.BoolVar(&metricsUseExpDecaySample, "metrics-exp-decay-sample", false, metricsUseExpDecaySampleUsage)
	flag.BoolVar(&disableMetricsCompat, "disable-metrics-compat", false, disableMetricsCompatsUsage)
	flag.StringVar(&applicationLog, "application-log", "", applicationLogUsage)
	flag.StringVar(&applicationLogLevel, "application-log-level", defaultApplicationLogLevel, applicationLogLevelUsage)
	flag.StringVar(&applicationLogPrefix, "application-log-prefix", defaultApplicationLogPrefix, applicationLogPrefixUsage)
	flag.StringVar(&accessLog, "access-log", "", accessLogUsage)
	flag.BoolVar(&accessLogDisabled, "access-log-disabled", false, accessLogDisabledUsage)
	flag.BoolVar(&accessLogJSONEnabled, "access-log-json-enabled", false, accessLogJSONEnabledUsage)
	flag.BoolVar(&accessLogStripQuery, "access-log-strip-query", false, accessLogStripQueryUsage)
	flag.BoolVar(&suppressRouteUpdateLogs, "suppress-route-update-logs", false, suppressRouteUpdateLogsUsage)

	// route sources:
	flag.StringVar(&etcdUrls, "etcd-urls", "", etcdUrlsUsage)
	flag.StringVar(&etcdPrefix, "etcd-prefix", defaultEtcdPrefix, etcdPrefixUsage)
	flag.StringVar(&innkeeperURL, "innkeeper-url", "", innkeeperURLUsage)
	flag.StringVar(&innkeeperAuthToken, "innkeeper-auth-token", "", innkeeperAuthTokenUsage)
	flag.StringVar(&innkeeperPreRouteFilters, "innkeeper-pre-route-filters", "", innkeeperPreRouteFiltersUsage)
	flag.StringVar(&innkeeperPostRouteFilters, "innkeeper-post-route-filters", "", innkeeperPostRouteFiltersUsage)
	flag.StringVar(&routesFile, "routes-file", "", routesFileUsage)
	flag.StringVar(&inlineRoutes, "inline-routes", "", inlineRoutesUsage)
	flag.Int64Var(&sourcePollTimeout, "source-poll-timeout", defaultSourcePollTimeout, sourcePollTimeoutUsage)

	// Kubernetes:
	flag.BoolVar(&kubernetesIngress, "kubernetes", false, kubernetesUsage)
	flag.BoolVar(&kubernetesInCluster, "kubernetes-in-cluster", false, kubernetesInClusterUsage)
	flag.StringVar(&kubernetesURL, "kubernetes-url", "", kubernetesURLUsage)
	flag.BoolVar(&kubernetesHealthcheck, "kubernetes-healthcheck", true, kubernetesHealthcheckUsage)
	flag.BoolVar(&kubernetesHTTPSRedirect, "kubernetes-https-redirect", true, kubernetesHTTPSRedirectUsage)
	flag.StringVar(&kubernetesIngressClass, "kubernetes-ingress-class", "", kubernetesIngressClassUsage)
	flag.StringVar(&whitelistedHealthCheckCIDR, "whitelisted-healthcheck-cidr", "", whitelistedHealthCheckCIDRUsage)
	flag.StringVar(&kubernetesPathModeString, "kubernetes-path-mode", "kubernetes-ingress", kubernetesPathModeUsage)

	// OAuth2:
	flag.StringVar(&oauthURL, "oauth-url", "", oauthURLUsage)
	flag.StringVar(&oauthScope, "oauth-scope", "", oauthScopeUsage)
	flag.StringVar(&oauthCredentialsDir, "oauth-credentials-dir", "", oauthCredentialsDirUsage)
	flag.StringVar(&oauth2TokeninfoURL, "oauth2-tokeninfo-url", "", oauth2TokeninfoURLUsage)

	// connections, timeouts:
	flag.IntVar(&idleConnsPerHost, "idle-conns-num", proxy.DefaultIdleConnsPerHost, idleConnsPerHostUsage)
	flag.StringVar(&closeIdleConnsPeriod, "close-idle-conns-period", strconv.Itoa(int(proxy.DefaultCloseIdleConnsPeriod/time.Second)), closeIdleConnsPeriodUsage)
	flag.DurationVar(&backendFlushInterval, "backend-flush-interval", defaultBackendFlushInterval, backendFlushIntervalUsage)
	flag.BoolVar(&experimentalUpgrade, "experimental-upgrade", defaultExperimentalUpgrade, experimentalUpgradeUsage)
	flag.DurationVar(&readTimeoutServer, "read-timeout-server", defaultReadTimeoutServer, readTimeoutServerUsage)
	flag.DurationVar(&readHeaderTimeoutServer, "read-header-timeout-server", defaultReadHeaderTimeoutServer, readHeaderTimeoutServerUsage)
	flag.DurationVar(&writeTimeoutServer, "write-timeout-server", defaultWriteTimeoutServer, writeTimeoutServerUsage)
	flag.DurationVar(&idleTimeoutServer, "idle-timeout-server", defaultIdleTimeoutServer, idleConnsPerHostUsage)
	flag.IntVar(&maxHeaderBytes, "max-header-bytes", http.DefaultMaxHeaderBytes, maxHeaderBytesUsage)
	flag.BoolVar(&enableConnMetricsServer, "enable-connection-metrics", false, enableConnMetricsServerUsage)
	flag.DurationVar(&timeoutBackend, "timeout-backend", defaultTimeoutBackend, timeoutBackendUsage)
	flag.DurationVar(&keepaliveBackend, "keepalive-backend", defaultKeepaliveBackend, keepaliveBackendUsage)
	flag.BoolVar(&enableDualstackBackend, "enable-dualstack-backend", true, enableDualstackBackendUsage)
	flag.DurationVar(&tlsHandshakeTimeoutBackend, "tls-timeout-backend", defaultTLSHandshakeTimeoutBackend, tlsHandshakeTimeoutBackendUsage)
	flag.IntVar(&maxIdleConnsBackend, "max-idle-connection-backend", defaultMaxIdleConnsBackend, maxIdleConnsBackendUsage)

	flag.Parse()

	// check if arguments were correctly parsed.
	if len(flag.Args()) != 0 {
		log.Fatalf("Invalid arguments: %s", flag.Args())
	}
}

func parseDurationFlag(ds string) (time.Duration, error) {
	d, perr := time.ParseDuration(ds)
	if perr == nil {
		return d, nil
	}

	if i, serr := strconv.Atoi(ds); serr == nil {
		return time.Duration(i) * time.Second, nil
	}

	// returning the first parse error as more informative
	return 0, perr
}

func main() {
	if printVersion {
		fmt.Printf(
			"Skipper version %s (commit: %s)\n",
			version, commit,
		)

		return
	}

	if logLevel, err := log.ParseLevel(applicationLogLevel); err != nil {
		log.Fatal(err)
	} else {
		log.SetLevel(logLevel)
	}

	var eus []string
	if len(etcdUrls) > 0 {
		eus = strings.Split(etcdUrls, ",")
	}

	clsic, err := parseDurationFlag(closeIdleConnsPeriod)
	if err != nil {
		flag.PrintDefaults()
		os.Exit(2)
	}

	var whitelistCIDRS []string
	if len(whitelistedHealthCheckCIDR) > 0 {
		whitelistCIDRS = strings.Split(whitelistedHealthCheckCIDR, ",")
	}

	kubernetesPathMode, err := kubernetes.ParsePathMode(kubernetesPathModeString)
	if err != nil {
		flag.PrintDefaults()
		os.Exit(2)
	}

	options := skipper.Options{

		// generic:
		Address:                         address,
		IgnoreTrailingSlash:             ignoreTrailingSlash,
		DevMode:                         devMode,
		SupportListener:                 supportListener,
		DebugListener:                   debugListener,
		CertPathTLS:                     certPathTLS,
		KeyPathTLS:                      keyPathTLS,
		MaxLoopbacks:                    maxLoopbacks,
		DefaultHTTPStatus:               defaultHTTPStatus,
		LoadBalancerHealthCheckInterval: loadBalancerHealthCheckInterval,
		ReverseSourcePredicate:          reverseSourcePredicate,
		MaxAuditBody:                    maxAuditBody,
		EnableBreakers:                  enableBreakers,
		BreakerSettings:                 breakers,
		EnableRatelimiters:              enableRatelimiters,
		RatelimitSettings:               ratelimits,
		MetricsFlavours:                 metricsFlavour.Get(),
		FilterPlugins:                   filterPlugins.Get(),
		PredicatePlugins:                predicatePlugins.Get(),
		DataClientPlugins:               dataclientPlugins.Get(),
		Plugins:                         multiPlugins.Get(),
		PluginDirs:                      []string{skipper.DefaultPluginDir},

		// logging, metrics, tracing:
		EnablePrometheusMetrics:             enablePrometheusMetrics,
		OpenTracing:                         strings.Split(openTracing, " "),
		OpenTracingInitialSpan:              openTracingInitialSpan,
		MetricsListener:                     metricsListener,
		MetricsPrefix:                       metricsPrefix,
		EnableProfile:                       enableProfile,
		EnableDebugGcMetrics:                debugGcMetrics,
		EnableRuntimeMetrics:                runtimeMetrics,
		EnableServeRouteMetrics:             serveRouteMetrics,
		EnableServeHostMetrics:              serveHostMetrics,
		EnableBackendHostMetrics:            backendHostMetrics,
		EnableAllFiltersMetrics:             allFiltersMetrics,
		EnableCombinedResponseMetrics:       combinedResponseMetrics,
		EnableRouteResponseMetrics:          routeResponseMetrics,
		EnableRouteBackendErrorsCounters:    routeBackendErrorCounters,
		EnableRouteStreamingErrorsCounters:  routeStreamErrorCounters,
		EnableRouteBackendMetrics:           routeBackendMetrics,
		MetricsUseExpDecaySample:            metricsUseExpDecaySample,
		DisableMetricsCompatibilityDefaults: disableMetricsCompat,
		ApplicationLogOutput:                applicationLog,
		ApplicationLogPrefix:                applicationLogPrefix,
		AccessLogOutput:                     accessLog,
		AccessLogDisabled:                   accessLogDisabled,
		AccessLogJSONEnabled:                accessLogJSONEnabled,
		AccessLogStripQuery:                 accessLogStripQuery,
		SuppressRouteUpdateLogs:             suppressRouteUpdateLogs,

		// route sources:
		EtcdUrls:                  eus,
		EtcdPrefix:                etcdPrefix,
		InnkeeperUrl:              innkeeperURL,
		InnkeeperAuthToken:        innkeeperAuthToken,
		InnkeeperPreRouteFilters:  innkeeperPreRouteFilters,
		InnkeeperPostRouteFilters: innkeeperPostRouteFilters,
		WatchRoutesFile:           routesFile,
		InlineRoutes:              inlineRoutes,
		SourcePollTimeout:         time.Duration(sourcePollTimeout) * time.Millisecond,

		// Kubernetes:
		Kubernetes:                 kubernetesIngress,
		KubernetesInCluster:        kubernetesInCluster,
		KubernetesURL:              kubernetesURL,
		KubernetesHealthcheck:      kubernetesHealthcheck,
		KubernetesHTTPSRedirect:    kubernetesHTTPSRedirect,
		KubernetesIngressClass:     kubernetesIngressClass,
		WhitelistedHealthCheckCIDR: whitelistCIDRS,
		KubernetesPathMode:         kubernetesPathMode,

		// OAuth2:
		OAuthUrl:            oauthURL,
		OAuthScope:          oauthScope,
		OAuthCredentialsDir: oauthCredentialsDir,
		OAuthTokeninfoURL:   oauth2TokeninfoURL,

		// connections, timeouts:
		IdleConnectionsPerHost:     idleConnsPerHost,
		CloseIdleConnsPeriod:       time.Duration(clsic) * time.Second,
		BackendFlushInterval:       backendFlushInterval,
		ExperimentalUpgrade:        experimentalUpgrade,
		ReadTimeoutServer:          readTimeoutServer,
		ReadHeaderTimeoutServer:    readHeaderTimeoutServer,
		WriteTimeoutServer:         writeTimeoutServer,
		IdleTimeoutServer:          idleTimeoutServer,
		MaxHeaderBytes:             maxHeaderBytes,
		EnableConnMetricsServer:    enableConnMetricsServer,
		TimeoutBackend:             timeoutBackend,
		KeepAliveBackend:           keepaliveBackend,
		DualStackBackend:           enableDualstackBackend,
		TLSHandshakeTimeoutBackend: tlsHandshakeTimeoutBackend,
		MaxIdleConnsBackend:        maxIdleConnsBackend,
	}

	if pluginDir != "" {
		options.PluginDirs = append(options.PluginDirs, pluginDir)
	}

	if insecure {
		options.ProxyFlags |= proxy.Insecure
	}

	if proxyPreserveHost {
		options.ProxyFlags |= proxy.PreserveHost
	}

	if removeHopHeaders {
		options.ProxyFlags |= proxy.HopHeadersRemoval
	}

	log.Fatal(skipper.Run(options))
}
