package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/swarm"
)

type Config struct {
	ConfigFile string

	// generic:
	Address                         string         `yaml:"address"`
	EnableTCPQueue                  bool           `yaml:"enable-tcp-queue"`
	ExpectedBytesPerRequest         int            `yaml:"expected-bytes-per-request"`
	MaxTCPListenerConcurrency       int            `yaml:"max-tcp-listener-concurrency"`
	MaxTCPListenerQueue             int            `yaml:"max-tcp-listener-queue"`
	IgnoreTrailingSlash             bool           `yaml:"ignore-trailing-slash"`
	Insecure                        bool           `yaml:"insecure"`
	ProxyPreserveHost               bool           `yaml:"proxy-preserve-host"`
	DevMode                         bool           `yaml:"dev-mode"`
	SupportListener                 string         `yaml:"support-listener"`
	DebugListener                   string         `yaml:"debug-listener"`
	CertPathTLS                     string         `yaml:"tls-cert"`
	KeyPathTLS                      string         `yaml:"tls-key"`
	StatusChecks                    *listFlag      `yaml:"status-checks"`
	PrintVersion                    bool           `yaml:"version"`
	MaxLoopbacks                    int            `yaml:"max-loopbacks"`
	DefaultHTTPStatus               int            `yaml:"default-http-status"`
	PluginDir                       string         `yaml:"plugindir"`
	LoadBalancerHealthCheckInterval time.Duration  `yaml:"lb-healthcheck-interval"`
	ReverseSourcePredicate          bool           `yaml:"reverse-source-predicate"`
	RemoveHopHeaders                bool           `yaml:"remove-hop-headers"`
	RfcPatchPath                    bool           `yaml:"rfc-patch-path"`
	MaxAuditBody                    int            `yaml:"max-audit-body"`
	EnableBreakers                  bool           `yaml:"enable-breakers"`
	Breakers                        breakerFlags   `yaml:"breaker"`
	EnableRatelimiters              bool           `yaml:"enable-ratelimits"`
	Ratelimits                      ratelimitFlags `yaml:"ratelimits"`
	EnableRouteLIFOMetrics          bool           `yaml:"enable-route-lifo-metrics"`
	MetricsFlavour                  *listFlag      `yaml:"metrics-flavour"`
	FilterPlugins                   *pluginFlag    `yaml:"filter-plugin"`
	PredicatePlugins                *pluginFlag    `yaml:"predicate-plugin"`
	DataclientPlugins               *pluginFlag    `yaml:"dataclient-plugin"`
	MultiPlugins                    *pluginFlag    `yaml:"multi-plugin"`

	// logging, metrics, tracing:
	EnablePrometheusMetrics             bool      `yaml:"enable-prometheus-metrics"`
	OpenTracing                         string    `yaml:"opentracing"`
	OpenTracingInitialSpan              string    `yaml:"opentracing-initial-span"`
	OpenTracingExcludedProxyTags        string    `yaml:"opentracing-excluded-proxy-tags"`
	OpentracingLogFilterLifecycleEvents bool      `yaml:"opentracing-log-filter-lifecycle-events"`
	OpentracingLogStreamEvents          bool      `yaml:"opentracing-log-stream-events"`
	OpentracingBackendNameTag           bool      `yaml:"opentracing-backend-name-tag"`
	MetricsListener                     string    `yaml:"metrics-listener"`
	MetricsPrefix                       string    `yaml:"metrics-prefix"`
	EnableProfile                       bool      `yaml:"enable-profile"`
	DebugGcMetrics                      bool      `yaml:"debug-gc-metrics"`
	RuntimeMetrics                      bool      `yaml:"runtime-metrics"`
	ServeRouteMetrics                   bool      `yaml:"serve-route-metrics"`
	ServeHostMetrics                    bool      `yaml:"serve-host-metrics"`
	BackendHostMetrics                  bool      `yaml:"backend-host-metrics"`
	AllFiltersMetrics                   bool      `yaml:"all-filters-metrics"`
	CombinedResponseMetrics             bool      `yaml:"combined-response-metrics"`
	RouteResponseMetrics                bool      `yaml:"route-response-metrics"`
	RouteBackendErrorCounters           bool      `yaml:"route-backend-error-counters"`
	RouteStreamErrorCounters            bool      `yaml:"route-stream-error-counters"`
	RouteBackendMetrics                 bool      `yaml:"route-backend-metrics"`
	RouteCreationMetrics                bool      `yaml:"route-creation-metrics"`
	MetricsUseExpDecaySample            bool      `yaml:"metrics-exp-decay-sample"`
	HistogramMetricBucketsString        string    `yaml:"histogram-metric-buckets"`
	HistogramMetricBuckets              []float64 `yaml:"-"`
	DisableMetricsCompat                bool      `yaml:"disable-metrics-compat"`
	ApplicationLog                      string    `yaml:"application-log"`
	ApplicationLogLevel                 log.Level `yaml:"-"`
	ApplicationLogLevelString           string    `yaml:"application-log-level"`
	ApplicationLogPrefix                string    `yaml:"application-log-prefix"`
	ApplicationLogJSONEnabled           bool      `yaml:"application-log-json-enabled"`
	AccessLog                           string    `yaml:"access-log"`
	AccessLogDisabled                   bool      `yaml:"access-log-disabled"`
	AccessLogJSONEnabled                bool      `yaml:"access-log-json-enabled"`
	AccessLogStripQuery                 bool      `yaml:"access-log-strip-query"`
	SuppressRouteUpdateLogs             bool      `yaml:"suppress-route-update-logs"`

	// route sources:
	EtcdUrls                  string               `yaml:"etcd-urls"`
	EtcdPrefix                string               `yaml:"etcd-prefix"`
	EtcdTimeout               time.Duration        `yaml:"etcd-timeout"`
	EtcdInsecure              bool                 `yaml:"etcd-insecure"`
	EtcdOAuthToken            string               `yaml:"etcd-oauth-token"`
	EtcdUsername              string               `yaml:"etcd-username"`
	EtcdPassword              string               `yaml:"etcd-password"`
	InnkeeperURL              string               `yaml:"innkeeper-url"`
	InnkeeperAuthToken        string               `yaml:"innkeeper-auth-token"`
	InnkeeperPreRouteFilters  string               `yaml:"innkeeper-pre-route-filters"`
	InnkeeperPostRouteFilters string               `yaml:"innkeeper-post-route-filters"`
	RoutesFile                string               `yaml:"routes-file"`
	InlineRoutes              string               `yaml:"inline-routes"`
	AppendFilters             *defaultFiltersFlags `yaml:"default-filters-append"`
	PrependFilters            *defaultFiltersFlags `yaml:"default-filters-prepend"`
	SourcePollTimeout         int64                `yaml:"source-poll-timeout"`
	WaitFirstRouteLoad        bool                 `yaml:"wait-first-route-load"`

	// Kubernetes:
	KubernetesIngress                       bool                `yaml:"kubernetes"`
	KubernetesInCluster                     bool                `yaml:"kubernetes-in-cluster"`
	KubernetesURL                           string              `yaml:"kubernetes-url"`
	KubernetesHealthcheck                   bool                `yaml:"kubernetes-healthcheck"`
	KubernetesHTTPSRedirect                 bool                `yaml:"kubernetes-https-redirect"`
	KubernetesHTTPSRedirectCode             int                 `yaml:"kubernetes-https-redirect-code"`
	KubernetesIngressClass                  string              `yaml:"kubernetes-ingress-class"`
	KubernetesRouteGroupClass               string              `yaml:"kubernetes-routegroup-class"`
	WhitelistedHealthCheckCIDR              string              `yaml:"whitelisted-healthcheck-cidr"`
	KubernetesPathModeString                string              `yaml:"kubernetes-path-mode"`
	KubernetesPathMode                      kubernetes.PathMode `yaml:"-"`
	KubernetesNamespace                     string              `yaml:"kubernetes-namespace"`
	KubernetesEnableEastWest                bool                `yaml:"enable-kubernetes-east-west"`
	KubernetesEastWestDomain                string              `yaml:"kubernetes-east-west-domain"`
	KubernetesEastWestRangeDomains          *listFlag           `yaml:"kubernetes-east-west-range-domains"`
	KubernetesEastWestRangePredicatesString string              `yaml:"kubernetes-east-west-range-predicates"`
	KubernetesEastWestRangePredicates       []*eskip.Predicate  `yaml:"-"`

	// Default filters
	DefaultFiltersDir string `yaml:"default-filters-dir"`

	// Auth:
	EnableOAuth2GrantFlow           bool          `yaml:"enable-oauth2-grant-flow"`
	OauthURL                        string        `yaml:"oauth-url"`
	OauthScope                      string        `yaml:"oauth-scope"`
	OauthCredentialsDir             string        `yaml:"oauth-credentials-dir"`
	Oauth2AuthURL                   string        `yaml:"oauth2-auth-url"`
	Oauth2TokenURL                  string        `yaml:"oauth2-token-url"`
	Oauth2RevokeTokenURL            string        `yaml:"oauth2-revoke-token-url"`
	Oauth2TokeninfoURL              string        `yaml:"oauth2-tokeninfo-url"`
	Oauth2TokeninfoTimeout          time.Duration `yaml:"oauth2-tokeninfo-timeout"`
	Oauth2SecretFile                string        `yaml:"oauth2-secret-file"`
	Oauth2ClientID                  string        `yaml:"oauth2-client-id"`
	Oauth2ClientSecret              string        `yaml:"oauth2-client-secret"`
	Oauth2ClientIDFile              string        `yaml:"oauth2-client-id-file"`
	Oauth2ClientSecretFile          string        `yaml:"oauth2-client-secret-file"`
	Oauth2AuthURLParameters         mapFlags      `yaml:"oauth2-auth-url-parameters"`
	Oauth2CallbackPath              string        `yaml:"oauth2-callback-path"`
	Oauth2TokenintrospectionTimeout time.Duration `yaml:"oauth2-tokenintrospect-timeout"`
	Oauth2AccessTokenHeaderName     string        `yaml:"oauth2-access-token-header-name"`
	Oauth2TokeninfoSubjectKey       string        `yaml:"oauth2-tokeninfo-subject-key"`
	Oauth2TokenCookieName           string        `yaml:"oauth2-token-cookie-name"`
	WebhookTimeout                  time.Duration `yaml:"webhook-timeout"`
	OidcSecretsFile                 string        `yaml:"oidc-secrets-file"`
	CredentialPaths                 *listFlag     `yaml:"credentials-paths"`
	CredentialsUpdateInterval       time.Duration `yaml:"credentials-update-interval"`

	// TLS client certs
	ClientKeyFile  string            `yaml:"client-tls-key"`
	ClientCertFile string            `yaml:"client-tls-cert"`
	Certificates   []tls.Certificate `yaml:"-"`

	// TLS version
	TLSMinVersion string `yaml:"tls-min-version"`

	// API Monitoring
	ApiUsageMonitoringEnable                       bool   `yaml:"enable-api-usage-monitoring"`
	ApiUsageMonitoringRealmKeys                    string `yaml:"api-usage-monitoring-realm-keys"`
	ApiUsageMonitoringClientKeys                   string `yaml:"api-usage-monitoring-client-keys"`
	ApiUsageMonitoringDefaultClientTrackingPattern string `yaml:"api-usage-monitoring-default-client-tracking-pattern"`
	ApiUsageMonitoringRealmsTrackingPattern        string `yaml:"api-usage-monitoring-realms-tracking-pattern"`

	// connections, timeouts:
	WaitForHealthcheckInterval   time.Duration `yaml:"wait-for-healthcheck-interval"`
	IdleConnsPerHost             int           `yaml:"idle-conns-num"`
	CloseIdleConnsPeriod         time.Duration `yaml:"close-idle-conns-period"`
	BackendFlushInterval         time.Duration `yaml:"backend-flush-interval"`
	ExperimentalUpgrade          bool          `yaml:"experimental-upgrade"`
	ExperimentalUpgradeAudit     bool          `yaml:"experimental-upgrade-audit"`
	ReadTimeoutServer            time.Duration `yaml:"read-timeout-server"`
	ReadHeaderTimeoutServer      time.Duration `yaml:"read-header-timeout-server"`
	WriteTimeoutServer           time.Duration `yaml:"write-timeout-server"`
	IdleTimeoutServer            time.Duration `yaml:"idle-timeout-server"`
	MaxHeaderBytes               int           `yaml:"max-header-bytes"`
	EnableConnMetricsServer      bool          `yaml:"enable-connection-metrics"`
	TimeoutBackend               time.Duration `yaml:"timeout-backend"`
	KeepaliveBackend             time.Duration `yaml:"keepalive-backend"`
	EnableDualstackBackend       bool          `yaml:"enable-dualstack-backend"`
	TlsHandshakeTimeoutBackend   time.Duration `yaml:"tls-timeout-backend"`
	ResponseHeaderTimeoutBackend time.Duration `yaml:"response-header-timeout-backend"`
	ExpectContinueTimeoutBackend time.Duration `yaml:"expect-continue-timeout-backend"`
	MaxIdleConnsBackend          int           `yaml:"max-idle-connection-backend"`
	DisableHTTPKeepalives        bool          `yaml:"disable-http-keepalives"`

	// swarm:
	EnableSwarm bool `yaml:"enable-swarm"`
	// redis based
	SwarmRedisURLs         *listFlag     `yaml:"swarm-redis-urls"`
	SwarmRedisDialTimeout  time.Duration `yaml:"swarm-redis-dial-timeout"`
	SwarmRedisReadTimeout  time.Duration `yaml:"swarm-redis-read-timeout"`
	SwarmRedisWriteTimeout time.Duration `yaml:"swarm-redis-write-timeout"`
	SwarmRedisPoolTimeout  time.Duration `yaml:"swarm-redis-pool-timeout"`
	SwarmRedisMinConns     int           `yaml:"swarm-redis-min-conns"`
	SwarmRedisMaxConns     int           `yaml:"swarm-redis-max-conns"`
	// swim based
	SwarmKubernetesNamespace          string        `yaml:"swarm-namespace"`
	SwarmKubernetesLabelSelectorKey   string        `yaml:"swarm-label-selector-key"`
	SwarmKubernetesLabelSelectorValue string        `yaml:"swarm-label-selector-value"`
	SwarmPort                         int           `yaml:"swarm-port"`
	SwarmMaxMessageBuffer             int           `yaml:"swarm-max-msg-buffer"`
	SwarmLeaveTimeout                 time.Duration `yaml:"swarm-leave-timeout"`
	SwarmStaticSelf                   string        `yaml:"swarm-static-self"`
	SwarmStaticOther                  string        `yaml:"swarm-static-other"`
}

const (
	// generic:
	defaultAddress                         = ":9090"
	defaultExpectedBytesPerRequest         = 50 * 1024 // 50kB
	defaultEtcdPrefix                      = "/skipper"
	defaultEtcdTimeout                     = time.Second
	defaultSourcePollTimeout               = int64(3000)
	defaultSupportListener                 = ":9911"
	defaultBackendFlushInterval            = 20 * time.Millisecond
	defaultLoadBalancerHealthCheckInterval = 0 // disabled
	defaultMaxAuditBody                    = 1024

	// metrics, logging:
	defaultMetricsListener      = ":9911" // deprecated
	defaultMetricsPrefix        = "skipper."
	defaultApplicationLogPrefix = "[APP]"
	defaultApplicationLogLevel  = "INFO"

	// connections, timeouts:
	defaultWaitForHealthcheckInterval   = (10 + 5) * 3 * time.Second // kube-ingress-aws-controller default
	defaultReadTimeoutServer            = 5 * time.Minute
	defaultReadHeaderTimeoutServer      = 60 * time.Second
	defaultWriteTimeoutServer           = 60 * time.Second
	defaultIdleTimeoutServer            = 60 * time.Second
	defaultTimeoutBackend               = 60 * time.Second
	defaultKeepaliveBackend             = 30 * time.Second
	defaultTLSHandshakeTimeoutBackend   = 60 * time.Second
	defaultResponseHeaderTimeoutBackend = 60 * time.Second
	defaultExpectContinueTimeoutBackend = 30 * time.Second
	defaultMaxIdleConnsBackend          = 0

	// Auth:
	defaultOAuthTokeninfoTimeout          = 2 * time.Second
	defaultOAuthTokenintrospectionTimeout = 2 * time.Second
	defaultWebhookTimeout                 = 2 * time.Second
	defaultCredentialsUpdateInterval      = 10 * time.Minute

	// API Monitoring
	defaultApiUsageMonitoringRealmKeys                    = ""
	defaultApiUsageMonitoringClientKeys                   = "sub"
	defaultApiUsageMonitoringDefaultClientTrackingPattern = ""
	defaultApiUsageMonitoringRealmsTrackingPattern        = "services"

	// TLS
	defaultMinTLSVersion = "1.2"

	configFileUsage = "if provided the flags will be loaded/overwritten by the values on the file (yaml)"

	// generic:
	addressUsage                         = "network address that skipper should listen on"
	startupChecksUsage                   = "experimental URLs to check before reporting healthy on startup"
	enableTCPQueueUsage                  = "enable the TCP listener queue"
	expectedBytesPerRequestUsage         = "bytes per request, that is used to calculate concurrency limits to buffer connection spikes"
	maxTCPListenerConcurrencyUsage       = "sets hardcoded max for TCP listener concurrency, normally calculated based on available memory cgroups with max TODO"
	maxTCPListenerQueueUsage             = "sets hardcoded max queue size for TCP listener, normally calculated 10x concurrency with max TODO:50k"
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
	rfcPatchPathUsage                    = "patches the incoming request path to preserve uncoded reserved characters according to RFC 2616 and RFC 3986"
	maxAuditBodyUsage                    = "sets the max body to read to log in the audit log body"
	enableRouteLIFOMetricsUsage          = "enable metrics for the individual route LIFO queues"

	// logging, metrics, tracing:
	enablePrometheusMetricsUsage             = "switch to Prometheus metrics format to expose metrics. *Deprecated*: use metrics-flavour"
	opentracingUsage                         = "list of arguments for opentracing (space separated), first argument is the tracer implementation"
	opentracingIngressSpanNameUsage          = "set the name of the initial, pre-routing, tracing span"
	openTracingExcludedProxyTagsUsage        = "set tags that should be excluded from spans created for proxy operation. must be a comma-separated list of strings."
	opentracingLogFilterLifecycleEventsUsage = "enables the logs for request & response filters' lifecycle events that are marking start & end times."
	opentracingLogStreamEventsUsage          = "enables the logs for events marking the times response headers & payload are streamed to the client"
	opentracingBackendNameTag                = "enables an additional tracing tag that contains a backend name for a route when it's available  (e.g. for RouteGroups) (default false)"
	metricsListenerUsage                     = "network address used for exposing the /metrics endpoint. An empty value disables metrics iff support listener is also empty."
	metricsPrefixUsage                       = "allows setting a custom path prefix for metrics export"
	enableProfileUsage                       = "enable profile information on the metrics endpoint with path /pprof"
	debugGcMetricsUsage                      = "enables reporting of the Go garbage collector statistics exported in debug.GCStats"
	runtimeMetricsUsage                      = "enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats"
	serveRouteMetricsUsage                   = "enables reporting total serve time metrics for each route"
	serveHostMetricsUsage                    = "enables reporting total serve time metrics for each host"
	backendHostMetricsUsage                  = "enables reporting total serve time metrics for each backend"
	allFiltersMetricsUsage                   = "enables reporting combined filter metrics for each route"
	combinedResponseMetricsUsage             = "enables reporting combined response time metrics"
	routeResponseMetricsUsage                = "enables reporting response time metrics for each route"
	routeBackendErrorCountersUsage           = "enables counting backend errors for each route"
	routeStreamErrorCountersUsage            = "enables counting streaming errors for each route"
	routeBackendMetricsUsage                 = "enables reporting backend response time metrics for each route"
	routeCreationMetricsUsage                = "enables reporting for route creation times"
	metricsFlavourUsage                      = "Metrics flavour is used to change the exposed metrics format. Supported metric formats: 'codahale' and 'prometheus', you can select both of them"
	metricsUseExpDecaySampleUsage            = "use exponentially decaying sample in metrics"
	histogramMetricBucketsUsage              = "use custom buckets for prometheus histograms, must be a comma-separated list of numbers"
	disableMetricsCompatsUsage               = "disables the default true value for all-filters-metrics, route-response-metrics, route-backend-errorCounters and route-stream-error-counters"
	applicationLogUsage                      = "output file for the application log. When not set, /dev/stderr is used"
	applicationLogLevelUsage                 = "log level for application logs, possible values: PANIC, FATAL, ERROR, WARN, INFO, DEBUG"
	applicationLogPrefixUsage                = "prefix for each log entry"
	applicationLogJSONEnabledUsage           = "when this flag is set, log in JSON format is used"
	accessLogUsage                           = "output file for the access log, When not set, /dev/stderr is used"
	accessLogDisabledUsage                   = "when this flag is set, no access log is printed"
	accessLogJSONEnabledUsage                = "when this flag is set, log in JSON format is used"
	accessLogStripQueryUsage                 = "when this flag is set, the access log strips the query strings from the access log"
	suppressRouteUpdateLogsUsage             = "print only summaries on route updates/deletes"

	// route sources:
	etcdUrlsUsage                  = "urls of nodes in an etcd cluster, storing route definitions"
	etcdPrefixUsage                = "path prefix for skipper related data in etcd"
	etcdTimeoutUsage               = "http client timeout duration for etcd"
	etcdInsecureUsage              = "ignore the verification of TLS certificates for etcd"
	etcdOAuthTokenUsage            = "optional token for OAuth authentication with etcd"
	etcdUsernameUsage              = "optional username for basic authentication with etcd"
	etcdPasswordUsage              = "optional password for basic authentication with etcd"
	innkeeperURLUsage              = "API endpoint of the Innkeeper service, storing route definitions"
	innkeeperAuthTokenUsage        = "fixed token for innkeeper authentication"
	innkeeperPreRouteFiltersUsage  = "filters to be prepended to each route loaded from Innkeeper"
	innkeeperPostRouteFiltersUsage = "filters to be appended to each route loaded from Innkeeper"
	routesFileUsage                = "file containing route definitions"
	inlineRoutesUsage              = "inline routes in eskip format"
	sourcePollTimeoutUsage         = "polling timeout of the routing data sources, in milliseconds"
	waitFirstRouteLoadUsage        = "prevent starting the listener before the first batch of routes were loaded"

	// Kubernetes:
	kubernetesUsage                        = "enables skipper to generate routes for ingress resources in kubernetes cluster"
	kubernetesInClusterUsage               = "specify if skipper is running inside kubernetes cluster"
	kubernetesURLUsage                     = "kubernetes API base URL for the ingress data client; requires kubectl proxy running; omit if kubernetes-in-cluster is set to true"
	kubernetesHealthcheckUsage             = "automatic healthcheck route for internal IPs with path /kube-system/healthz; valid only with kubernetes"
	kubernetesHTTPSRedirectUsage           = "automatic HTTP->HTTPS redirect route; valid only with kubernetes"
	kubernetesHTTPSRedirectCodeUsage       = "overrides the default redirect code (308) when used together with -kubernetes-https-redirect"
	kubernetesIngressClassUsage            = "ingress class regular expression used to filter ingress resources for kubernetes"
	kubernetesRouteGroupClassUsage         = "route group class regular expression used to filter route group resources for kubernetes"
	whitelistedHealthCheckCIDRUsage        = "sets the iprange/CIDRS to be whitelisted during healthcheck"
	kubernetesPathModeUsage                = "controls the default interpretation of Kubernetes ingress paths: <kubernetes-ingress|path-regexp|path-prefix>"
	kubernetesNamespaceUsage               = "watch only this namespace for ingresses"
	kubernetesEnableEastWestUsage          = "*Deprecated*: use -kubernetes-east-west-range feature. Enables east-west communication, which automatically adds routes for Ingress objects with hostname <name>.<namespace>.skipper.cluster.local"
	kubernetesEastWestDomainUsage          = "set the east-west domain. *Deprecated*: use -kubernetes-east-west-range feature. Defaults to .skipper.cluster.local"
	kubernetesEastWestRangeDomainsUsage    = "set the the cluster internal domains for east west traffic. Identified routes to such domains will include the -kubernetes-east-west-range-predicates"
	kubernetesEastWestRangePredicatesUsage = "set the predicates that will be appended to routes identified as to -kubernetes-east-west-range-domains"

	// Auth:
	oauth2GrantFlowEnableUsage           = "enables OAuth2 Grant Flow filter"
	oauthURLUsage                        = "OAuth2 URL for Innkeeper authentication"
	oauthCredentialsDirUsage             = "directory where oauth credentials are stored: client.json and user.json"
	oauthScopeUsage                      = "the whitespace separated list of oauth scopes"
	oauth2AuthURLUsage                   = "sets the OAuth2 Auth URL to redirect the requests to when login is required"
	oauth2TokenURLUsage                  = "the url where the access code should be exchanged for the access token"
	oauth2RevokeTokenURLUsage            = "the url where the access and refresh tokens can be revoked when logging out"
	oauth2TokeninfoURLUsage              = "sets the default tokeninfo URL to query information about an incoming OAuth2 token in oauth2Tokeninfo filters"
	oauth2TokeninfoTimeoutUsage          = "sets the default tokeninfo request timeout duration to 2000ms"
	oauth2SecretFileUsage                = "sets the filename with the encryption key for the authentication cookie and grant flow state stored in secrets registry"
	oauth2ClientIDUsage                  = "sets the OAuth2 client id of the current service, used to exchange the access code"
	oauth2ClientSecretUsage              = "sets the OAuth2 client secret associated with the oauth2-client-id, used to exchange the access code"
	oauth2ClientIDFileUsage              = "sets the path of the file containing the OAuth2 client id of the current service, used to exchange the access code"
	oauth2ClientSecretFileUsage          = "sets the path of the file containing the OAuth2 client secret associated with the oauth2-client-id, used to exchange the access code"
	oauth2CallbackPathUsage              = "sets the path where the OAuth2 callback requests with the authorization code should be redirected to"
	oauth2TokenintrospectionTimeoutUsage = "sets the default tokenintrospection request timeout duration to 2000ms"
	oauth2AuthURLParametersUsage         = "sets additional parameters to send when calling the OAuth2 authorize or token endpoints as key-value pairs"
	oauth2AccessTokenHeaderNameUsage     = "sets the access token to a header on the request with this name"
	oauth2TokeninfoSubjectKeyUsage       = "the key containing the subject ID in the tokeninfo map"
	oauth2TokenCookieNameUsage           = "sets the name of the cookie where the encrypted token is stored"
	webhookTimeoutUsage                  = "sets the webhook request timeout duration, defaults to 2s"
	oidcSecretsFileUsage                 = "file storing the encryption key of the OID Connect token"
	credentialPathsUsage                 = "directories or files to watch for credentials to use by bearerinjector filter"
	credentialsUpdateIntervalUsage       = "sets the interval to update secrets"

	// TLS client certs
	clientKeyFileUsage  = "TLS Key file for backend connections, multiple keys may be given comma separated - the order must match the certs"
	clientCertFileUsage = "TLS certificate files for backend connections, multiple keys may be given comma separated - the order must match the keys"

	// TLS version
	minTLSVersionUsage = "minimal TLS Version to be used in server, proxy and client connections"

	// API Monitoring:
	apiUsageMonitoringEnableUsage                       = "enables the apiUsageMonitoring filter"
	apiUsageMonitoringRealmKeysUsage                    = "name of the property in the JWT payload that contains the authority realm"
	apiUsageMonitoringClientKeysUsage                   = "comma separated list of names of the properties in the JWT body that contains the client ID"
	apiUsageMonitoringDefaultClientTrackingPatternUsage = "*Deprecated*: set `client_tracking_pattern` directly on filter"
	apiUsageMonitoringRealmsTrackingPatternUsage        = "regular expression used for matching monitored realms (defaults is 'services')"

	// Default filters
	defaultFiltersDirUsage = "path to directory which contains default filter configurations per service and namespace (disabled if not set)"

	// connections, timeouts:
	waitForHealthcheckIntervalUsage   = "period waiting to become unhealthy in the loadbalancer pool in front of this instance, before shutdown triggered by SIGINT or SIGTERM"
	idleConnsPerHostUsage             = "maximum idle connections per backend host"
	closeIdleConnsPeriodUsage         = "sets the time interval of closing all idle connections. Not closing when 0"
	backendFlushIntervalUsage         = "flush interval for upgraded proxy connections"
	experimentalUpgradeUsage          = "enable experimental feature to handle upgrade protocol requests"
	experimentalUpgradeAuditUsage     = "enable audit logging of the request line and the messages during the experimental web socket upgrades"
	readTimeoutServerUsage            = "set ReadTimeout for http server connections"
	readHeaderTimeoutServerUsage      = "set ReadHeaderTimeout for http server connections"
	writeTimeoutServerUsage           = "set WriteTimeout for http server connections"
	idleTimeoutServerUsage            = "set IdleTimeout for http server connections"
	maxHeaderBytesUsage               = "set MaxHeaderBytes for http server connections"
	enableConnMetricsServerUsage      = "enables connection metrics for http server connections"
	timeoutBackendUsage               = "sets the TCP client connection timeout for backend connections"
	keepaliveBackendUsage             = "sets the keepalive for backend connections"
	enableDualstackBackendUsage       = "enables DualStack for backend connections"
	tlsHandshakeTimeoutBackendUsage   = "sets the TLS handshake timeout for backend connections"
	responseHeaderTimeoutBackendUsage = "sets the HTTP response header timeout for backend connections"
	expectContinueTimeoutBackendUsage = "sets the HTTP expect continue timeout for backend connections"
	maxIdleConnsBackendUsage          = "sets the maximum idle connections for all backend connections"
	disableHTTPKeepalivesUsage        = "forces backend to always create a new connection"

	// swarm:
	enableSwarmUsage                       = "enable swarm communication between nodes in a skipper fleet"
	swarmKubernetesNamespaceUsage          = "Kubernetes namespace to find swarm peer instances"
	swarmKubernetesLabelSelectorKeyUsage   = "Kubernetes labelselector key to find swarm peer instances"
	swarmKubernetesLabelSelectorValueUsage = "Kubernetes labelselector value to find swarm peer instances"
	swarmPortUsage                         = "swarm port to use to communicate with our peers"
	swarmMaxMessageBufferUsage             = "swarm max message buffer size to use for member list messages"
	swarmLeaveTimeoutUsage                 = "swarm leave timeout to use for leaving the memberlist on timeout"
	swarmRedisURLsUsage                    = "Redis URLs as comma separated list, used for building a swarm, for example in redis based cluster ratelimits"
	swarmStaticSelfUsage                   = "set static swarm self node, for example 127.0.0.1:9001"
	swarmStaticOtherUsage                  = "set static swarm all nodes, for example 127.0.0.1:9002,127.0.0.1:9003"
	swarmRedisDialTimeoutUsage             = "set redis client dial timeout"
	swarmRedisReadTimeoutUsage             = "set redis socket read timeout"
	swarmRedisWriteTimeoutUsage            = "set redis socket write timeout"
	swarmRedisPoolTimeoutUsage             = "set redis get connection from pool timeout"
	swarmRedisMaxConnsUsage                = "set max number of connections to redis"
	swarmRedisMinConnsUsage                = "set min number of connections to redis"
)

func NewConfig() *Config {
	cfg := new(Config)
	cfg.MetricsFlavour = commaListFlag("codahale", "prometheus")
	cfg.StatusChecks = commaListFlag()
	cfg.FilterPlugins = newPluginFlag()
	cfg.PredicatePlugins = newPluginFlag()
	cfg.DataclientPlugins = newPluginFlag()
	cfg.MultiPlugins = newPluginFlag()
	cfg.CredentialPaths = commaListFlag()
	cfg.SwarmRedisURLs = commaListFlag()
	cfg.AppendFilters = &defaultFiltersFlags{}
	cfg.PrependFilters = &defaultFiltersFlags{}
	cfg.KubernetesEastWestRangeDomains = commaListFlag()

	flag.StringVar(&cfg.ConfigFile, "config-file", "", configFileUsage)

	// generic:
	flag.StringVar(&cfg.Address, "address", defaultAddress, addressUsage)
	flag.BoolVar(&cfg.EnableTCPQueue, "enable-tcp-queue", false, enableTCPQueueUsage)
	flag.IntVar(&cfg.ExpectedBytesPerRequest, "expected-bytes-per-request", defaultExpectedBytesPerRequest, expectedBytesPerRequestUsage)
	flag.IntVar(&cfg.MaxTCPListenerConcurrency, "max-tcp-listener-concurrency", 0, maxTCPListenerConcurrencyUsage)
	flag.IntVar(&cfg.MaxTCPListenerQueue, "max-tcp-listener-queue", 0, maxTCPListenerQueueUsage)
	flag.BoolVar(&cfg.IgnoreTrailingSlash, "ignore-trailing-slash", false, ignoreTrailingSlashUsage)
	flag.BoolVar(&cfg.Insecure, "insecure", false, insecureUsage)
	flag.BoolVar(&cfg.ProxyPreserveHost, "proxy-preserve-host", false, proxyPreserveHostUsage)
	flag.BoolVar(&cfg.DevMode, "dev-mode", false, devModeUsage)
	flag.StringVar(&cfg.SupportListener, "support-listener", defaultSupportListener, supportListenerUsage)
	flag.StringVar(&cfg.DebugListener, "debug-listener", "", debugEndpointUsage)
	flag.StringVar(&cfg.CertPathTLS, "tls-cert", "", certPathTLSUsage)
	flag.StringVar(&cfg.KeyPathTLS, "tls-key", "", keyPathTLSUsage)
	flag.Var(cfg.StatusChecks, "status-checks", startupChecksUsage)
	flag.BoolVar(&cfg.PrintVersion, "version", false, versionUsage)
	flag.IntVar(&cfg.MaxLoopbacks, "max-loopbacks", proxy.DefaultMaxLoopbacks, maxLoopbacksUsage)
	flag.IntVar(&cfg.DefaultHTTPStatus, "default-http-status", http.StatusNotFound, defaultHTTPStatusUsage)
	flag.StringVar(&cfg.PluginDir, "plugindir", "", pluginDirUsage)
	flag.DurationVar(&cfg.LoadBalancerHealthCheckInterval, "lb-healthcheck-interval", defaultLoadBalancerHealthCheckInterval, loadBalancerHealthCheckIntervalUsage)
	flag.BoolVar(&cfg.ReverseSourcePredicate, "reverse-source-predicate", false, reverseSourcePredicateUsage)
	flag.BoolVar(&cfg.RemoveHopHeaders, "remove-hop-headers", false, enableHopHeadersRemovalUsage)
	flag.BoolVar(&cfg.RfcPatchPath, "rfc-patch-path", false, rfcPatchPathUsage)
	flag.IntVar(&cfg.MaxAuditBody, "max-audit-body", defaultMaxAuditBody, maxAuditBodyUsage)
	flag.BoolVar(&cfg.EnableBreakers, "enable-breakers", false, enableBreakersUsage)
	flag.Var(&cfg.Breakers, "breaker", breakerUsage)
	flag.BoolVar(&cfg.EnableRatelimiters, "enable-ratelimits", false, enableRatelimitsUsage)
	flag.Var(&cfg.Ratelimits, "ratelimits", ratelimitsUsage)
	flag.BoolVar(&cfg.EnableRouteLIFOMetrics, "enable-route-lifo-metrics", false, enableRouteLIFOMetricsUsage)
	flag.Var(cfg.MetricsFlavour, "metrics-flavour", metricsFlavourUsage)
	flag.Var(cfg.FilterPlugins, "filter-plugin", filterPluginUsage)
	flag.Var(cfg.PredicatePlugins, "predicate-plugin", predicatePluginUsage)
	flag.Var(cfg.DataclientPlugins, "dataclient-plugin", dataclientPluginUsage)
	flag.Var(cfg.MultiPlugins, "multi-plugin", multiPluginUsage)

	// logging, metrics, tracing:
	flag.BoolVar(&cfg.EnablePrometheusMetrics, "enable-prometheus-metrics", false, enablePrometheusMetricsUsage)
	flag.StringVar(&cfg.OpenTracing, "opentracing", "noop", opentracingUsage)
	flag.StringVar(&cfg.OpenTracingInitialSpan, "opentracing-initial-span", "ingress", opentracingIngressSpanNameUsage)
	flag.StringVar(&cfg.OpenTracingExcludedProxyTags, "opentracing-excluded-proxy-tags", "", openTracingExcludedProxyTagsUsage)
	flag.BoolVar(&cfg.OpentracingLogFilterLifecycleEvents, "opentracing-log-filter-lifecycle-events", true, opentracingLogFilterLifecycleEventsUsage)
	flag.BoolVar(&cfg.OpentracingLogStreamEvents, "opentracing-log-stream-events", true, opentracingLogStreamEventsUsage)
	flag.BoolVar(&cfg.OpentracingBackendNameTag, "opentracing-backend-name-tag", false, opentracingBackendNameTag)
	flag.StringVar(&cfg.MetricsListener, "metrics-listener", defaultMetricsListener, metricsListenerUsage)
	flag.StringVar(&cfg.MetricsPrefix, "metrics-prefix", defaultMetricsPrefix, metricsPrefixUsage)
	flag.BoolVar(&cfg.EnableProfile, "enable-profile", false, enableProfileUsage)
	flag.BoolVar(&cfg.DebugGcMetrics, "debug-gc-metrics", false, debugGcMetricsUsage)
	flag.BoolVar(&cfg.RuntimeMetrics, "runtime-metrics", true, runtimeMetricsUsage)
	flag.BoolVar(&cfg.ServeRouteMetrics, "serve-route-metrics", false, serveRouteMetricsUsage)
	flag.BoolVar(&cfg.ServeHostMetrics, "serve-host-metrics", false, serveHostMetricsUsage)
	flag.BoolVar(&cfg.BackendHostMetrics, "backend-host-metrics", false, backendHostMetricsUsage)
	flag.BoolVar(&cfg.AllFiltersMetrics, "all-filters-metrics", false, allFiltersMetricsUsage)
	flag.BoolVar(&cfg.CombinedResponseMetrics, "combined-response-metrics", false, combinedResponseMetricsUsage)
	flag.BoolVar(&cfg.RouteResponseMetrics, "route-response-metrics", false, routeResponseMetricsUsage)
	flag.BoolVar(&cfg.RouteBackendErrorCounters, "route-backend-error-counters", false, routeBackendErrorCountersUsage)
	flag.BoolVar(&cfg.RouteStreamErrorCounters, "route-stream-error-counters", false, routeStreamErrorCountersUsage)
	flag.BoolVar(&cfg.RouteBackendMetrics, "route-backend-metrics", false, routeBackendMetricsUsage)
	flag.BoolVar(&cfg.RouteCreationMetrics, "route-creation-metrics", false, routeCreationMetricsUsage)
	flag.BoolVar(&cfg.MetricsUseExpDecaySample, "metrics-exp-decay-sample", false, metricsUseExpDecaySampleUsage)
	flag.StringVar(&cfg.HistogramMetricBucketsString, "histogram-metric-buckets", "", histogramMetricBucketsUsage)
	flag.BoolVar(&cfg.DisableMetricsCompat, "disable-metrics-compat", false, disableMetricsCompatsUsage)
	flag.StringVar(&cfg.ApplicationLog, "application-log", "", applicationLogUsage)
	flag.StringVar(&cfg.ApplicationLogLevelString, "application-log-level", defaultApplicationLogLevel, applicationLogLevelUsage)
	flag.StringVar(&cfg.ApplicationLogPrefix, "application-log-prefix", defaultApplicationLogPrefix, applicationLogPrefixUsage)
	flag.BoolVar(&cfg.ApplicationLogJSONEnabled, "application-log-json-enabled", false, applicationLogJSONEnabledUsage)
	flag.StringVar(&cfg.AccessLog, "access-log", "", accessLogUsage)
	flag.BoolVar(&cfg.AccessLogDisabled, "access-log-disabled", false, accessLogDisabledUsage)
	flag.BoolVar(&cfg.AccessLogJSONEnabled, "access-log-json-enabled", false, accessLogJSONEnabledUsage)
	flag.BoolVar(&cfg.AccessLogStripQuery, "access-log-strip-query", false, accessLogStripQueryUsage)
	flag.BoolVar(&cfg.SuppressRouteUpdateLogs, "suppress-route-update-logs", false, suppressRouteUpdateLogsUsage)

	// route sources:
	flag.StringVar(&cfg.EtcdUrls, "etcd-urls", "", etcdUrlsUsage)
	flag.StringVar(&cfg.EtcdPrefix, "etcd-prefix", defaultEtcdPrefix, etcdPrefixUsage)
	flag.DurationVar(&cfg.EtcdTimeout, "etcd-timeout", defaultEtcdTimeout, etcdTimeoutUsage)
	flag.BoolVar(&cfg.EtcdInsecure, "etcd-insecure", false, etcdInsecureUsage)
	flag.StringVar(&cfg.EtcdOAuthToken, "etcd-oauth-token", "", etcdOAuthTokenUsage)
	flag.StringVar(&cfg.EtcdUsername, "etcd-username", "", etcdUsernameUsage)
	flag.StringVar(&cfg.EtcdPassword, "etcd-password", "", etcdPasswordUsage)
	flag.StringVar(&cfg.InnkeeperURL, "innkeeper-url", "", innkeeperURLUsage)
	flag.StringVar(&cfg.InnkeeperAuthToken, "innkeeper-auth-token", "", innkeeperAuthTokenUsage)
	flag.StringVar(&cfg.InnkeeperPreRouteFilters, "innkeeper-pre-route-filters", "", innkeeperPreRouteFiltersUsage)
	flag.StringVar(&cfg.InnkeeperPostRouteFilters, "innkeeper-post-route-filters", "", innkeeperPostRouteFiltersUsage)
	flag.StringVar(&cfg.RoutesFile, "routes-file", "", routesFileUsage)
	flag.StringVar(&cfg.InlineRoutes, "inline-routes", "", inlineRoutesUsage)
	flag.Int64Var(&cfg.SourcePollTimeout, "source-poll-timeout", defaultSourcePollTimeout, sourcePollTimeoutUsage)
	flag.Var(cfg.AppendFilters, "default-filters-append", defaultAppendFiltersUsage)
	flag.Var(cfg.PrependFilters, "default-filters-prepend", defaultPrependFiltersUsage)
	flag.BoolVar(&cfg.WaitFirstRouteLoad, "wait-first-route-load", false, waitFirstRouteLoadUsage)

	// Kubernetes:
	flag.BoolVar(&cfg.KubernetesIngress, "kubernetes", false, kubernetesUsage)
	flag.BoolVar(&cfg.KubernetesInCluster, "kubernetes-in-cluster", false, kubernetesInClusterUsage)
	flag.StringVar(&cfg.KubernetesURL, "kubernetes-url", "", kubernetesURLUsage)
	flag.BoolVar(&cfg.KubernetesHealthcheck, "kubernetes-healthcheck", true, kubernetesHealthcheckUsage)
	flag.BoolVar(&cfg.KubernetesHTTPSRedirect, "kubernetes-https-redirect", true, kubernetesHTTPSRedirectUsage)
	flag.IntVar(&cfg.KubernetesHTTPSRedirectCode, "kubernetes-https-redirect-code", 308, kubernetesHTTPSRedirectCodeUsage)
	flag.StringVar(&cfg.KubernetesIngressClass, "kubernetes-ingress-class", "", kubernetesIngressClassUsage)
	flag.StringVar(&cfg.KubernetesRouteGroupClass, "kubernetes-routegroup-class", "", kubernetesRouteGroupClassUsage)
	flag.StringVar(&cfg.WhitelistedHealthCheckCIDR, "whitelisted-healthcheck-cidr", "", whitelistedHealthCheckCIDRUsage)
	flag.StringVar(&cfg.KubernetesPathModeString, "kubernetes-path-mode", "kubernetes-ingress", kubernetesPathModeUsage)
	flag.StringVar(&cfg.KubernetesNamespace, "kubernetes-namespace", "", kubernetesNamespaceUsage)
	flag.BoolVar(&cfg.KubernetesEnableEastWest, "enable-kubernetes-east-west", false, kubernetesEnableEastWestUsage)
	flag.StringVar(&cfg.KubernetesEastWestDomain, "kubernetes-east-west-domain", "", kubernetesEastWestDomainUsage)
	flag.Var(cfg.KubernetesEastWestRangeDomains, "kubernetes-east-west-range-domains", kubernetesEastWestRangeDomainsUsage)
	flag.StringVar(&cfg.KubernetesEastWestRangePredicatesString, "kubernetes-east-west-range-predicates", "", kubernetesEastWestRangePredicatesUsage)

	// Auth:
	flag.BoolVar(&cfg.EnableOAuth2GrantFlow, "enable-oauth2-grant-flow", false, oauth2GrantFlowEnableUsage)
	flag.StringVar(&cfg.OauthURL, "oauth-url", "", oauthURLUsage)
	flag.StringVar(&cfg.OauthScope, "oauth-scope", "", oauthScopeUsage)
	flag.StringVar(&cfg.OauthCredentialsDir, "oauth-credentials-dir", "", oauthCredentialsDirUsage)
	flag.StringVar(&cfg.Oauth2AuthURL, "oauth2-auth-url", "", oauth2AuthURLUsage)
	flag.StringVar(&cfg.Oauth2TokenURL, "oauth2-token-url", "", oauth2TokenURLUsage)
	flag.StringVar(&cfg.Oauth2RevokeTokenURL, "oauth2-revoke-token-url", "", oauth2RevokeTokenURLUsage)
	flag.StringVar(&cfg.Oauth2TokeninfoURL, "oauth2-tokeninfo-url", "", oauth2TokeninfoURLUsage)
	flag.StringVar(&cfg.Oauth2SecretFile, "oauth2-secret-file", "", oauth2SecretFileUsage)
	flag.StringVar(&cfg.Oauth2ClientID, "oauth2-client-id", "", oauth2ClientIDUsage)
	flag.StringVar(&cfg.Oauth2ClientSecret, "oauth2-client-secret", "", oauth2ClientSecretUsage)
	flag.StringVar(&cfg.Oauth2ClientIDFile, "oauth2-client-id-file", "", oauth2ClientIDFileUsage)
	flag.StringVar(&cfg.Oauth2ClientSecretFile, "oauth2-client-secret-file", "", oauth2ClientSecretFileUsage)
	flag.StringVar(&cfg.Oauth2CallbackPath, "oauth2-callback-path", "", oauth2CallbackPathUsage)
	flag.DurationVar(&cfg.Oauth2TokeninfoTimeout, "oauth2-tokeninfo-timeout", defaultOAuthTokeninfoTimeout, oauth2TokeninfoTimeoutUsage)
	flag.DurationVar(&cfg.Oauth2TokenintrospectionTimeout, "oauth2-tokenintrospect-timeout", defaultOAuthTokenintrospectionTimeout, oauth2TokenintrospectionTimeoutUsage)
	flag.Var(&cfg.Oauth2AuthURLParameters, "oauth2-auth-url-parameters", oauth2AuthURLParametersUsage)
	flag.StringVar(&cfg.Oauth2AccessTokenHeaderName, "oauth2-access-token-header-name", "", oauth2AccessTokenHeaderNameUsage)
	flag.StringVar(&cfg.Oauth2TokeninfoSubjectKey, "oauth2-tokeninfo-subject-key", "uid", oauth2AccessTokenHeaderNameUsage)
	flag.StringVar(&cfg.Oauth2TokenCookieName, "oauth2-token-cookie-name", "oauth2-grant", oauth2TokenCookieNameUsage)
	flag.DurationVar(&cfg.WebhookTimeout, "webhook-timeout", defaultWebhookTimeout, webhookTimeoutUsage)
	flag.StringVar(&cfg.OidcSecretsFile, "oidc-secrets-file", "", oidcSecretsFileUsage)
	flag.Var(cfg.CredentialPaths, "credentials-paths", credentialPathsUsage)
	flag.DurationVar(&cfg.CredentialsUpdateInterval, "credentials-update-interval", defaultCredentialsUpdateInterval, credentialsUpdateIntervalUsage)

	// TLS client certs
	flag.StringVar(&cfg.ClientKeyFile, "client-tls-key", "", clientKeyFileUsage)
	flag.StringVar(&cfg.ClientCertFile, "client-tls-cert", "", clientCertFileUsage)

	// TLS version
	flag.StringVar(&cfg.TLSMinVersion, "tls-min-version", defaultMinTLSVersion, minTLSVersionUsage)

	// API Monitoring:
	flag.BoolVar(&cfg.ApiUsageMonitoringEnable, "enable-api-usage-monitoring", false, apiUsageMonitoringEnableUsage)
	flag.StringVar(&cfg.ApiUsageMonitoringRealmKeys, "api-usage-monitoring-realm-keys", defaultApiUsageMonitoringRealmKeys, apiUsageMonitoringRealmKeysUsage)
	flag.StringVar(&cfg.ApiUsageMonitoringClientKeys, "api-usage-monitoring-client-keys", defaultApiUsageMonitoringClientKeys, apiUsageMonitoringClientKeysUsage)
	flag.StringVar(&cfg.ApiUsageMonitoringDefaultClientTrackingPattern, "api-usage-monitoring-default-client-tracking-pattern", defaultApiUsageMonitoringDefaultClientTrackingPattern, apiUsageMonitoringDefaultClientTrackingPatternUsage)
	flag.StringVar(&cfg.ApiUsageMonitoringRealmsTrackingPattern, "api-usage-monitoring-realms-tracking-pattern", defaultApiUsageMonitoringRealmsTrackingPattern, apiUsageMonitoringRealmsTrackingPatternUsage)

	// Default filters:
	flag.StringVar(&cfg.DefaultFiltersDir, "default-filters-dir", "", defaultFiltersDirUsage)

	// Connections, timeouts:
	flag.DurationVar(&cfg.WaitForHealthcheckInterval, "wait-for-healthcheck-interval", defaultWaitForHealthcheckInterval, waitForHealthcheckIntervalUsage)
	flag.IntVar(&cfg.IdleConnsPerHost, "idle-conns-num", proxy.DefaultIdleConnsPerHost, idleConnsPerHostUsage)
	flag.DurationVar(&cfg.CloseIdleConnsPeriod, "close-idle-conns-period", proxy.DefaultCloseIdleConnsPeriod, closeIdleConnsPeriodUsage)
	flag.DurationVar(&cfg.BackendFlushInterval, "backend-flush-interval", defaultBackendFlushInterval, backendFlushIntervalUsage)
	flag.BoolVar(&cfg.ExperimentalUpgrade, "experimental-upgrade", false, experimentalUpgradeUsage)
	flag.BoolVar(&cfg.ExperimentalUpgradeAudit, "experimental-upgrade-audit", false, experimentalUpgradeAuditUsage)
	flag.DurationVar(&cfg.ReadTimeoutServer, "read-timeout-server", defaultReadTimeoutServer, readTimeoutServerUsage)
	flag.DurationVar(&cfg.ReadHeaderTimeoutServer, "read-header-timeout-server", defaultReadHeaderTimeoutServer, readHeaderTimeoutServerUsage)
	flag.DurationVar(&cfg.WriteTimeoutServer, "write-timeout-server", defaultWriteTimeoutServer, writeTimeoutServerUsage)
	flag.DurationVar(&cfg.IdleTimeoutServer, "idle-timeout-server", defaultIdleTimeoutServer, idleTimeoutServerUsage)
	flag.IntVar(&cfg.MaxHeaderBytes, "max-header-bytes", http.DefaultMaxHeaderBytes, maxHeaderBytesUsage)
	flag.BoolVar(&cfg.EnableConnMetricsServer, "enable-connection-metrics", false, enableConnMetricsServerUsage)
	flag.DurationVar(&cfg.TimeoutBackend, "timeout-backend", defaultTimeoutBackend, timeoutBackendUsage)
	flag.DurationVar(&cfg.KeepaliveBackend, "keepalive-backend", defaultKeepaliveBackend, keepaliveBackendUsage)
	flag.BoolVar(&cfg.EnableDualstackBackend, "enable-dualstack-backend", true, enableDualstackBackendUsage)
	flag.DurationVar(&cfg.TlsHandshakeTimeoutBackend, "tls-timeout-backend", defaultTLSHandshakeTimeoutBackend, tlsHandshakeTimeoutBackendUsage)
	flag.DurationVar(&cfg.ResponseHeaderTimeoutBackend, "response-header-timeout-backend", defaultResponseHeaderTimeoutBackend, responseHeaderTimeoutBackendUsage)
	flag.DurationVar(&cfg.ExpectContinueTimeoutBackend, "expect-continue-timeout-backend", defaultExpectContinueTimeoutBackend, expectContinueTimeoutBackendUsage)
	flag.IntVar(&cfg.MaxIdleConnsBackend, "max-idle-connection-backend", defaultMaxIdleConnsBackend, maxIdleConnsBackendUsage)
	flag.BoolVar(&cfg.DisableHTTPKeepalives, "disable-http-keepalives", false, disableHTTPKeepalivesUsage)

	// Swarm:
	flag.BoolVar(&cfg.EnableSwarm, "enable-swarm", false, enableSwarmUsage)
	flag.Var(cfg.SwarmRedisURLs, "swarm-redis-urls", swarmRedisURLsUsage)
	flag.DurationVar(&cfg.SwarmRedisDialTimeout, "swarm-redis-dial-timeout", ratelimit.DefaultDialTimeout, swarmRedisDialTimeoutUsage)
	flag.DurationVar(&cfg.SwarmRedisReadTimeout, "swarm-redis-read-timeout", ratelimit.DefaultReadTimeout, swarmRedisReadTimeoutUsage)
	flag.DurationVar(&cfg.SwarmRedisWriteTimeout, "swarm-redis-write-timeout", ratelimit.DefaultWriteTimeout, swarmRedisWriteTimeoutUsage)
	flag.DurationVar(&cfg.SwarmRedisPoolTimeout, "swarm-redis-pool-timeout", ratelimit.DefaultPoolTimeout, swarmRedisPoolTimeoutUsage)
	flag.IntVar(&cfg.SwarmRedisMinConns, "swarm-redis-min-conns", ratelimit.DefaultMinConns, swarmRedisMinConnsUsage)
	flag.IntVar(&cfg.SwarmRedisMaxConns, "swarm-redis-max-conns", ratelimit.DefaultMaxConns, swarmRedisMaxConnsUsage)
	flag.StringVar(&cfg.SwarmKubernetesNamespace, "swarm-namespace", swarm.DefaultNamespace, swarmKubernetesNamespaceUsage)
	flag.StringVar(&cfg.SwarmKubernetesLabelSelectorKey, "swarm-label-selector-key", swarm.DefaultLabelSelectorKey, swarmKubernetesLabelSelectorKeyUsage)
	flag.StringVar(&cfg.SwarmKubernetesLabelSelectorValue, "swarm-label-selector-value", swarm.DefaultLabelSelectorValue, swarmKubernetesLabelSelectorValueUsage)
	flag.IntVar(&cfg.SwarmPort, "swarm-port", swarm.DefaultPort, swarmPortUsage)
	flag.IntVar(&cfg.SwarmMaxMessageBuffer, "swarm-max-msg-buffer", swarm.DefaultMaxMessageBuffer, swarmMaxMessageBufferUsage)
	flag.DurationVar(&cfg.SwarmLeaveTimeout, "swarm-leave-timeout", swarm.DefaultLeaveTimeout, swarmLeaveTimeoutUsage)
	flag.StringVar(&cfg.SwarmStaticSelf, "swarm-static-self", "", swarmStaticSelfUsage)
	flag.StringVar(&cfg.SwarmStaticOther, "swarm-static-other", "", swarmStaticOtherUsage)

	return cfg
}

func (c *Config) Parse() error {
	flag.Parse()

	// check if arguments were correctly parsed.
	if len(flag.Args()) != 0 {
		return fmt.Errorf("invalid arguments: %s", flag.Args())
	}

	if c.ConfigFile != "" {
		yamlFile, err := ioutil.ReadFile(c.ConfigFile)
		if err != nil {
			return fmt.Errorf("invalid config file: %v", err)
		}

		err = yaml.Unmarshal(yamlFile, c)
		if err != nil {
			return fmt.Errorf("unmarshalling config file error: %v", err)
		}

		flag.Parse()
	}

	if c.ApiUsageMonitoringDefaultClientTrackingPattern != defaultApiUsageMonitoringDefaultClientTrackingPattern {
		log.Warn(`"api-usage-monitoring-default-client-tracking-pattern" parameter is deprecated`)
	}

	logLevel, err := log.ParseLevel(c.ApplicationLogLevelString)
	if err != nil {
		return err
	}

	kubernetesPathMode, err := kubernetes.ParsePathMode(c.KubernetesPathModeString)
	if err != nil {
		return err
	}

	if c.KubernetesEnableEastWest {
		log.Warn(`"kubernetes-enable-east-west" parameter is deprecated. Check the "kubernetes-east-west-range" feature`)
	}

	kubernetesEastWestRangePredicates, err := eskip.ParsePredicates(c.KubernetesEastWestRangePredicatesString)
	if err != nil {
		return fmt.Errorf("invalid east-west-range-predicates: %w", err)
	}

	histogramBuckets, err := c.parseHistogramBuckets()
	if err != nil {
		return err
	}

	c.ApplicationLogLevel = logLevel
	c.KubernetesPathMode = kubernetesPathMode
	c.KubernetesEastWestRangePredicates = kubernetesEastWestRangePredicates
	c.HistogramMetricBuckets = histogramBuckets

	if c.ClientKeyFile != "" && c.ClientCertFile != "" {
		certsFiles := strings.Split(c.ClientCertFile, ",")
		keyFiles := strings.Split(c.ClientKeyFile, ",")

		var certificates []tls.Certificate
		for i := range keyFiles {
			certificate, err := tls.LoadX509KeyPair(certsFiles[i], keyFiles[i])
			if err != nil {
				return fmt.Errorf("invalid key/cert pair: %v", err)
			}

			certificates = append(certificates, certificate)
		}

		c.Certificates = certificates
	}

	return nil
}

func (c *Config) ToOptions() skipper.Options {
	var eus []string
	if len(c.EtcdUrls) > 0 {
		eus = strings.Split(c.EtcdUrls, ",")
	}

	var whitelistCIDRS []string
	if len(c.WhitelistedHealthCheckCIDR) > 0 {
		whitelistCIDRS = strings.Split(c.WhitelistedHealthCheckCIDR, ",")
	}

	options := skipper.Options{
		// generic:
		Address:                         c.Address,
		StatusChecks:                    c.StatusChecks.values,
		EnableTCPQueue:                  c.EnableTCPQueue,
		ExpectedBytesPerRequest:         c.ExpectedBytesPerRequest,
		MaxTCPListenerConcurrency:       c.MaxTCPListenerConcurrency,
		MaxTCPListenerQueue:             c.MaxTCPListenerQueue,
		IgnoreTrailingSlash:             c.IgnoreTrailingSlash,
		DevMode:                         c.DevMode,
		SupportListener:                 c.SupportListener,
		DebugListener:                   c.DebugListener,
		CertPathTLS:                     c.CertPathTLS,
		KeyPathTLS:                      c.KeyPathTLS,
		MaxLoopbacks:                    c.MaxLoopbacks,
		DefaultHTTPStatus:               c.DefaultHTTPStatus,
		LoadBalancerHealthCheckInterval: c.LoadBalancerHealthCheckInterval,
		ReverseSourcePredicate:          c.ReverseSourcePredicate,
		MaxAuditBody:                    c.MaxAuditBody,
		EnableBreakers:                  c.EnableBreakers,
		BreakerSettings:                 c.Breakers,
		EnableRatelimiters:              c.EnableRatelimiters,
		RatelimitSettings:               c.Ratelimits,
		EnableRouteLIFOMetrics:          c.EnableRouteLIFOMetrics,
		MetricsFlavours:                 c.MetricsFlavour.values,
		FilterPlugins:                   c.FilterPlugins.values,
		PredicatePlugins:                c.PredicatePlugins.values,
		DataClientPlugins:               c.DataclientPlugins.values,
		Plugins:                         c.MultiPlugins.values,
		PluginDirs:                      []string{skipper.DefaultPluginDir},

		// logging, metrics, tracing:
		EnablePrometheusMetrics:             c.EnablePrometheusMetrics,
		OpenTracing:                         strings.Split(c.OpenTracing, " "),
		OpenTracingInitialSpan:              c.OpenTracingInitialSpan,
		OpenTracingExcludedProxyTags:        strings.Split(c.OpenTracingExcludedProxyTags, ","),
		OpenTracingLogStreamEvents:          c.OpentracingLogStreamEvents,
		OpenTracingLogFilterLifecycleEvents: c.OpentracingLogFilterLifecycleEvents,
		MetricsListener:                     c.MetricsListener,
		MetricsPrefix:                       c.MetricsPrefix,
		EnableProfile:                       c.EnableProfile,
		EnableDebugGcMetrics:                c.DebugGcMetrics,
		EnableRuntimeMetrics:                c.RuntimeMetrics,
		EnableServeRouteMetrics:             c.ServeRouteMetrics,
		EnableServeHostMetrics:              c.ServeHostMetrics,
		EnableBackendHostMetrics:            c.BackendHostMetrics,
		EnableAllFiltersMetrics:             c.AllFiltersMetrics,
		EnableCombinedResponseMetrics:       c.CombinedResponseMetrics,
		EnableRouteResponseMetrics:          c.RouteResponseMetrics,
		EnableRouteBackendErrorsCounters:    c.RouteBackendErrorCounters,
		EnableRouteStreamingErrorsCounters:  c.RouteStreamErrorCounters,
		EnableRouteBackendMetrics:           c.RouteBackendMetrics,
		EnableRouteCreationMetrics:          c.RouteCreationMetrics,
		MetricsUseExpDecaySample:            c.MetricsUseExpDecaySample,
		HistogramMetricBuckets:              c.HistogramMetricBuckets,
		DisableMetricsCompatibilityDefaults: c.DisableMetricsCompat,
		ApplicationLogOutput:                c.ApplicationLog,
		ApplicationLogPrefix:                c.ApplicationLogPrefix,
		ApplicationLogJSONEnabled:           c.ApplicationLogJSONEnabled,
		AccessLogOutput:                     c.AccessLog,
		AccessLogDisabled:                   c.AccessLogDisabled,
		AccessLogJSONEnabled:                c.AccessLogJSONEnabled,
		AccessLogStripQuery:                 c.AccessLogStripQuery,
		SuppressRouteUpdateLogs:             c.SuppressRouteUpdateLogs,

		// route sources:
		EtcdUrls:                  eus,
		EtcdPrefix:                c.EtcdPrefix,
		EtcdWaitTimeout:           c.EtcdTimeout,
		EtcdInsecure:              c.EtcdInsecure,
		EtcdOAuthToken:            c.EtcdOAuthToken,
		EtcdUsername:              c.EtcdUsername,
		EtcdPassword:              c.EtcdPassword,
		InnkeeperUrl:              c.InnkeeperURL,
		InnkeeperAuthToken:        c.InnkeeperAuthToken,
		InnkeeperPreRouteFilters:  c.InnkeeperPreRouteFilters,
		InnkeeperPostRouteFilters: c.InnkeeperPostRouteFilters,
		WatchRoutesFile:           c.RoutesFile,
		InlineRoutes:              c.InlineRoutes,
		DefaultFilters: &eskip.DefaultFilters{
			Prepend: c.PrependFilters.filters,
			Append:  c.AppendFilters.filters,
		},
		SourcePollTimeout:  time.Duration(c.SourcePollTimeout) * time.Millisecond,
		WaitFirstRouteLoad: c.WaitFirstRouteLoad,

		// Kubernetes:
		Kubernetes:                        c.KubernetesIngress,
		KubernetesInCluster:               c.KubernetesInCluster,
		KubernetesURL:                     c.KubernetesURL,
		KubernetesHealthcheck:             c.KubernetesHealthcheck,
		KubernetesHTTPSRedirect:           c.KubernetesHTTPSRedirect,
		KubernetesHTTPSRedirectCode:       c.KubernetesHTTPSRedirectCode,
		KubernetesIngressClass:            c.KubernetesIngressClass,
		KubernetesRouteGroupClass:         c.KubernetesRouteGroupClass,
		WhitelistedHealthCheckCIDR:        whitelistCIDRS,
		KubernetesPathMode:                c.KubernetesPathMode,
		KubernetesNamespace:               c.KubernetesNamespace,
		KubernetesEnableEastWest:          c.KubernetesEnableEastWest,
		KubernetesEastWestDomain:          c.KubernetesEastWestDomain,
		KubernetesEastWestRangeDomains:    c.KubernetesEastWestRangeDomains.values,
		KubernetesEastWestRangePredicates: c.KubernetesEastWestRangePredicates,

		// API Monitoring:
		ApiUsageMonitoringEnable:                c.ApiUsageMonitoringEnable,
		ApiUsageMonitoringRealmKeys:             c.ApiUsageMonitoringRealmKeys,
		ApiUsageMonitoringClientKeys:            c.ApiUsageMonitoringClientKeys,
		ApiUsageMonitoringRealmsTrackingPattern: c.ApiUsageMonitoringRealmsTrackingPattern,

		// Default filters:
		DefaultFiltersDir: c.DefaultFiltersDir,

		// Auth:
		EnableOAuth2GrantFlow:          c.EnableOAuth2GrantFlow,
		OAuthUrl:                       c.OauthURL,
		OAuthScope:                     c.OauthScope,
		OAuthCredentialsDir:            c.OauthCredentialsDir,
		OAuth2AuthURL:                  c.Oauth2AuthURL,
		OAuth2TokenURL:                 c.Oauth2TokenURL,
		OAuth2RevokeTokenURL:           c.Oauth2RevokeTokenURL,
		OAuthTokeninfoURL:              c.Oauth2TokeninfoURL,
		OAuthTokeninfoTimeout:          c.Oauth2TokeninfoTimeout,
		OAuth2SecretFile:               c.Oauth2SecretFile,
		OAuth2ClientID:                 c.Oauth2ClientID,
		OAuth2ClientSecret:             c.Oauth2ClientSecret,
		OAuth2ClientIDFile:             c.Oauth2ClientIDFile,
		OAuth2ClientSecretFile:         c.Oauth2ClientSecretFile,
		OAuth2CallbackPath:             c.Oauth2CallbackPath,
		OAuthTokenintrospectionTimeout: c.Oauth2TokenintrospectionTimeout,
		OAuth2AuthURLParameters:        c.Oauth2AuthURLParameters.values,
		OAuth2AccessTokenHeaderName:    c.Oauth2AccessTokenHeaderName,
		OAuth2TokeninfoSubjectKey:      c.Oauth2TokeninfoSubjectKey,
		OAuth2TokenCookieName:          c.Oauth2TokenCookieName,
		WebhookTimeout:                 c.WebhookTimeout,
		OIDCSecretsFile:                c.OidcSecretsFile,
		CredentialsPaths:               c.CredentialPaths.values,
		CredentialsUpdateInterval:      c.CredentialsUpdateInterval,

		// connections, timeouts:
		WaitForHealthcheckInterval:   c.WaitForHealthcheckInterval,
		IdleConnectionsPerHost:       c.IdleConnsPerHost,
		CloseIdleConnsPeriod:         c.CloseIdleConnsPeriod,
		BackendFlushInterval:         c.BackendFlushInterval,
		ExperimentalUpgrade:          c.ExperimentalUpgrade,
		ExperimentalUpgradeAudit:     c.ExperimentalUpgradeAudit,
		ReadTimeoutServer:            c.ReadTimeoutServer,
		ReadHeaderTimeoutServer:      c.ReadHeaderTimeoutServer,
		WriteTimeoutServer:           c.WriteTimeoutServer,
		IdleTimeoutServer:            c.IdleTimeoutServer,
		MaxHeaderBytes:               c.MaxHeaderBytes,
		EnableConnMetricsServer:      c.EnableConnMetricsServer,
		TimeoutBackend:               c.TimeoutBackend,
		KeepAliveBackend:             c.KeepaliveBackend,
		DualStackBackend:             c.EnableDualstackBackend,
		TLSHandshakeTimeoutBackend:   c.TlsHandshakeTimeoutBackend,
		ResponseHeaderTimeoutBackend: c.ResponseHeaderTimeoutBackend,
		ExpectContinueTimeoutBackend: c.ExpectContinueTimeoutBackend,
		MaxIdleConnsBackend:          c.MaxIdleConnsBackend,
		DisableHTTPKeepalives:        c.DisableHTTPKeepalives,

		// swarm:
		EnableSwarm: c.EnableSwarm,
		// redis based
		SwarmRedisURLs:         c.SwarmRedisURLs.values,
		SwarmRedisDialTimeout:  c.SwarmRedisDialTimeout,
		SwarmRedisReadTimeout:  c.SwarmRedisReadTimeout,
		SwarmRedisWriteTimeout: c.SwarmRedisWriteTimeout,
		SwarmRedisPoolTimeout:  c.SwarmRedisPoolTimeout,
		SwarmRedisMinIdleConns: c.SwarmRedisMinConns,
		SwarmRedisMaxIdleConns: c.SwarmRedisMaxConns,
		// swim based
		SwarmKubernetesNamespace:          c.SwarmKubernetesNamespace,
		SwarmKubernetesLabelSelectorKey:   c.SwarmKubernetesLabelSelectorKey,
		SwarmKubernetesLabelSelectorValue: c.SwarmKubernetesLabelSelectorValue,
		SwarmPort:                         c.SwarmPort,
		SwarmMaxMessageBuffer:             c.SwarmMaxMessageBuffer,
		SwarmLeaveTimeout:                 c.SwarmLeaveTimeout,
		// swim on localhost for testing
		SwarmStaticSelf:  c.SwarmStaticSelf,
		SwarmStaticOther: c.SwarmStaticOther,
	}

	if c.PluginDir != "" {
		options.PluginDirs = append(options.PluginDirs, c.PluginDir)
	}

	if c.Insecure {
		options.ProxyFlags |= proxy.Insecure
	}

	if c.ProxyPreserveHost {
		options.ProxyFlags |= proxy.PreserveHost
	}

	if c.RemoveHopHeaders {
		options.ProxyFlags |= proxy.HopHeadersRemoval
	}

	if c.RfcPatchPath {
		options.ProxyFlags |= proxy.PatchPath
	}

	if c.Certificates != nil && len(c.Certificates) > 0 {
		options.ClientTLS = &tls.Config{
			Certificates: c.Certificates,
			MinVersion:   c.getMinTLSVersion(),
		}
	}

	return options
}

func (c *Config) getMinTLSVersion() uint16 {
	tlsVersionTable := map[string]uint16{
		"1.3": tls.VersionTLS13,
		"13":  tls.VersionTLS13,
		"1.2": tls.VersionTLS12,
		"12":  tls.VersionTLS12,
		"1.1": tls.VersionTLS11,
		"11":  tls.VersionTLS11,
		"1.0": tls.VersionTLS10,
		"10":  tls.VersionTLS10,
	}
	if v, ok := tlsVersionTable[c.TLSMinVersion]; ok {
		return v
	}
	log.Infof("No valid minimal TLS version confiured (set to '%s'), fallback to default: %s", c.TLSMinVersion, defaultMinTLSVersion)
	return tlsVersionTable[defaultMinTLSVersion]
}

func (c *Config) parseHistogramBuckets() ([]float64, error) {
	if c.HistogramMetricBucketsString == "" {
		return prometheus.DefBuckets, nil
	}

	var result []float64
	thresholds := strings.Split(c.HistogramMetricBucketsString, ",")
	for _, v := range thresholds {
		bucket, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("unable to parse histogram-metric-buckets: %v", err)
		}
		result = append(result, bucket)
	}
	sort.Float64s(result)
	return result, nil
}
