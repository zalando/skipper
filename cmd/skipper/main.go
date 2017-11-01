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
	"github.com/zalando/skipper/proxy"
)

const (
	defaultAddress           = ":9090"
	defaultEtcdPrefix        = "/skipper"
	defaultSourcePollTimeout = int64(3000)
	defaultSupportListener   = ":9911"
	// deprecated
	defaultMetricsListener      = ":9911"
	defaultMetricsPrefix        = "skipper."
	defaultRuntimeMetrics       = true
	defaultApplicationLogPrefix = "[APP]"
	defaultApplicationLogLevel  = "INFO"
	defaultBackendFlushInterval = 20 * time.Millisecond
	defaultExperimentalUpgrade  = false

	addressUsage                   = "network address that skipper should listen on"
	etcdUrlsUsage                  = "urls of nodes in an etcd cluster, storing route definitions"
	etcdPrefixUsage                = "path prefix for skipper related data in etcd"
	kubernetesUsage                = "enables skipper to generate routes for ingress resources in kubernetes cluster"
	kubernetesInClusterUsage       = "specify if skipper is running inside kubernetes cluster"
	kubernetesURLUsage             = "kubernetes API base URL for the ingress data client; requires kubectl proxy running; omit if kubernetes-in-cluster is set to true"
	kubernetesHealthcheckUsage     = "automatic healthcheck route for internal IPs with path /kube-system/healthz; valid only with kubernetes"
	kubernetesHTTPSRedirectUsage   = "automatic HTTP->HTTPS redirect route; valid only with kubernetes"
	kubernetesIngressClassUsage    = "ingress class regular expression used to filter ingress resources for kubernetes"
	innkeeperURLUsage              = "API endpoint of the Innkeeper service, storing route definitions"
	innkeeperAuthTokenUsage        = "fixed token for innkeeper authentication"
	innkeeperPreRouteFiltersUsage  = "filters to be prepended to each route loaded from Innkeeper"
	innkeeperPostRouteFiltersUsage = "filters to be appended to each route loaded from Innkeeper"
	oauthURLUsage                  = "OAuth2 URL for Innkeeper authentication"
	oauthCredentialsDirUsage       = "directory where oauth credentials are stored: client.json and user.json"
	oauthScopeUsage                = "the whitespace separated list of oauth scopes"
	routesFileUsage                = "file containing static route definitions"
	inlineRoutesUsage              = "inline routes in eskip format"
	sourcePollTimeoutUsage         = "polling timeout of the routing data sources, in milliseconds"
	insecureUsage                  = "flag indicating to ignore the verification of the TLS certificates of the backend services"
	proxyPreserveHostUsage         = "flag indicating to preserve the incoming request 'Host' header in the outgoing requests"
	idleConnsPerHostUsage          = "maximum idle connections per backend host"
	closeIdleConnsPeriodUsage      = "period of closing all idle connections in seconds or as a duration string. Not closing when less than 0"
	devModeUsage                   = "enables developer time behavior, like ubuffered routing updates"
	supportListenerUsage           = "network address used for exposing the /metrics endpoint. An empty value disables support endpoint."
	metricsListenerUsage           = "network address used for exposing the /metrics endpoint. An empty value disables metrics iff support listener is also empty."
	metricsPrefixUsage             = "allows setting a custom path prefix for metrics export"
	enableProfileUsage             = "enable profile information on the metrics endpoint with path /pprof"
	debugGcMetricsUsage            = "enables reporting of the Go garbage collector statistics exported in debug.GCStats"
	runtimeMetricsUsage            = "enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats"
	serveRouteMetricsUsage         = "enables reporting total serve time metrics for each route"
	serveHostMetricsUsage          = "enables reporting total serve time metrics for each host"
	backendHostMetricsUsage        = "enables reporting total serve time metrics for each backend"
	allFiltersMetricsUsage         = "enables reporting combined filter metrics for each route"
	combinedResponseMetricsUsage   = "enables reporting combined response time metrics"
	routeResponseMetricsUsage      = "enables reporting response time metrics for each route"
	routeBackendErrorCountersUsage = "enables counting backend errors for each route"
	routeStreamErrorCountersUsage  = "enables counting streaming errors for each route"
	routeBackendMetricsUsage       = "enables reporting backend response time metrics for each route"
	metricsUseExpDecaySampleUsage  = "use exponentially decaying sample in metrics"
	disableMetricsCompatsUsage     = "disables the default true value for all-filters-metrics, route-response-metrics, route-backend-errorCounters and route-stream-error-counters"
	applicationLogUsage            = "output file for the application log. When not set, /dev/stderr is used"
	applicationLogLevelUsage       = "log level for application logs, possible values: PANIC, FATAL, ERROR, WARN, INFO, DEBUG"
	applicationLogPrefixUsage      = "prefix for each log entry"
	accessLogUsage                 = "output file for the access log, When not set, /dev/stderr is used"
	accessLogDisabledUsage         = "when this flag is set, no access log is printed"
	accessLogJSONEnabledUsage      = "when this flag is set, log in JSON format is used"
	debugEndpointUsage             = "when this address is set, skipper starts an additional listener returning the original and transformed requests"
	certPathTLSUsage               = "the path on the local filesystem to the certificate file (including any intermediates)"
	keyPathTLSUsage                = "the path on the local filesystem to the certificate's private key file"
	backendFlushIntervalUsage      = "flush interval for upgraded proxy connections"
	experimentalUpgradeUsage       = "enable experimental feature to handle upgrade protocol requests"
	versionUsage                   = "print Skipper version"
	maxLoopbacksUsage              = "maximum number of loopbacks for an incoming request, set to -1 to disable loopbacks"
	opentracingUsage               = "list of arguments for opentracing (space separated), first argument is the tracer implementation"
	defaultHTTPStatusUsage         = "default HTTP status used when no route is found for a request"
	pluginDirUsage                 = "set the directory to load plugins from, default is ./"
	suppressRouteUpdateLogsUsage   = "print only summaries on route updates/deletes"
)

var (
	version string
	commit  string
)

var (
	address                   string
	etcdUrls                  string
	etcdPrefix                string
	insecure                  bool
	proxyPreserveHost         bool
	idleConnsPerHost          int
	closeIdleConnsPeriod      string
	kubernetes                bool
	kubernetesInCluster       bool
	kubernetesURL             string
	kubernetesHealthcheck     bool
	kubernetesHTTPSRedirect   bool
	kubernetesIngressClass    string
	innkeeperURL              string
	sourcePollTimeout         int64
	routesFile                string
	inlineRoutes              string
	oauthURL                  string
	oauthScope                string
	oauthCredentialsDir       string
	innkeeperAuthToken        string
	innkeeperPreRouteFilters  string
	innkeeperPostRouteFilters string
	devMode                   bool
	supportListener           string
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
	debugListener             string
	certPathTLS               string
	keyPathTLS                string
	backendFlushInterval      time.Duration
	experimentalUpgrade       bool
	printVersion              bool
	maxLoopbacks              int
	enableBreakers            bool
	breakers                  breakerFlags
	enableRatelimiters        bool
	ratelimits                ratelimitFlags
	openTracing               string
	defaultHTTPStatus         int
	pluginDir                 string
	suppressRouteUpdateLogs   bool
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", "", etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.BoolVar(&proxyPreserveHost, "proxy-preserve-host", false, proxyPreserveHostUsage)
	flag.IntVar(&idleConnsPerHost, "idle-conns-num", proxy.DefaultIdleConnsPerHost, idleConnsPerHostUsage)
	flag.StringVar(&closeIdleConnsPeriod, "close-idle-conns-period", strconv.Itoa(int(proxy.DefaultCloseIdleConnsPeriod/time.Second)), closeIdleConnsPeriodUsage)
	flag.StringVar(&etcdPrefix, "etcd-prefix", defaultEtcdPrefix, etcdPrefixUsage)
	flag.BoolVar(&kubernetes, "kubernetes", false, kubernetesUsage)
	flag.BoolVar(&kubernetesInCluster, "kubernetes-in-cluster", false, kubernetesInClusterUsage)
	flag.StringVar(&kubernetesURL, "kubernetes-url", "", kubernetesURLUsage)
	flag.BoolVar(&kubernetesHealthcheck, "kubernetes-healthcheck", true, kubernetesHealthcheckUsage)
	flag.BoolVar(&kubernetesHTTPSRedirect, "kubernetes-https-redirect", true, kubernetesHTTPSRedirectUsage)
	flag.StringVar(&kubernetesIngressClass, "kubernetes-ingress-class", "", kubernetesIngressClassUsage)
	flag.StringVar(&innkeeperURL, "innkeeper-url", "", innkeeperURLUsage)
	flag.Int64Var(&sourcePollTimeout, "source-poll-timeout", defaultSourcePollTimeout, sourcePollTimeoutUsage)
	flag.StringVar(&routesFile, "routes-file", "", routesFileUsage)
	flag.StringVar(&inlineRoutes, "inline-routes", "", inlineRoutesUsage)
	flag.StringVar(&oauthURL, "oauth-url", "", oauthURLUsage)
	flag.StringVar(&oauthScope, "oauth-scope", "", oauthScopeUsage)
	flag.StringVar(&oauthCredentialsDir, "oauth-credentials-dir", "", oauthCredentialsDirUsage)
	flag.StringVar(&innkeeperAuthToken, "innkeeper-auth-token", "", innkeeperAuthTokenUsage)
	flag.StringVar(&innkeeperPreRouteFilters, "innkeeper-pre-route-filters", "", innkeeperPreRouteFiltersUsage)
	flag.StringVar(&innkeeperPostRouteFilters, "innkeeper-post-route-filters", "", innkeeperPostRouteFiltersUsage)
	flag.BoolVar(&devMode, "dev-mode", false, devModeUsage)
	flag.StringVar(&supportListener, "support-listener", defaultSupportListener, supportListenerUsage)
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
	flag.StringVar(&debugListener, "debug-listener", "", debugEndpointUsage)
	flag.StringVar(&certPathTLS, "tls-cert", "", certPathTLSUsage)
	flag.StringVar(&keyPathTLS, "tls-key", "", keyPathTLSUsage)
	flag.DurationVar(&backendFlushInterval, "backend-flush-interval", defaultBackendFlushInterval, backendFlushIntervalUsage)
	flag.BoolVar(&experimentalUpgrade, "experimental-upgrade", defaultExperimentalUpgrade, experimentalUpgradeUsage)
	flag.BoolVar(&printVersion, "version", false, versionUsage)
	flag.IntVar(&maxLoopbacks, "max-loopbacks", proxy.DefaultMaxLoopbacks, maxLoopbacksUsage)
	flag.BoolVar(&enableBreakers, "enable-breakers", false, enableBreakersUsage)
	flag.Var(&breakers, "breaker", breakerUsage)
	flag.BoolVar(&enableRatelimiters, "enable-ratelimits", false, enableRatelimitUsage)
	flag.Var(&ratelimits, "ratelimits", ratelimitUsage)
	flag.StringVar(&openTracing, "opentracing", "noop", opentracingUsage)
	flag.StringVar(&pluginDir, "plugindir", ".", pluginDirUsage)
	flag.IntVar(&defaultHTTPStatus, "default-http-status", http.StatusNotFound, defaultHTTPStatusUsage)
	flag.BoolVar(&suppressRouteUpdateLogs, "suppress-route-update-logs", false, suppressRouteUpdateLogsUsage)
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

	options := skipper.Options{
		Address:                             address,
		EtcdUrls:                            eus,
		EtcdPrefix:                          etcdPrefix,
		Kubernetes:                          kubernetes,
		KubernetesInCluster:                 kubernetesInCluster,
		KubernetesURL:                       kubernetesURL,
		KubernetesHealthcheck:               kubernetesHealthcheck,
		KubernetesHTTPSRedirect:             kubernetesHTTPSRedirect,
		KubernetesIngressClass:              kubernetesIngressClass,
		InnkeeperUrl:                        innkeeperURL,
		SourcePollTimeout:                   time.Duration(sourcePollTimeout) * time.Millisecond,
		RoutesFile:                          routesFile,
		InlineRoutes:                        inlineRoutes,
		IdleConnectionsPerHost:              idleConnsPerHost,
		CloseIdleConnsPeriod:                time.Duration(clsic) * time.Second,
		IgnoreTrailingSlash:                 false,
		OAuthUrl:                            oauthURL,
		OAuthScope:                          oauthScope,
		OAuthCredentialsDir:                 oauthCredentialsDir,
		InnkeeperAuthToken:                  innkeeperAuthToken,
		InnkeeperPreRouteFilters:            innkeeperPreRouteFilters,
		InnkeeperPostRouteFilters:           innkeeperPostRouteFilters,
		DevMode:                             devMode,
		MetricsListener:                     metricsListener,
		SupportListener:                     supportListener,
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
		DebugListener:                       debugListener,
		CertPathTLS:                         certPathTLS,
		KeyPathTLS:                          keyPathTLS,
		BackendFlushInterval:                backendFlushInterval,
		ExperimentalUpgrade:                 experimentalUpgrade,
		MaxLoopbacks:                        maxLoopbacks,
		EnableBreakers:                      enableBreakers,
		BreakerSettings:                     breakers,
		EnableRatelimiters:                  enableRatelimiters,
		RatelimitSettings:                   ratelimits,
		OpenTracing:                         strings.Split(openTracing, " "),
		PluginDir:                           pluginDir,
		DefaultHTTPStatus:                   defaultHTTPStatus,
		SuppressRouteUpdateLogs:             suppressRouteUpdateLogs,
	}

	if insecure {
		options.ProxyFlags |= proxy.Insecure
	}

	if proxyPreserveHost {
		options.ProxyFlags |= proxy.PreserveHost
	}

	log.Fatal(skipper.Run(options))
}
