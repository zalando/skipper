package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/otel"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/swarm"
)

type Config struct {
	ConfigFile string
	Flags      *flag.FlagSet

	// generic:
	Address                          string         `yaml:"address"`
	InsecureAddress                  string         `yaml:"insecure-address"`
	EnableTCPQueue                   bool           `yaml:"enable-tcp-queue"`
	ExpectedBytesPerRequest          int            `yaml:"expected-bytes-per-request"`
	MaxTCPListenerConcurrency        int            `yaml:"max-tcp-listener-concurrency"`
	MaxTCPListenerQueue              int            `yaml:"max-tcp-listener-queue"`
	EnableCopyStreamPoolExperimental bool           `yaml:"enable-copy-stream-pool"`
	IgnoreTrailingSlash              bool           `yaml:"ignore-trailing-slash"`
	Insecure                         bool           `yaml:"insecure"`
	ProxyPreserveHost                bool           `yaml:"proxy-preserve-host"`
	DevMode                          bool           `yaml:"dev-mode"`
	SupportListener                  string         `yaml:"support-listener"`
	DebugListener                    string         `yaml:"debug-listener"`
	CertPathTLS                      string         `yaml:"tls-cert"`
	KeyPathTLS                       string         `yaml:"tls-key"`
	StatusChecks                     *listFlag      `yaml:"status-checks"`
	PrintVersion                     bool           `yaml:"version"`
	MaxLoopbacks                     int            `yaml:"max-loopbacks"`
	DefaultHTTPStatus                int            `yaml:"default-http-status"`
	PluginDir                        string         `yaml:"plugindir"`
	LoadBalancerHealthCheckInterval  time.Duration  `yaml:"lb-healthcheck-interval"`
	ReverseSourcePredicate           bool           `yaml:"reverse-source-predicate"`
	RemoveHopHeaders                 bool           `yaml:"remove-hop-headers"`
	RfcPatchPath                     bool           `yaml:"rfc-patch-path"`
	MaxAuditBody                     int            `yaml:"max-audit-body"`
	MaxMatcherBufferSize             uint64         `yaml:"max-matcher-buffer-size"`
	EnableBreakers                   bool           `yaml:"enable-breakers"`
	Breakers                         breakerFlags   `yaml:"breaker"`
	EnableRatelimiters               bool           `yaml:"enable-ratelimits"`
	Ratelimits                       ratelimitFlags `yaml:"ratelimits"`
	EnableRouteFIFOMetrics           bool           `yaml:"enable-route-fifo-metrics"`
	EnableRouteLIFOMetrics           bool           `yaml:"enable-route-lifo-metrics"`
	MetricsFlavour                   *listFlag      `yaml:"metrics-flavour"`
	FilterPlugins                    *pluginFlag    `yaml:"filter-plugin"`
	PredicatePlugins                 *pluginFlag    `yaml:"predicate-plugin"`
	DataclientPlugins                *pluginFlag    `yaml:"dataclient-plugin"`
	MultiPlugins                     *pluginFlag    `yaml:"multi-plugin"`
	CompressEncodings                *listFlag      `yaml:"compress-encodings"`

	// logging, metrics, profiling, tracing:
	EnablePrometheusMetrics             bool      `yaml:"enable-prometheus-metrics"`
	EnablePrometheusStartLabel          bool      `yaml:"enable-prometheus-start-label"`
	OpenTracing                         string    `yaml:"opentracing"`
	OpenTracingInitialSpan              string    `yaml:"opentracing-initial-span"`
	OpenTracingExcludedProxyTags        string    `yaml:"opentracing-excluded-proxy-tags"`
	OpenTracingDisableFilterSpans       bool      `yaml:"opentracing-disable-filter-spans"`
	OpentracingLogFilterLifecycleEvents bool      `yaml:"opentracing-log-filter-lifecycle-events"`
	OpentracingLogStreamEvents          bool      `yaml:"opentracing-log-stream-events"`
	OpentracingBackendNameTag           bool      `yaml:"opentracing-backend-name-tag"`
	MetricsListener                     string    `yaml:"metrics-listener"`
	MetricsPrefix                       string    `yaml:"metrics-prefix"`
	EnableProfile                       bool      `yaml:"enable-profile"`
	BlockProfileRate                    int       `yaml:"block-profile-rate"`
	MutexProfileFraction                int       `yaml:"mutex-profile-fraction"`
	MemProfileRate                      int       `yaml:"memory-profile-rate"`
	DebugGcMetrics                      bool      `yaml:"debug-gc-metrics"`
	RuntimeMetrics                      bool      `yaml:"runtime-metrics"`
	ServeRouteMetrics                   bool      `yaml:"serve-route-metrics"`
	ServeRouteCounter                   bool      `yaml:"serve-route-counter"`
	ServeHostMetrics                    bool      `yaml:"serve-host-metrics"`
	ServeHostCounter                    bool      `yaml:"serve-host-counter"`
	ServeMethodMetric                   bool      `yaml:"serve-method-metric"`
	ServeStatusCodeMetric               bool      `yaml:"serve-status-code-metric"`
	BackendHostMetrics                  bool      `yaml:"backend-host-metrics"`
	ProxyRequestMetrics                 bool      `yaml:"proxy-request-metrics"`
	ProxyResponseMetrics                bool      `yaml:"proxy-response-metrics"`
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
	ResponseSizeBucketsString           string    `yaml:"response-size-buckets"`
	ResponseSizeBuckets                 []float64 `yaml:"-"`
	RequestSizeBucketsString            string    `yaml:"request-size-buckets"`
	RequestSizeBuckets                  []float64 `yaml:"-"`
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

	OpenTelemetry *otel.Options `yaml:"open-telemetry"`

	// route sources:
	EtcdUrls           string               `yaml:"etcd-urls"`
	EtcdPrefix         string               `yaml:"etcd-prefix"`
	EtcdTimeout        time.Duration        `yaml:"etcd-timeout"`
	EtcdInsecure       bool                 `yaml:"etcd-insecure"`
	EtcdOAuthToken     string               `yaml:"etcd-oauth-token"`
	EtcdUsername       string               `yaml:"etcd-username"`
	EtcdPassword       string               `yaml:"etcd-password"`
	RoutesFile         string               `yaml:"routes-file"`
	RoutesURLs         *listFlag            `yaml:"routes-urls"`
	InlineRoutes       string               `yaml:"inline-routes"`
	ForwardBackendURL  string               `yaml:"forward-backend-url"`
	AppendFilters      *defaultFiltersFlags `yaml:"default-filters-append"`
	PrependFilters     *defaultFiltersFlags `yaml:"default-filters-prepend"`
	DisabledFilters    *listFlag            `yaml:"disabled-filters"`
	EditRoute          routeChangerConfig   `yaml:"edit-route"`
	CloneRoute         routeChangerConfig   `yaml:"clone-route"`
	SourcePollTimeout  int64                `yaml:"source-poll-timeout"`
	WaitFirstRouteLoad bool                 `yaml:"wait-first-route-load"`

	// Forwarded headers
	ForwardedHeadersList            *listFlag            `yaml:"forwarded-headers"`
	ForwardedHeaders                net.ForwardedHeaders `yaml:"-"`
	ForwardedHeadersExcludeCIDRList *listFlag            `yaml:"forwarded-headers-exclude-cidrs"`
	ForwardedHeadersExcludeCIDRs    net.IPNets           `yaml:"-"`

	// host patch:
	NormalizeHost bool          `yaml:"normalize-host"`
	HostPatch     net.HostPatch `yaml:"-"`

	ValidateQuery    bool      `yaml:"validate-query"`
	ValidateQueryLog bool      `yaml:"validate-query-log"`
	MaxContentLength int64     `yaml:"max-content-length"`
	RefusePayload    multiFlag `yaml:"refuse-payload"`

	// Kubernetes:
	KubernetesIngress                                    bool                               `yaml:"kubernetes"`
	KubernetesInCluster                                  bool                               `yaml:"kubernetes-in-cluster"`
	KubernetesURL                                        string                             `yaml:"kubernetes-url"`
	KubernetesTokenFile                                  string                             `yaml:"kubernetes-token-file"`
	KubernetesHealthcheck                                bool                               `yaml:"kubernetes-healthcheck"`
	KubernetesHTTPSRedirect                              bool                               `yaml:"kubernetes-https-redirect"`
	KubernetesHTTPSRedirectCode                          int                                `yaml:"kubernetes-https-redirect-code"`
	KubernetesDisableCatchAllRoutes                      bool                               `yaml:"kubernetes-disable-catchall-routes"`
	KubernetesIngressClass                               string                             `yaml:"kubernetes-ingress-class"`
	KubernetesRouteGroupClass                            string                             `yaml:"kubernetes-routegroup-class"`
	WhitelistedHealthCheckCIDR                           string                             `yaml:"whitelisted-healthcheck-cidr"`
	KubernetesPathModeString                             string                             `yaml:"kubernetes-path-mode"`
	KubernetesPathMode                                   kubernetes.PathMode                `yaml:"-"`
	KubernetesNamespace                                  string                             `yaml:"kubernetes-namespace"`
	KubernetesEnableEndpointSlices                       bool                               `yaml:"enable-kubernetes-endpointslices"`
	KubernetesEnableEastWest                             bool                               `yaml:"enable-kubernetes-east-west"`
	KubernetesEastWestDomain                             string                             `yaml:"kubernetes-east-west-domain"`
	KubernetesEastWestRangeDomains                       *listFlag                          `yaml:"kubernetes-east-west-range-domains"`
	KubernetesEastWestRangePredicatesString              string                             `yaml:"kubernetes-east-west-range-predicates"`
	KubernetesEastWestRangeAnnotationPredicatesString    multiFlag                          `yaml:"kubernetes-east-west-range-annotation-predicates"`
	KubernetesEastWestRangeAnnotationFiltersAppendString multiFlag                          `yaml:"kubernetes-east-west-range-annotation-filters-append"`
	KubernetesAnnotationPredicatesString                 multiFlag                          `yaml:"kubernetes-annotation-predicates"`
	KubernetesAnnotationFiltersAppendString              multiFlag                          `yaml:"kubernetes-annotation-filters-append"`
	KubernetesEastWestRangeAnnotationPredicates          []kubernetes.AnnotationPredicates  `yaml:"-"`
	KubernetesEastWestRangeAnnotationFiltersAppend       []kubernetes.AnnotationFilters     `yaml:"-"`
	KubernetesAnnotationPredicates                       []kubernetes.AnnotationPredicates  `yaml:"-"`
	KubernetesAnnotationFiltersAppend                    []kubernetes.AnnotationFilters     `yaml:"-"`
	KubernetesEastWestRangePredicates                    []*eskip.Predicate                 `yaml:"-"`
	EnableKubernetesExternalNames                        bool                               `yaml:"enable-kubernetes-external-names"`
	KubernetesOnlyAllowedExternalNames                   bool                               `yaml:"kubernetes-only-allowed-external-names"`
	KubernetesAllowedExternalNames                       regexpListFlag                     `yaml:"kubernetes-allowed-external-names"`
	KubernetesRedisServiceNamespace                      string                             `yaml:"kubernetes-redis-service-namespace"`
	KubernetesRedisServiceName                           string                             `yaml:"kubernetes-redis-service-name"`
	KubernetesRedisServicePort                           int                                `yaml:"kubernetes-redis-service-port"`
	KubernetesValkeyServiceNamespace                     string                             `yaml:"kubernetes-valkey-service-namespace"`
	KubernetesValkeyServiceName                          string                             `yaml:"kubernetes-valkey-service-name"`
	KubernetesValkeyServicePort                          int                                `yaml:"kubernetes-valkey-service-port"`
	KubernetesBackendTrafficAlgorithmString              string                             `yaml:"kubernetes-backend-traffic-algorithm"`
	KubernetesBackendTrafficAlgorithm                    kubernetes.BackendTrafficAlgorithm `yaml:"-"`
	KubernetesDefaultLoadBalancerAlgorithm               string                             `yaml:"kubernetes-default-lb-algorithm"`
	KubernetesForceService                               bool                               `yaml:"kubernetes-force-service"`
	KubernetesIngressStatusFromService                   string                             `yaml:"kubernetes-ingress-status-from-service"`

	// Default filters
	DefaultFiltersDir string `yaml:"default-filters-dir"`

	// Auth:
	EnableOAuth2GrantFlow             bool          `yaml:"enable-oauth2-grant-flow"`
	Oauth2AuthURL                     string        `yaml:"oauth2-auth-url"`
	Oauth2TokenURL                    string        `yaml:"oauth2-token-url"`
	Oauth2RevokeTokenURL              string        `yaml:"oauth2-revoke-token-url"`
	Oauth2TokeninfoURL                string        `yaml:"oauth2-tokeninfo-url"`
	Oauth2TokeninfoTimeout            time.Duration `yaml:"oauth2-tokeninfo-timeout"`
	Oauth2TokeninfoCacheSize          int           `yaml:"oauth2-tokeninfo-cache-size"`
	Oauth2TokeninfoCacheTTL           time.Duration `yaml:"oauth2-tokeninfo-cache-ttl"`
	Oauth2SecretFile                  string        `yaml:"oauth2-secret-file"`
	Oauth2ClientID                    string        `yaml:"oauth2-client-id"`
	Oauth2ClientSecret                string        `yaml:"oauth2-client-secret"`
	Oauth2ClientIDFile                string        `yaml:"oauth2-client-id-file"`
	Oauth2ClientSecretFile            string        `yaml:"oauth2-client-secret-file"`
	Oauth2AuthURLParameters           mapFlags      `yaml:"oauth2-auth-url-parameters"`
	Oauth2CallbackPath                string        `yaml:"oauth2-callback-path"`
	Oauth2TokenintrospectionTimeout   time.Duration `yaml:"oauth2-tokenintrospect-timeout"`
	Oauth2AccessTokenHeaderName       string        `yaml:"oauth2-access-token-header-name"`
	Oauth2TokeninfoSubjectKey         string        `yaml:"oauth2-tokeninfo-subject-key"`
	Oauth2GrantTokeninfoKeys          *listFlag     `yaml:"oauth2-grant-tokeninfo-keys"`
	Oauth2TokenCookieName             string        `yaml:"oauth2-token-cookie-name"`
	Oauth2TokenCookieRemoveSubdomains int           `yaml:"oauth2-token-cookie-remove-subdomains"`
	Oauth2GrantInsecure               bool          `yaml:"oauth2-grant-insecure"`
	WebhookTimeout                    time.Duration `yaml:"webhook-timeout"`
	OidcSecretsFile                   string        `yaml:"oidc-secrets-file"`
	OIDCCookieValidity                time.Duration `yaml:"oidc-cookie-validity"`
	OidcDistributedClaimsTimeout      time.Duration `yaml:"oidc-distributed-claims-timeout"`
	OIDCCookieRemoveSubdomains        int           `yaml:"oidc-cookie-remove-subdomains"`
	CredentialPaths                   *listFlag     `yaml:"credentials-paths"`
	CredentialsUpdateInterval         time.Duration `yaml:"credentials-update-interval"`

	// TLS configuration for the validation webhook
	ValidationWebhookEnabled  bool   `yaml:"validation-webhook-enabled"`
	ValidationWebhookAddress  string `yaml:"validation-webhook-address"`
	ValidationWebhookCertFile string `yaml:"validation-webhook-cert-file"`
	ValidationWebhookKeyFile  string `yaml:"validation-webhook-key-file"`
	EnableAdvancedValidation  bool   `yaml:"enable-advanced-validation"`

	// TLS client certs
	ClientKeyFile  string            `yaml:"client-tls-key"`
	ClientCertFile string            `yaml:"client-tls-cert"`
	Certificates   []tls.Certificate `yaml:"-"`

	// TLS version
	TLSMinVersion string             `yaml:"tls-min-version"`
	TLSClientAuth tls.ClientAuthType `yaml:"tls-client-auth"`

	// Exclude insecure cipher suites
	ExcludeInsecureCipherSuites bool `yaml:"exclude-insecure-cipher-suites"`

	// TLS Config
	KubernetesEnableTLS bool `yaml:"kubernetes-enable-tls"`

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
	KeepaliveServer              time.Duration `yaml:"keepalive-server"`
	KeepaliveRequestsServer      int           `yaml:"keepalive-requests-server"`
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
	SwarmRedisURLs               *listFlag     `yaml:"swarm-redis-urls"`
	SwarmRedisUsername           string        `yaml:"swarm-redis-username"`
	SwarmRedisPassword           string        `yaml:"swarm-redis-password"`
	SwarmRedisHashAlgorithm      string        `yaml:"swarm-redis-hash-algorithm"`
	SwarmRedisDialTimeout        time.Duration `yaml:"swarm-redis-dial-timeout"`
	SwarmRedisReadTimeout        time.Duration `yaml:"swarm-redis-read-timeout"`
	SwarmRedisWriteTimeout       time.Duration `yaml:"swarm-redis-write-timeout"`
	SwarmRedisPoolTimeout        time.Duration `yaml:"swarm-redis-pool-timeout"`
	SwarmRedisMinConns           int           `yaml:"swarm-redis-min-conns"`
	SwarmRedisMaxConns           int           `yaml:"swarm-redis-max-conns"`
	SwarmRedisEndpointsRemoteURL string        `yaml:"swarm-redis-remote"`
	SwarmRedisUpdateInterval     time.Duration `yaml:"swarm-redis-update-interval"`
	SwarmRedisHeartbeatFrequency time.Duration `yaml:"swarm-redis-heartbeat-frequency"`
	// valkey based
	SwarmValkeyURLs               *listFlag     `yaml:"swarm-valkey-urls"`
	SwarmValkeyEndpointsRemoteURL string        `yaml:"swarm-valkey-remote"`
	SwarmValkeyUsername           string        `yaml:"swarm-valkey-username"`
	SwarmValkeyPassword           string        `yaml:"swarm-valkey-password"`
	SwarmValkeyConnLifetime       time.Duration `yaml:"swarm-valkey-conn-lifetime"`
	SwarmValkeyConnWriteTimeout   time.Duration `yaml:"swarm-valkey-conn-timeout"`
	SwarmValkeyUpdateInterval     time.Duration `yaml:"swarm-valkey-update-interval"`
	// swim based
	SwarmKubernetesNamespace          string        `yaml:"swarm-namespace"`
	SwarmKubernetesLabelSelectorKey   string        `yaml:"swarm-label-selector-key"`
	SwarmKubernetesLabelSelectorValue string        `yaml:"swarm-label-selector-value"`
	SwarmPort                         int           `yaml:"swarm-port"`
	SwarmMaxMessageBuffer             int           `yaml:"swarm-max-msg-buffer"`
	SwarmLeaveTimeout                 time.Duration `yaml:"swarm-leave-timeout"`
	SwarmStaticSelf                   string        `yaml:"swarm-static-self"`
	SwarmStaticOther                  string        `yaml:"swarm-static-other"`

	ClusterRatelimitMaxGroupShards int `yaml:"cluster-ratelimit-max-group-shards"`

	EnableLua  bool      `yaml:"enable-lua"`
	LuaModules *listFlag `yaml:"lua-modules"`
	LuaSources *listFlag `yaml:"lua-sources"`

	EnableOpenPolicyAgent                              bool          `yaml:"enable-open-policy-agent"`
	EnableOpenPolicyAgentCustomControlLoop             bool          `yaml:"enable-open-policy-agent-custom-control-loop"`
	OpenPolicyAgentControlLoopInterval                 time.Duration `yaml:"open-policy-agent-control-loop-interval"`
	OpenPolicyAgentControlLoopMaxJitter                time.Duration `yaml:"open-policy-agent-control-loop-max-jitter"`
	EnableOpenPolicyAgentDataPreProcessingOptimization bool          `yaml:"enable-open-policy-agent-data-preprocessing-optimization"`
	EnableOpenPolicyAgentPreloading                    bool          `yaml:"enable-open-policy-agent-preloading"`
	OpenPolicyAgentConfigTemplate                      string        `yaml:"open-policy-agent-config-template"`
	OpenPolicyAgentEnvoyMetadata                       string        `yaml:"open-policy-agent-envoy-metadata"`
	OpenPolicyAgentCleanerInterval                     time.Duration `yaml:"open-policy-agent-cleaner-interval"`
	OpenPolicyAgentStartupTimeout                      time.Duration `yaml:"open-policy-agent-startup-timeout"`
	OpenPolicyAgentRequestBodyBufferSize               int64         `yaml:"open-policy-agent-request-body-buffer-size"`
	OpenPolicyAgentMaxRequestBodySize                  int64         `yaml:"open-policy-agent-max-request-body-size"`
	OpenPolicyAgentMaxMemoryBodyParsing                int64         `yaml:"open-policy-agent-max-memory-body-parsing"`

	PassiveHealthCheck mapFlags `yaml:"passive-health-check"`
}

const (
	// TLS
	defaultMinTLSVersion = "1.2"

	// environment keys:
	redisPasswordEnv  = "SWARM_REDIS_PASSWORD"
	valkeyPasswordEnv = "SWARM_VALKEY_PASSWORD"
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
	cfg.SwarmValkeyURLs = commaListFlag()
	cfg.AppendFilters = &defaultFiltersFlags{}
	cfg.PrependFilters = &defaultFiltersFlags{}
	cfg.DisabledFilters = commaListFlag()
	cfg.CloneRoute = routeChangerConfig{}
	cfg.EditRoute = routeChangerConfig{}
	cfg.KubernetesEastWestRangeDomains = commaListFlag()
	cfg.RoutesURLs = commaListFlag()
	cfg.ForwardedHeadersList = commaListFlag()
	cfg.ForwardedHeadersExcludeCIDRList = commaListFlag()
	cfg.CompressEncodings = commaListFlag("gzip", "deflate", "br")
	cfg.LuaModules = commaListFlag()
	cfg.LuaSources = commaListFlag()
	cfg.Oauth2GrantTokeninfoKeys = commaListFlag()

	flag := flag.NewFlagSet("", flag.ExitOnError)
	flag.StringVar(&cfg.ConfigFile, "config-file", "", "if provided the flags will be loaded/overwritten by the values on the file (yaml)")

	// generic:
	flag.StringVar(&cfg.Address, "address", ":9090", "network address that skipper should listen on")
	flag.StringVar(&cfg.InsecureAddress, "insecure-address", "", "insecure network address that skipper should listen on when TLS is enabled")
	flag.BoolVar(&cfg.EnableTCPQueue, "enable-tcp-queue", false, "enable the TCP listener queue")
	flag.IntVar(&cfg.ExpectedBytesPerRequest, "expected-bytes-per-request", 50*1024, "bytes per request, that is used to calculate concurrency limits to buffer connection spikes")
	flag.IntVar(&cfg.MaxTCPListenerConcurrency, "max-tcp-listener-concurrency", 0, "sets hardcoded max for TCP listener concurrency, normally calculated based on available memory cgroups with max TODO")
	flag.IntVar(&cfg.MaxTCPListenerQueue, "max-tcp-listener-queue", 0, "sets hardcoded max queue size for TCP listener, normally calculated 10x concurrency with max TODO:50k")
	flag.BoolVar(&cfg.EnableCopyStreamPoolExperimental, "enable-copy-stream-pool", false, "flag to use a pooled copy stream in the proxy. This is an optimization that is experimental and this option might disappear in the future")
	flag.BoolVar(&cfg.IgnoreTrailingSlash, "ignore-trailing-slash", false, "flag indicating to ignore trailing slashes in paths when routing")
	flag.BoolVar(&cfg.Insecure, "insecure", false, "flag indicating to ignore the verification of the TLS certificates of the backend services")
	flag.BoolVar(&cfg.ProxyPreserveHost, "proxy-preserve-host", false, "flag indicating to preserve the incoming request 'Host' header in the outgoing requests")
	flag.BoolVar(&cfg.DevMode, "dev-mode", false, "enables developer time behavior, like unbuffered routing updates")
	flag.StringVar(&cfg.SupportListener, "support-listener", ":9911", "network address used for exposing the /metrics endpoint. An empty value disables support endpoint.")
	flag.StringVar(&cfg.DebugListener, "debug-listener", "", "when this address is set, skipper starts an additional listener returning the original and transformed requests")
	flag.StringVar(&cfg.CertPathTLS, "tls-cert", "", "the path on the local filesystem to the certificate file(s) (including any intermediates), multiple may be given comma separated")
	flag.StringVar(&cfg.KeyPathTLS, "tls-key", "", "the path on the local filesystem to the certificate's private key file(s), multiple keys may be given comma separated - the order must match the certs")
	flag.Var(cfg.StatusChecks, "status-checks", "experimental URLs to check before reporting healthy on startup")
	flag.BoolVar(&cfg.PrintVersion, "version", false, "print Skipper version")
	flag.IntVar(&cfg.MaxLoopbacks, "max-loopbacks", proxy.DefaultMaxLoopbacks, "maximum number of loopbacks for an incoming request, set to -1 to disable loopbacks")
	flag.IntVar(&cfg.DefaultHTTPStatus, "default-http-status", http.StatusNotFound, "default HTTP status used when no route is found for a request")
	flag.StringVar(&cfg.PluginDir, "plugindir", "", "set the directory to load plugins from, default is ./")
	flag.DurationVar(&cfg.LoadBalancerHealthCheckInterval, "lb-healthcheck-interval", 0, "This is *deprecated* and not in use anymore")
	flag.BoolVar(&cfg.ReverseSourcePredicate, "reverse-source-predicate", false, "reverse the order of finding the client IP from X-Forwarded-For header")
	flag.BoolVar(&cfg.RemoveHopHeaders, "remove-hop-headers", false, "enables removal of Hop-Headers according to RFC-2616")
	flag.BoolVar(&cfg.RfcPatchPath, "rfc-patch-path", false, "patches the incoming request path to preserve uncoded reserved characters according to RFC 2616 and RFC 3986")
	flag.IntVar(&cfg.MaxAuditBody, "max-audit-body", 1024, "sets the max body to read to log in the audit log body")
	flag.Uint64Var(&cfg.MaxMatcherBufferSize, "max-matcher-buffer-size", 2097152, "sets the maximum read size of the body read by the block filter, default is 2MiB")
	flag.BoolVar(&cfg.EnableBreakers, "enable-breakers", false, enableBreakersUsage)
	flag.Var(&cfg.Breakers, "breaker", breakerUsage)
	flag.BoolVar(&cfg.EnableRatelimiters, "enable-ratelimits", false, enableRatelimitsUsage)
	flag.Var(&cfg.Ratelimits, "ratelimits", ratelimitsUsage)
	flag.BoolVar(&cfg.EnableRouteFIFOMetrics, "enable-route-fifo-metrics", false, "enable metrics for the individual route FIFO queues")
	flag.BoolVar(&cfg.EnableRouteLIFOMetrics, "enable-route-lifo-metrics", false, "enable metrics for the individual route LIFO queues")
	flag.Var(cfg.MetricsFlavour, "metrics-flavour", "Metrics flavour is used to change the exposed metrics format. Supported metric formats: 'codahale' and 'prometheus', you can select both of them by using one option with ',' separated values")
	flag.Var(cfg.FilterPlugins, "filter-plugin", "set a custom filter plugins to load, a comma separated list of name and arguments")
	flag.Var(cfg.PredicatePlugins, "predicate-plugin", "set a custom predicate plugins to load, a comma separated list of name and arguments")
	flag.Var(cfg.DataclientPlugins, "dataclient-plugin", "set a custom dataclient plugins to load, a comma separated list of name and arguments")
	flag.Var(cfg.MultiPlugins, "multi-plugin", "set a custom multitype plugins to load, a comma separated list of name and arguments")
	flag.Var(cfg.CompressEncodings, "compress-encodings", "set encodings supported for compression, the order defines priority when Accept-Header has equal quality values, see RFC 7231 section 5.3.1")

	// logging, metrics, tracing:
	flag.BoolVar(&cfg.EnablePrometheusMetrics, "enable-prometheus-metrics", false, "*Deprecated*: use metrics-flavour. Switch to Prometheus metrics format to expose metrics")
	flag.StringVar(&cfg.OpenTracing, "opentracing", "noop", "list of arguments for opentracing (space separated), first argument is the tracer implementation")
	flag.StringVar(&cfg.OpenTracingInitialSpan, "opentracing-initial-span", "ingress", "set the name of the initial, pre-routing, tracing span")
	flag.StringVar(&cfg.OpenTracingExcludedProxyTags, "opentracing-excluded-proxy-tags", "", "set tags that should be excluded from spans created for proxy operation. must be a comma-separated list of strings.")
	flag.BoolVar(&cfg.OpenTracingDisableFilterSpans, "opentracing-disable-filter-spans", false, "disable creation of spans representing request and response filters")
	flag.BoolVar(&cfg.OpentracingLogFilterLifecycleEvents, "opentracing-log-filter-lifecycle-events", true, "enables the logs for request & response filters' lifecycle events that are marking start & end times.")
	flag.BoolVar(&cfg.OpentracingLogStreamEvents, "opentracing-log-stream-events", true, "enables the logs for events marking the times response headers & payload are streamed to the client")
	flag.BoolVar(&cfg.OpentracingBackendNameTag, "opentracing-backend-name-tag", false, "enables an additional tracing tag that contains a backend name for a route when it's available  (e.g. for RouteGroups) (default false)")
	flag.StringVar(&cfg.MetricsListener, "metrics-listener", ":9911", "network address used for exposing the /metrics endpoint. An empty value disables metrics iff support listener is also empty.")
	flag.StringVar(&cfg.MetricsPrefix, "metrics-prefix", "skipper.", "allows setting a custom path prefix for metrics export")
	flag.BoolVar(&cfg.EnableProfile, "enable-profile", false, "enable profile information on the metrics endpoint with path /pprof")
	flag.IntVar(&cfg.BlockProfileRate, "block-profile-rate", 0, "block profile sample rate, see runtime.SetBlockProfileRate")
	flag.IntVar(&cfg.MutexProfileFraction, "mutex-profile-fraction", 0, "mutex profile fraction rate, see runtime.SetMutexProfileFraction")
	flag.IntVar(&cfg.MemProfileRate, "memory-profile-rate", 0, "memory profile rate, see runtime.MemProfileRate, keeps default 512 kB")
	flag.BoolVar(&cfg.EnablePrometheusStartLabel, "enable-prometheus-start-label", false, "adds start label to each prometheus counter with the value of counter creation timestamp as unix nanoseconds")
	flag.BoolVar(&cfg.DebugGcMetrics, "debug-gc-metrics", false, "enables reporting of the Go garbage collector statistics exported in debug.GCStats")
	flag.BoolVar(&cfg.RuntimeMetrics, "runtime-metrics", true, "enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats")
	flag.BoolVar(&cfg.ServeRouteMetrics, "serve-route-metrics", false, "enables reporting total serve time metrics for each route")
	flag.BoolVar(&cfg.ServeRouteCounter, "serve-route-counter", false, "enables reporting counting metrics for each route. Has the route, HTTP method and status code as labels. Currently just implemented for the Prometheus metrics flavour")
	flag.BoolVar(&cfg.ServeHostMetrics, "serve-host-metrics", false, "enables reporting total serve time metrics for each host")
	flag.BoolVar(&cfg.ServeHostCounter, "serve-host-counter", false, "enables reporting counting metrics for each host. Has the route, HTTP method and status code as labels. Currently just implemented for the Prometheus metrics flavour")
	flag.BoolVar(&cfg.ServeMethodMetric, "serve-method-metric", true, "enables the HTTP method as a domain of the total serve time metric. It affects both route and host split metrics")
	flag.BoolVar(&cfg.ServeStatusCodeMetric, "serve-status-code-metric", true, "enables the HTTP response status code as a domain of the total serve time metric. It affects both route and host split metrics")
	flag.BoolVar(&cfg.BackendHostMetrics, "backend-host-metrics", false, "enables reporting total serve time metrics for each backend")
	flag.BoolVar(&cfg.ProxyRequestMetrics, "proxy-request-metrics", false, "enables reporting latency / time spent in handling the request part of the proxy operation i.e., the duration from entry till before the backend round trip")
	flag.BoolVar(&cfg.ProxyResponseMetrics, "proxy-response-metrics", false, "enables reporting latency / time spent in handling the response part of the proxy operation i.e., the duration from after the backend round trip till the response is served")
	flag.BoolVar(&cfg.AllFiltersMetrics, "all-filters-metrics", false, "enables reporting combined filter metrics for each route")
	flag.BoolVar(&cfg.CombinedResponseMetrics, "combined-response-metrics", false, "enables reporting combined response time metrics")
	flag.BoolVar(&cfg.RouteResponseMetrics, "route-response-metrics", false, "enables reporting response time metrics for each route")
	flag.BoolVar(&cfg.RouteBackendErrorCounters, "route-backend-error-counters", false, "enables counting backend errors for each route")
	flag.BoolVar(&cfg.RouteStreamErrorCounters, "route-stream-error-counters", false, "enables counting streaming errors for each route")
	flag.BoolVar(&cfg.RouteBackendMetrics, "route-backend-metrics", false, "enables reporting backend response time metrics for each route")
	flag.BoolVar(&cfg.RouteCreationMetrics, "route-creation-metrics", false, "enables reporting for route creation times")
	flag.BoolVar(&cfg.MetricsUseExpDecaySample, "metrics-exp-decay-sample", false, "use exponentially decaying sample in metrics")
	flag.StringVar(&cfg.HistogramMetricBucketsString, "histogram-metric-buckets", "", "use custom buckets for prometheus histograms, must be a comma-separated list of numbers")
	flag.StringVar(&cfg.ResponseSizeBucketsString, "response-size-buckets", "", "use custom buckets for prometheus response size metrics, must be a comma-separated list of numbers")
	flag.StringVar(&cfg.RequestSizeBucketsString, "request-size-buckets", "", "use custom buckets for prometheus request header size metrics, must be a comma-separated list of numbers")
	flag.BoolVar(&cfg.DisableMetricsCompat, "disable-metrics-compat", false, "disables the default true value for all-filters-metrics, route-response-metrics, route-backend-errorCounters and route-stream-error-counters")
	flag.StringVar(&cfg.ApplicationLog, "application-log", "", "output file for the application log. When not set, /dev/stderr is used")
	flag.StringVar(&cfg.ApplicationLogLevelString, "application-log-level", "INFO", "log level for application logs, possible values: PANIC, FATAL, ERROR, WARN, INFO, DEBUG")
	flag.StringVar(&cfg.ApplicationLogPrefix, "application-log-prefix", "[APP]", "prefix for each log entry")
	flag.BoolVar(&cfg.ApplicationLogJSONEnabled, "application-log-json-enabled", false, "when this flag is set, log in JSON format is used")
	flag.StringVar(&cfg.AccessLog, "access-log", "", "output file for the access log, When not set, /dev/stderr is used")
	flag.BoolVar(&cfg.AccessLogDisabled, "access-log-disabled", false, "when this flag is set, no access log is printed")
	flag.BoolVar(&cfg.AccessLogJSONEnabled, "access-log-json-enabled", false, "when this flag is set, log in JSON format is used")
	flag.BoolVar(&cfg.AccessLogStripQuery, "access-log-strip-query", false, "when this flag is set, the access log strips the query strings from the access log")
	flag.BoolVar(&cfg.SuppressRouteUpdateLogs, "suppress-route-update-logs", false, "print only summaries on route updates/deletes")

	flag.Var(newYamlFlag(&cfg.OpenTelemetry), "open-telemetry", "OpenTelemetry configuration in YAML format, use flow-style for convenience")

	// route sources:
	flag.StringVar(&cfg.EtcdUrls, "etcd-urls", "", "urls of nodes in an etcd cluster, storing route definitions")
	flag.StringVar(&cfg.EtcdPrefix, "etcd-prefix", "/skipper", "path prefix for skipper related data in etcd")
	flag.DurationVar(&cfg.EtcdTimeout, "etcd-timeout", time.Second, "http client timeout duration for etcd")
	flag.BoolVar(&cfg.EtcdInsecure, "etcd-insecure", false, "ignore the verification of TLS certificates for etcd")
	flag.StringVar(&cfg.EtcdOAuthToken, "etcd-oauth-token", "", "optional token for OAuth authentication with etcd")
	flag.StringVar(&cfg.EtcdUsername, "etcd-username", "", "optional username for basic authentication with etcd")
	flag.StringVar(&cfg.EtcdPassword, "etcd-password", "", "optional password for basic authentication with etcd")
	flag.StringVar(&cfg.RoutesFile, "routes-file", "", "file containing route definitions")
	flag.Var(cfg.RoutesURLs, "routes-urls", "comma separated URLs to route definitions in eskip format")
	flag.StringVar(&cfg.InlineRoutes, "inline-routes", "", "inline routes in eskip format")
	flag.StringVar(&cfg.ForwardBackendURL, "forward-backend-url", "", "target url of the <forward> backend")
	flag.Int64Var(&cfg.SourcePollTimeout, "source-poll-timeout", int64(3000), "polling timeout of the routing data sources, in milliseconds")
	flag.Var(cfg.AppendFilters, "default-filters-append", "set of default filters to apply to append to all filters of all routes")
	flag.Var(cfg.PrependFilters, "default-filters-prepend", "set of default filters to apply to prepend to all filters of all routes")
	flag.Var(cfg.DisabledFilters, "disabled-filters", "comma separated list of filters unavailable for use")
	flag.Var(&cfg.EditRoute, "edit-route", "match and edit filters and predicates of all routes")
	flag.Var(&cfg.CloneRoute, "clone-route", "clone all matching routes and replace filters and predicates of all matched routes")
	flag.BoolVar(&cfg.WaitFirstRouteLoad, "wait-first-route-load", false, "prevent starting the listener before the first batch of routes were loaded")

	// Forwarded headers
	flag.Var(cfg.ForwardedHeadersList, "forwarded-headers", "comma separated list of headers to add to the incoming request before routing\n"+
		"X-Forwarded-For sets or appends with comma the remote IP of the request to the X-Forwarded-For header value\n"+
		"X-Forwarded-Host sets X-Forwarded-Host value to the request host\n"+
		"X-Forwarded-Method sets X-Forwarded-Method value to the request method\n"+
		"X-Forwarded-Uri sets X-Forwarded-Uri value to the requestURI\n"+
		"X-Forwarded-Port=<port> sets X-Forwarded-Port value\n"+
		"X-Forwarded-Proto=<http|https> sets X-Forwarded-Proto value")
	flag.Var(cfg.ForwardedHeadersExcludeCIDRList, "forwarded-headers-exclude-cidrs", "disables addition of forwarded headers for the remote host IPs from the comma separated list of CIDRs")

	flag.BoolVar(&cfg.NormalizeHost, "normalize-host", false, "converts request host to lowercase and removes port and trailing dot if any")
	flag.BoolVar(&cfg.ValidateQuery, "validate-query", true, "Validates the HTTP Query of a request and if invalid responds with status code 400")
	flag.BoolVar(&cfg.ValidateQueryLog, "validate-query-log", true, "Enable logging for validate query logs")
	flag.Int64Var(&cfg.MaxContentLength, "max-content-length", 0, "Limit the maximum Content-Length value")

	flag.Var(&cfg.RefusePayload, "refuse-payload", "refuse requests that match configured value. Can be set multiple times")

	// Kubernetes:
	flag.BoolVar(&cfg.KubernetesIngress, "kubernetes", false, "enables skipper to generate routes for ingress resources in kubernetes cluster. Enables -normalize-host")
	flag.BoolVar(&cfg.KubernetesInCluster, "kubernetes-in-cluster", false, "specify if skipper is running inside kubernetes cluster. It will automatically discover API server URL and service account token")
	flag.StringVar(&cfg.KubernetesURL, "kubernetes-url", "", "kubernetes API server URL, ignored if kubernetes-in-cluster is set to true")
	flag.StringVar(&cfg.KubernetesTokenFile, "kubernetes-token-file", "", "kubernetes token file path, ignored if kubernetes-in-cluster is set to true")
	flag.BoolVar(&cfg.KubernetesHealthcheck, "kubernetes-healthcheck", true, "automatic healthcheck route for internal IPs with path /kube-system/healthz; valid only with kubernetes")
	flag.BoolVar(&cfg.KubernetesHTTPSRedirect, "kubernetes-https-redirect", true, "automatic HTTP->HTTPS redirect route; valid only with kubernetes")
	flag.IntVar(&cfg.KubernetesHTTPSRedirectCode, "kubernetes-https-redirect-code", 308, "overrides the default redirect code (308) when used together with -kubernetes-https-redirect")
	flag.BoolVar(&cfg.KubernetesDisableCatchAllRoutes, "kubernetes-disable-catchall-routes", false, "disables creation of catchall routes")
	flag.StringVar(&cfg.KubernetesIngressClass, "kubernetes-ingress-class", "", "ingress class regular expression used to filter ingress resources for kubernetes")
	flag.StringVar(&cfg.KubernetesRouteGroupClass, "kubernetes-routegroup-class", "", "route group class regular expression used to filter route group resources for kubernetes")
	flag.StringVar(&cfg.WhitelistedHealthCheckCIDR, "whitelisted-healthcheck-cidr", "", "sets the iprange/CIDRS to be whitelisted during healthcheck")
	flag.StringVar(&cfg.KubernetesPathModeString, "kubernetes-path-mode", "kubernetes-ingress", "controls the default interpretation of Kubernetes ingress paths: <kubernetes-ingress|path-regexp|path-prefix>")
	flag.StringVar(&cfg.KubernetesNamespace, "kubernetes-namespace", "", "watch only this namespace for ingresses")
	flag.BoolVar(&cfg.KubernetesEnableEndpointSlices, "enable-kubernetes-endpointslices", false, "Enables that skipper fetches Kubernetes endpointslices instead of endpoints to scale more than 1000 pods within a service")
	flag.BoolVar(&cfg.KubernetesEnableEastWest, "enable-kubernetes-east-west", false, "*Deprecated*: use kubernetes-east-west-range feature. Enables east-west communication, which automatically adds routes for Ingress objects with hostname <name>.<namespace>.skipper.cluster.local")
	flag.StringVar(&cfg.KubernetesEastWestDomain, "kubernetes-east-west-domain", "", "*Deprecated*: use kubernetes-east-west-range feature. Sets the east-west domain, defaults to .skipper.cluster.local")
	flag.Var(cfg.KubernetesEastWestRangeDomains, "kubernetes-east-west-range-domains", "set the cluster internal domains for east west traffic. Identified routes to such domains will include the -kubernetes-east-west-range-predicates")
	flag.StringVar(&cfg.KubernetesEastWestRangePredicatesString, "kubernetes-east-west-range-predicates", "", "set the predicates that will be appended to routes identified as to -kubernetes-east-west-range-domains")
	flag.Var(&cfg.KubernetesAnnotationPredicatesString, "kubernetes-annotation-predicates", "configures predicates appended to non east-west routes of annotated resources. E.g. -kubernetes-annotation-predicates='zone-a=true=Foo() && Bar()' will add 'Foo() && Bar()' predicates to all non east-west routes of ingress or routegroup annotated with 'zone-a: true'. For east-west routes use -kubernetes-east-west-range-annotation-predicates.")
	flag.Var(&cfg.KubernetesAnnotationFiltersAppendString, "kubernetes-annotation-filters-append", "configures filters appended to non east-west routes of annotated resources. E.g. -kubernetes-annotation-filters-append='zone-a=true=foo() -> bar()' will add 'foo() -> bar()' filters to all non east-west routes of ingress or routegroup annotated with 'zone-a: true'. For east-west routes use -kubernetes-east-west-range-annotation-filters-append.")
	flag.Var(&cfg.KubernetesEastWestRangeAnnotationPredicatesString, "kubernetes-east-west-range-annotation-predicates", "similar to -kubernetes-annotation-predicates configures predicates appended to east-west routes of annotated resources. See also -kubernetes-east-west-range-domains.")
	flag.Var(&cfg.KubernetesEastWestRangeAnnotationFiltersAppendString, "kubernetes-east-west-range-annotation-filters-append", "similar to -kubernetes-annotation-filters-append configures filters appended to east-west routes of annotated resources. See also -kubernetes-east-west-range-domains.")
	flag.BoolVar(&cfg.EnableKubernetesExternalNames, "enable-kubernetes-external-names", false, "only if enabled we allow to use external name services as backends in Ingress")
	flag.BoolVar(&cfg.KubernetesOnlyAllowedExternalNames, "kubernetes-only-allowed-external-names", false, "only accept external name services, route group network backends and route group explicit LB endpoints from an allow list defined by zero or more -kubernetes-allowed-external-name flags")
	flag.Var(&cfg.KubernetesAllowedExternalNames, "kubernetes-allowed-external-name", "set zero or more regular expressions from which at least one should be matched by the external name services, route group network addresses and explicit endpoints domain names")
	flag.StringVar(&cfg.KubernetesRedisServiceNamespace, "kubernetes-redis-service-namespace", "", "Sets namespace for redis to be used to lookup endpoints")
	flag.StringVar(&cfg.KubernetesRedisServiceName, "kubernetes-redis-service-name", "", "Sets name for redis to be used to lookup endpoints")
	flag.IntVar(&cfg.KubernetesRedisServicePort, "kubernetes-redis-service-port", 6379, "Sets the port for redis to be used to lookup endpoints")
	flag.StringVar(&cfg.KubernetesValkeyServiceNamespace, "kubernetes-valkey-service-namespace", "", "Sets namespace for valkey to be used to lookup endpoints")
	flag.StringVar(&cfg.KubernetesValkeyServiceName, "kubernetes-valkey-service-name", "", "Sets name for valkey to be used to lookup endpoints")
	flag.IntVar(&cfg.KubernetesValkeyServicePort, "kubernetes-valkey-service-port", 6379, "Sets the port for valkey to be used to lookup endpoints")
	flag.StringVar(&cfg.KubernetesBackendTrafficAlgorithmString, "kubernetes-backend-traffic-algorithm", kubernetes.TrafficPredicateAlgorithm.String(), "sets the algorithm to be used for traffic splitting between backends: traffic-predicate or traffic-segment-predicate")
	flag.StringVar(&cfg.KubernetesDefaultLoadBalancerAlgorithm, "kubernetes-default-lb-algorithm", kubernetes.DefaultLoadBalancerAlgorithm, "sets the default algorithm to be used for load balancing between backend endpoints, available options: roundRobin, consistentHash, random, powerOfRandomNChoices")
	flag.BoolVar(&cfg.KubernetesForceService, "kubernetes-force-service", false, "overrides default Skipper functionality and routes traffic using Kubernetes Services instead of Endpoints")
	flag.StringVar(&cfg.KubernetesIngressStatusFromService, "kubernetes-ingress-status-from-service", "", "when set to <namespace>/<name>, updates ingress status.loadBalancer.ingress from the referenced service")

	// Auth:
	flag.BoolVar(&cfg.EnableOAuth2GrantFlow, "enable-oauth2-grant-flow", false, "enables OAuth2 Grant Flow filter")
	flag.StringVar(&cfg.Oauth2AuthURL, "oauth2-auth-url", "", "sets the OAuth2 Auth URL to redirect the requests to when login is required")
	flag.StringVar(&cfg.Oauth2TokenURL, "oauth2-token-url", "", "the url where the access code should be exchanged for the access token")
	flag.StringVar(&cfg.Oauth2RevokeTokenURL, "oauth2-revoke-token-url", "", "the url where the access and refresh tokens can be revoked when logging out")
	flag.StringVar(&cfg.Oauth2TokeninfoURL, "oauth2-tokeninfo-url", "", "sets the default tokeninfo URL to query information about an incoming OAuth2 token in oauth2Tokeninfo filters")
	flag.StringVar(&cfg.Oauth2SecretFile, "oauth2-secret-file", "", "sets the filename with the encryption key for the authentication cookie and grant flow state stored in secrets registry")
	flag.StringVar(&cfg.Oauth2ClientID, "oauth2-client-id", "", "sets the OAuth2 client id of the current service, used to exchange the access code. Falls back to env variable OAUTH2_CLIENT_ID if value is empty.")
	flag.StringVar(&cfg.Oauth2ClientSecret, "oauth2-client-secret", "", "sets the OAuth2 client secret associated with the oauth2-client-id, used to exchange the access code. Falls back to env variable OAUTH2_CLIENT_SECRET if value is empty.")
	flag.StringVar(&cfg.Oauth2ClientIDFile, "oauth2-client-id-file", "", "sets the path of the file containing the OAuth2 client id of the current service, used to exchange the access code. "+
		"File name may contain {host} placeholder which will be replaced by the request host")
	flag.StringVar(&cfg.Oauth2ClientSecretFile, "oauth2-client-secret-file", "", "sets the path of the file containing the OAuth2 client secret associated with the oauth2-client-id, used to exchange the access code. "+
		"File name may contain {host} placeholder which will be replaced by the request host")
	flag.StringVar(&cfg.Oauth2CallbackPath, "oauth2-callback-path", "", "sets the path where the OAuth2 callback requests with the authorization code should be redirected to")
	flag.DurationVar(&cfg.Oauth2TokeninfoTimeout, "oauth2-tokeninfo-timeout", 2*time.Second, "sets the default tokeninfo request timeout duration to 2000ms")
	flag.IntVar(&cfg.Oauth2TokeninfoCacheSize, "oauth2-tokeninfo-cache-size", 0, "non-zero value enables tokeninfo cache and sets the maximum number of cached tokens")
	flag.DurationVar(&cfg.Oauth2TokeninfoCacheTTL, "oauth2-tokeninfo-cache-ttl", 0, "non-zero value limits the lifetime of a cached tokeninfo which otherwise equals the tokeninfo 'expires_in' field value")
	flag.DurationVar(&cfg.Oauth2TokenintrospectionTimeout, "oauth2-tokenintrospect-timeout", 2*time.Second, "sets the default tokenintrospection request timeout duration to 2000ms")
	flag.Var(&cfg.Oauth2AuthURLParameters, "oauth2-auth-url-parameters", "sets additional parameters to send when calling the OAuth2 authorize or token endpoints as key-value pairs")
	flag.StringVar(&cfg.Oauth2AccessTokenHeaderName, "oauth2-access-token-header-name", "", "sets the access token to a header on the request with this name")
	flag.StringVar(&cfg.Oauth2TokeninfoSubjectKey, "oauth2-tokeninfo-subject-key", "uid", "sets the tokeninfo subject key")
	flag.Var(cfg.Oauth2GrantTokeninfoKeys, "oauth2-grant-tokeninfo-keys", "non-empty comma separated list configures keys to preserve in OAuth2 Grant Flow tokeninfo")
	flag.StringVar(&cfg.Oauth2TokenCookieName, "oauth2-token-cookie-name", "oauth2-grant", "sets the name of the cookie where the encrypted token is stored")
	flag.IntVar(&cfg.Oauth2TokenCookieRemoveSubdomains, "oauth2-token-cookie-remove-subdomains", 1, "sets the number of subdomains to remove from the callback request hostname to obtain token cookie domain")
	flag.BoolVar(&cfg.Oauth2GrantInsecure, "oauth2-grant-insecure", false, "omits Secure attribute of the token cookie and uses http scheme for callback url")
	flag.DurationVar(&cfg.WebhookTimeout, "webhook-timeout", 2*time.Second, "sets the webhook request timeout duration")
	flag.BoolVar(&cfg.ValidationWebhookEnabled, "validation-webhook-enabled", false, "enables validation webhook for incoming requests")
	flag.StringVar(&cfg.ValidationWebhookAddress, "validation-webhook-address", ":9000", "address of the validation webhook service")
	flag.StringVar(&cfg.ValidationWebhookCertFile, "validation-webhook-cert-file", "", "path to the certificate file for the validation webhook")
	flag.StringVar(&cfg.ValidationWebhookKeyFile, "validation-webhook-key-file", "", "path to the key file for the validation webhook")
	flag.BoolVar(&cfg.EnableAdvancedValidation, "enable-advanced-validation", false, "enables advanced validation logic for Kubernetes resources")

	flag.StringVar(&cfg.OidcSecretsFile, "oidc-secrets-file", "", "file storing the encryption key of the OID Connect token. Enables OIDC filters")
	flag.DurationVar(&cfg.OIDCCookieValidity, "oidc-cookie-validity", time.Hour, "sets the cookie expiry time to +1h for OIDC filters, when no 'exp' claim is found in the JWT token")
	flag.DurationVar(&cfg.OidcDistributedClaimsTimeout, "oidc-distributed-claims-timeout", 2*time.Second, "sets the default OIDC distributed claims request timeout duration to 2000ms")
	flag.IntVar(&cfg.OIDCCookieRemoveSubdomains, "oidc-cookie-remove-subdomains", 1, "sets the number of subdomains to remove from the callback request hostname to obtain token cookie domain")
	flag.Var(cfg.CredentialPaths, "credentials-paths", "directories or files to watch for credentials to use by bearerinjector filter")
	flag.DurationVar(&cfg.CredentialsUpdateInterval, "credentials-update-interval", 10*time.Minute, "sets the interval to update secrets")
	flag.BoolVar(&cfg.EnableOpenPolicyAgent, "enable-open-policy-agent", false, "enables Open Policy Agent filters")
	flag.BoolVar(&cfg.EnableOpenPolicyAgentCustomControlLoop, "enable-open-policy-agent-custom-control-loop", false, "when enabled skipper will use a custom control loop to orchestrate certain opa behaviour (like the download of new bundles) instead of relying on periodic plugin triggers")
	flag.BoolVar(&cfg.EnableOpenPolicyAgentPreloading, "enable-open-policy-agent-preloading", false, "EXPERIMENTAL: when enabled, OPA instances will be pre-loaded during route processing instead of during filter creation, making filter creation non-blocking")
	flag.DurationVar(&cfg.OpenPolicyAgentControlLoopInterval, "open-policy-agent-control-loop-interval", openpolicyagent.DefaultControlLoopInterval, "Interval between the execution of the control loop. Only applies if the custom control loop is enabled")
	flag.DurationVar(&cfg.OpenPolicyAgentControlLoopMaxJitter, "open-policy-agent-control-loop-max-jitter", openpolicyagent.DefaultControlLoopMaxJitter, "Maximum jitter to add to the control loop interval. Only applies if the custom control loop is enabled")
	flag.BoolVar(&cfg.EnableOpenPolicyAgentDataPreProcessingOptimization, "enable-open-policy-agent-data-preprocessing-optimization", false, "As a latency optimization, open policy agent will read values from in-memory storage as pre converted ASTs, removing conversion overhead at evaluation time. Currently experimental and if successful will be enabled by default")
	flag.StringVar(&cfg.OpenPolicyAgentConfigTemplate, "open-policy-agent-config-template", "", "file containing a template for an Open Policy Agent configuration file that is interpolated for each OPA filter instance")
	flag.StringVar(&cfg.OpenPolicyAgentEnvoyMetadata, "open-policy-agent-envoy-metadata", "", "JSON file containing meta-data passed as input for compatibility with Envoy policies in the format")
	flag.DurationVar(&cfg.OpenPolicyAgentCleanerInterval, "open-policy-agent-cleaner-interval", openpolicyagent.DefaultCleanIdlePeriod, "Duration in seconds to wait before cleaning up unused opa instances")
	flag.DurationVar(&cfg.OpenPolicyAgentStartupTimeout, "open-policy-agent-startup-timeout", openpolicyagent.DefaultOpaStartupTimeout, "Maximum duration in seconds to wait for the open policy agent to start up and if the custom control loop is enabled, how long to wait for the processing of each instance to finish (to f.ex. download updated bundles)")
	flag.Int64Var(&cfg.OpenPolicyAgentMaxRequestBodySize, "open-policy-agent-max-request-body-size", openpolicyagent.DefaultMaxRequestBodySize, "Maximum number of bytes from a http request body that are passed as input to the policy")
	flag.Int64Var(&cfg.OpenPolicyAgentRequestBodyBufferSize, "open-policy-agent-request-body-buffer-size", openpolicyagent.DefaultRequestBodyBufferSize, "Read buffer size for the request body")
	flag.Int64Var(&cfg.OpenPolicyAgentMaxMemoryBodyParsing, "open-policy-agent-max-memory-body-parsing", openpolicyagent.DefaultMaxMemoryBodyParsing, "Total number of bytes used to parse http request bodies across all requests. Once the limit is met, requests will be rejected.")

	// TLS client certs
	flag.StringVar(&cfg.ClientKeyFile, "client-tls-key", "", "TLS Key file for backend connections, multiple keys may be given comma separated - the order must match the certs")
	flag.StringVar(&cfg.ClientCertFile, "client-tls-cert", "", "TLS certificate files for backend connections, multiple keys may be given comma separated - the order must match the keys")

	// TLS version
	flag.StringVar(&cfg.TLSMinVersion, "tls-min-version", defaultMinTLSVersion, "minimal TLS Version to be used in server, proxy and client connections")
	flag.Func("tls-client-auth", "TLS client authentication policy for server, one of: "+
		"NoClientCert, RequestClientCert, RequireAnyClientCert, VerifyClientCertIfGiven or RequireAndVerifyClientCert. "+
		"See https://pkg.go.dev/crypto/tls#ClientAuthType for details.", cfg.setTLSClientAuth)

	// Exclude insecure cipher suites
	flag.BoolVar(&cfg.ExcludeInsecureCipherSuites, "exclude-insecure-cipher-suites", false, "excludes insecure cipher suites")

	// API Monitoring:
	flag.BoolVar(&cfg.ApiUsageMonitoringEnable, "enable-api-usage-monitoring", false, "enables the apiUsageMonitoring filter")
	flag.StringVar(&cfg.ApiUsageMonitoringRealmKeys, "api-usage-monitoring-realm-keys", "", "name of the property in the JWT payload that contains the authority realm")
	flag.StringVar(&cfg.ApiUsageMonitoringClientKeys, "api-usage-monitoring-client-keys", "sub", "comma separated list of names of the properties in the JWT body that contains the client ID")
	flag.StringVar(&cfg.ApiUsageMonitoringDefaultClientTrackingPattern, "api-usage-monitoring-default-client-tracking-pattern", "", "*Deprecated*: set `client_tracking_pattern` directly on filter")
	flag.StringVar(&cfg.ApiUsageMonitoringRealmsTrackingPattern, "api-usage-monitoring-realms-tracking-pattern", "services", "regular expression used for matching monitored realms (defaults is 'services')")

	// Default filters:
	flag.StringVar(&cfg.DefaultFiltersDir, "default-filters-dir", "", "path to directory which contains default filter configurations per service and namespace (disabled if not set)")

	// Connections, timeouts:
	flag.DurationVar(&cfg.WaitForHealthcheckInterval, "wait-for-healthcheck-interval", (10+5)*3*time.Second, "period waiting to become unhealthy in the loadbalancer pool in front of this instance, before shutdown triggered by SIGINT or SIGTERM") // kube-ingress-aws-controller default
	flag.IntVar(&cfg.IdleConnsPerHost, "idle-conns-num", proxy.DefaultIdleConnsPerHost, "maximum idle connections per backend host")
	flag.DurationVar(&cfg.CloseIdleConnsPeriod, "close-idle-conns-period", proxy.DefaultCloseIdleConnsPeriod, "sets the time interval of closing all idle connections. Not closing when 0")
	flag.DurationVar(&cfg.BackendFlushInterval, "backend-flush-interval", 20*time.Millisecond, "flush interval for upgraded proxy connections")
	flag.BoolVar(&cfg.ExperimentalUpgrade, "experimental-upgrade", false, "enable experimental feature to handle upgrade protocol requests")
	flag.BoolVar(&cfg.ExperimentalUpgradeAudit, "experimental-upgrade-audit", false, "enable audit logging of the request line and the messages during the experimental web socket upgrades")
	flag.DurationVar(&cfg.ReadTimeoutServer, "read-timeout-server", 5*time.Minute, "set ReadTimeout for http server connections")
	flag.DurationVar(&cfg.ReadHeaderTimeoutServer, "read-header-timeout-server", 60*time.Second, "set ReadHeaderTimeout for http server connections")
	flag.DurationVar(&cfg.WriteTimeoutServer, "write-timeout-server", 60*time.Second, "set WriteTimeout for http server connections")
	flag.DurationVar(&cfg.IdleTimeoutServer, "idle-timeout-server", 60*time.Second, "set IdleTimeout for http server connections")
	flag.DurationVar(&cfg.KeepaliveServer, "keepalive-server", 0*time.Second, "sets maximum age for http server connections. The connection is closed after it existed for this duration. Default is 0 for unlimited.")
	flag.IntVar(&cfg.KeepaliveRequestsServer, "keepalive-requests-server", 0, "sets maximum number of requests for http server connections. The connection is closed after serving this number of requests. Default is 0 for unlimited.")
	flag.IntVar(&cfg.MaxHeaderBytes, "max-header-bytes", http.DefaultMaxHeaderBytes, "set MaxHeaderBytes for http server connections")
	flag.BoolVar(&cfg.EnableConnMetricsServer, "enable-connection-metrics", false, "enables connection metrics for http server connections")
	flag.DurationVar(&cfg.TimeoutBackend, "timeout-backend", 60*time.Second, "sets the TCP client connection timeout for backend connections")
	flag.DurationVar(&cfg.KeepaliveBackend, "keepalive-backend", 30*time.Second, "sets the keepalive for backend connections")
	flag.BoolVar(&cfg.EnableDualstackBackend, "enable-dualstack-backend", true, "enables DualStack for backend connections")
	flag.DurationVar(&cfg.TlsHandshakeTimeoutBackend, "tls-timeout-backend", 60*time.Second, "sets the TLS handshake timeout for backend connections")
	flag.DurationVar(&cfg.ResponseHeaderTimeoutBackend, "response-header-timeout-backend", 60*time.Second, "sets the HTTP response header timeout for backend connections")
	flag.DurationVar(&cfg.ExpectContinueTimeoutBackend, "expect-continue-timeout-backend", 30*time.Second, "sets the HTTP expect continue timeout for backend connections")
	flag.IntVar(&cfg.MaxIdleConnsBackend, "max-idle-connection-backend", 0, "sets the maximum idle connections for all backend connections")
	flag.BoolVar(&cfg.DisableHTTPKeepalives, "disable-http-keepalives", false, "forces backend to always create a new connection")
	flag.BoolVar(&cfg.KubernetesEnableTLS, "kubernetes-enable-tls", false, "enable using kubernetes resources to terminate tls")

	// Swarm:
	flag.BoolVar(&cfg.EnableSwarm, "enable-swarm", false, "enable swarm communication between nodes in a skipper fleet")
	// redis
	flag.Var(cfg.SwarmRedisURLs, "swarm-redis-urls", "Redis URLs as comma separated list, used for building a swarm, for example in redis based cluster ratelimits.\nUse "+redisPasswordEnv+" environment variable or 'swarm-redis-password' key in config file to set redis password")
	flag.StringVar(&cfg.SwarmRedisHashAlgorithm, "swarm-redis-hash-algorithm", "", "sets hash algorithm to be used in redis ring client to find the shard <jump|mpchash|rendezvous|rendezvousVnodes>, defaults to github.com/redis/go-redis default")
	flag.DurationVar(&cfg.SwarmRedisDialTimeout, "swarm-redis-dial-timeout", net.DefaultDialTimeout, "set redis client dial timeout")
	flag.DurationVar(&cfg.SwarmRedisReadTimeout, "swarm-redis-read-timeout", net.DefaultReadTimeout, "set redis socket read timeout")
	flag.DurationVar(&cfg.SwarmRedisWriteTimeout, "swarm-redis-write-timeout", net.DefaultWriteTimeout, "set redis socket write timeout")
	flag.DurationVar(&cfg.SwarmRedisPoolTimeout, "swarm-redis-pool-timeout", net.DefaultPoolTimeout, "set redis get connection from pool timeout")
	flag.IntVar(&cfg.SwarmRedisMinConns, "swarm-redis-min-conns", net.DefaultMinConns, "set min number of connections to redis")
	flag.IntVar(&cfg.SwarmRedisMaxConns, "swarm-redis-max-conns", net.DefaultMaxConns, "set max number of connections to redis")
	flag.StringVar(&cfg.SwarmRedisEndpointsRemoteURL, "swarm-redis-remote", "", "Remote URL to pull redis endpoints from.")
	flag.DurationVar(&cfg.SwarmRedisUpdateInterval, "swarm-redis-update-interval", net.DefaultUpdateInterval, "set update interval to update redis addresses")
	flag.DurationVar(&cfg.SwarmRedisHeartbeatFrequency, "swarm-redis-heartbeat-frequency", net.DefaultHeartbeatFrequency, "set redis heartbeat frequency")
	// valkey
	flag.Var(cfg.SwarmValkeyURLs, "swarm-valkey-urls", "Valkey URLs as comma separated list, used for building a swarm, for example in valkey based cluster ratelimits.\nUse "+valkeyPasswordEnv+" environment variable or 'swarm-valkey-password' key in config file to set valkey password")
	flag.StringVar(&cfg.SwarmValkeyEndpointsRemoteURL, "swarm-valkey-remote", "", "Remote URL to pull valkey endpoints from.")
	flag.DurationVar(&cfg.SwarmValkeyConnLifetime, "swarm-valkey-conn-lifetime", net.DefaultConnLifeTime, "set valkey client connection life time")
	flag.DurationVar(&cfg.SwarmValkeyConnWriteTimeout, "swarm-valkey-conn-timeout", net.DefaultConnWriteTimeout, "set valkey client timeout for connect,read,write")
	flag.DurationVar(&cfg.SwarmValkeyUpdateInterval, "swarm-valkey-update-interval", net.DefaultUpdateInterval, "set update interval to update valkey addresses")
	// swim
	flag.StringVar(&cfg.SwarmKubernetesNamespace, "swarm-namespace", swarm.DefaultNamespace, "Kubernetes namespace to find swarm peer instances")
	flag.StringVar(&cfg.SwarmKubernetesLabelSelectorKey, "swarm-label-selector-key", swarm.DefaultLabelSelectorKey, "Kubernetes labelselector key to find swarm peer instances")
	flag.StringVar(&cfg.SwarmKubernetesLabelSelectorValue, "swarm-label-selector-value", swarm.DefaultLabelSelectorValue, "Kubernetes labelselector value to find swarm peer instances")
	flag.IntVar(&cfg.SwarmPort, "swarm-port", swarm.DefaultPort, "swarm port to use to communicate with our peers")
	flag.IntVar(&cfg.SwarmMaxMessageBuffer, "swarm-max-msg-buffer", swarm.DefaultMaxMessageBuffer, "swarm max message buffer size to use for member list messages")
	flag.DurationVar(&cfg.SwarmLeaveTimeout, "swarm-leave-timeout", swarm.DefaultLeaveTimeout, "swarm leave timeout to use for leaving the memberlist on timeout")
	flag.StringVar(&cfg.SwarmStaticSelf, "swarm-static-self", "", "set static swarm self node, for example 127.0.0.1:9001")
	flag.StringVar(&cfg.SwarmStaticOther, "swarm-static-other", "", "set static swarm all nodes, for example 127.0.0.1:9002,127.0.0.1:9003")

	flag.IntVar(&cfg.ClusterRatelimitMaxGroupShards, "cluster-ratelimit-max-group-shards", 1, "sets the maximum number of group shards for the clusterRatelimit filter")

	flag.BoolVar(&cfg.EnableLua, "enable-lua", false, "enable the Lua scripting engine to be able to use the lua() filter")
	flag.Var(cfg.LuaModules, "lua-modules", "comma separated list of lua filter modules. Use <module>.<symbol> to selectively enable module symbols, for example: package,base._G,base.print,json")
	flag.Var(cfg.LuaSources, "lua-sources", `comma separated list of lua input types for the lua() filter. Valid sources "", "file", "inline", "file,inline" and "none". Use "file" to only allow lua file references in lua filter. Default "" is the same as "file","inline". Use "none" to disable lua filters.`)

	// Passive Health Checks
	flag.Var(&cfg.PassiveHealthCheck, "passive-health-check", "sets the parameters for passive health check feature")

	cfg.Flags = flag
	return cfg
}

func validate(c *Config) error {
	_, err := log.ParseLevel(c.ApplicationLogLevelString)
	if err != nil {
		return err
	}
	_, err = kubernetes.ParsePathMode(c.KubernetesPathModeString)
	if err != nil {
		return err
	}
	_, err = eskip.ParsePredicates(c.KubernetesEastWestRangePredicatesString)
	if err != nil {
		return fmt.Errorf("invalid east-west-range-predicates: %w", err)
	}

	_, err = parseAnnotationPredicates(c.KubernetesAnnotationPredicatesString)
	if err != nil {
		return fmt.Errorf("invalid annotation predicates: %q, %w", c.KubernetesAnnotationPredicatesString, err)
	}

	_, err = parseAnnotationFilters(c.KubernetesAnnotationFiltersAppendString)
	if err != nil {
		return fmt.Errorf("invalid annotation filters: %q, %w", c.KubernetesAnnotationFiltersAppendString, err)
	}

	_, err = parseAnnotationPredicates(c.KubernetesEastWestRangeAnnotationPredicatesString)
	if err != nil {
		return fmt.Errorf("invalid east-west annotation predicates: %q, %w", c.KubernetesEastWestRangeAnnotationPredicatesString, err)
	}

	_, err = parseAnnotationFilters(c.KubernetesEastWestRangeAnnotationFiltersAppendString)
	if err != nil {
		return fmt.Errorf("invalid east-west annotation filters: %q, %w", c.KubernetesEastWestRangeAnnotationFiltersAppendString, err)
	}

	_, err = kubernetes.ParseBackendTrafficAlgorithm(c.KubernetesBackendTrafficAlgorithmString)
	if err != nil {
		return err
	}
	_, err = c.parseHistogramBuckets(c.HistogramMetricBucketsString, prometheus.DefBuckets)
	if err != nil {
		return err
	}
	_, err = c.parseHistogramBuckets(c.ResponseSizeBucketsString, metrics.DefaultResponseSizeBuckets)
	if err != nil {
		return err
	}
	_, err = c.parseHistogramBuckets(c.RequestSizeBucketsString, metrics.DefaultRequestSizeBuckets)
	if err != nil {
		return err
	}
	return c.parseForwardedHeaders()
}

func (c *Config) Parse() error {
	return c.ParseArgs(os.Args[0], os.Args[1:])
}

func (c *Config) ParseArgs(progname string, args []string) error {
	c.Flags.Init(progname, flag.ExitOnError)
	err := c.Flags.Parse(args)
	if err != nil {
		return err
	}

	// check if arguments were correctly parsed.
	if len(c.Flags.Args()) != 0 {
		return fmt.Errorf("invalid arguments: %s", c.Flags.Args())
	}

	configKeys := make(map[string]interface{})
	if c.ConfigFile != "" {
		yamlFile, err := os.ReadFile(c.ConfigFile)
		if err != nil {
			return fmt.Errorf("invalid config file: %w", err)
		}

		err = yaml.Unmarshal(yamlFile, c)
		if err != nil {
			return fmt.Errorf("unmarshalling config file error: %w", err)
		}

		_ = yaml.Unmarshal(yamlFile, configKeys)

		err = c.Flags.Parse(args)
		if err != nil {
			return err
		}
	}

	c.checkDeprecated(configKeys,
		"enable-prometheus-metrics",
		"api-usage-monitoring-default-client-tracking-pattern",
		"enable-kubernetes-east-west",
		"kubernetes-east-west-domain",
		"lb-healthcheck-interval",
	)

	if err := validate(c); err != nil {
		return err
	}

	c.ApplicationLogLevel, _ = log.ParseLevel(c.ApplicationLogLevelString)
	c.KubernetesPathMode, _ = kubernetes.ParsePathMode(c.KubernetesPathModeString)
	c.KubernetesEastWestRangePredicates, _ = eskip.ParsePredicates(c.KubernetesEastWestRangePredicatesString)
	c.KubernetesAnnotationPredicates, _ = parseAnnotationPredicates(c.KubernetesAnnotationPredicatesString)
	c.KubernetesAnnotationFiltersAppend, _ = parseAnnotationFilters(c.KubernetesAnnotationFiltersAppendString)
	c.KubernetesEastWestRangeAnnotationPredicates, _ = parseAnnotationPredicates(c.KubernetesEastWestRangeAnnotationPredicatesString)
	c.KubernetesEastWestRangeAnnotationFiltersAppend, _ = parseAnnotationFilters(c.KubernetesEastWestRangeAnnotationFiltersAppendString)
	c.KubernetesBackendTrafficAlgorithm, _ = kubernetes.ParseBackendTrafficAlgorithm(c.KubernetesBackendTrafficAlgorithmString)
	c.HistogramMetricBuckets, _ = c.parseHistogramBuckets(c.HistogramMetricBucketsString, prometheus.DefBuckets)
	c.ResponseSizeBuckets, _ = c.parseHistogramBuckets(c.ResponseSizeBucketsString, metrics.DefaultResponseSizeBuckets)
	c.RequestSizeBuckets, _ = c.parseHistogramBuckets(c.RequestSizeBucketsString, metrics.DefaultRequestSizeBuckets)

	if c.ClientKeyFile != "" && c.ClientCertFile != "" {
		certsFiles := strings.Split(c.ClientCertFile, ",")
		keyFiles := strings.Split(c.ClientKeyFile, ",")

		var certificates []tls.Certificate
		for i := range keyFiles {
			certificate, err := tls.LoadX509KeyPair(certsFiles[i], keyFiles[i])
			if err != nil {
				return fmt.Errorf("invalid key/cert pair: %w", err)
			}

			certificates = append(certificates, certificate)
		}

		c.Certificates = certificates
	}

	if c.NormalizeHost || c.KubernetesIngress {
		c.HostPatch = net.HostPatch{
			ToLower:           true,
			RemovePort:        true,
			RemoteTrailingDot: true,
		}
	}

	c.parseEnv()
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
		Address:                          c.Address,
		InsecureAddress:                  c.InsecureAddress,
		StatusChecks:                     c.StatusChecks.values,
		EnableTCPQueue:                   c.EnableTCPQueue,
		ExpectedBytesPerRequest:          c.ExpectedBytesPerRequest,
		MaxTCPListenerConcurrency:        c.MaxTCPListenerConcurrency,
		MaxTCPListenerQueue:              c.MaxTCPListenerQueue,
		EnableCopyStreamPoolExperimental: c.EnableCopyStreamPoolExperimental,
		IgnoreTrailingSlash:              c.IgnoreTrailingSlash,
		DevMode:                          c.DevMode,
		SupportListener:                  c.SupportListener,
		DebugListener:                    c.DebugListener,
		CertPathTLS:                      c.CertPathTLS,
		KeyPathTLS:                       c.KeyPathTLS,
		TLSClientAuth:                    c.TLSClientAuth,
		TLSMinVersion:                    c.getMinTLSVersion(),
		CipherSuites:                     c.filterCipherSuites(),
		MaxLoopbacks:                     c.MaxLoopbacks,
		DefaultHTTPStatus:                c.DefaultHTTPStatus,
		ReverseSourcePredicate:           c.ReverseSourcePredicate,
		MaxAuditBody:                     c.MaxAuditBody,
		MaxMatcherBufferSize:             c.MaxMatcherBufferSize,
		EnableBreakers:                   c.EnableBreakers,
		BreakerSettings:                  c.Breakers,
		EnableRatelimiters:               c.EnableRatelimiters,
		RatelimitSettings:                c.Ratelimits,
		EnableRouteFIFOMetrics:           c.EnableRouteFIFOMetrics,
		EnableRouteLIFOMetrics:           c.EnableRouteLIFOMetrics,
		MetricsFlavours:                  c.MetricsFlavour.values,
		FilterPlugins:                    c.FilterPlugins.values,
		PredicatePlugins:                 c.PredicatePlugins.values,
		DataClientPlugins:                c.DataclientPlugins.values,
		Plugins:                          c.MultiPlugins.values,
		PluginDirs:                       []string{skipper.DefaultPluginDir},
		CompressEncodings:                c.CompressEncodings.values,

		// logging, metrics, profiling, tracing:
		EnablePrometheusMetrics:             c.EnablePrometheusMetrics,
		EnablePrometheusStartLabel:          c.EnablePrometheusStartLabel,
		OpenTracing:                         strings.Split(c.OpenTracing, " "),
		OpenTracingInitialSpan:              c.OpenTracingInitialSpan,
		OpenTracingExcludedProxyTags:        strings.Split(c.OpenTracingExcludedProxyTags, ","),
		OpenTracingDisableFilterSpans:       c.OpenTracingDisableFilterSpans,
		OpenTracingLogStreamEvents:          c.OpentracingLogStreamEvents,
		OpenTracingLogFilterLifecycleEvents: c.OpentracingLogFilterLifecycleEvents,
		MetricsListener:                     c.MetricsListener,
		MetricsPrefix:                       c.MetricsPrefix,
		EnableProfile:                       c.EnableProfile,
		BlockProfileRate:                    c.BlockProfileRate,
		MutexProfileFraction:                c.MutexProfileFraction,
		EnableDebugGcMetrics:                c.DebugGcMetrics,
		EnableRuntimeMetrics:                c.RuntimeMetrics,
		EnableServeRouteMetrics:             c.ServeRouteMetrics,
		EnableServeRouteCounter:             c.ServeRouteCounter,
		EnableServeHostMetrics:              c.ServeHostMetrics,
		EnableServeHostCounter:              c.ServeHostCounter,
		EnableServeMethodMetric:             c.ServeMethodMetric,
		EnableServeStatusCodeMetric:         c.ServeStatusCodeMetric,
		EnableProxyRequestMetrics:           c.ProxyRequestMetrics,
		EnableProxyResponseMetrics:          c.ProxyResponseMetrics,
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
		ResponseSizeBuckets:                 c.ResponseSizeBuckets,
		RequestSizeBuckets:                  c.RequestSizeBuckets,
		DisableMetricsCompatibilityDefaults: c.DisableMetricsCompat,
		ApplicationLogOutput:                c.ApplicationLog,
		ApplicationLogPrefix:                c.ApplicationLogPrefix,
		ApplicationLogJSONEnabled:           c.ApplicationLogJSONEnabled,
		AccessLogOutput:                     c.AccessLog,
		AccessLogDisabled:                   c.AccessLogDisabled,
		AccessLogJSONEnabled:                c.AccessLogJSONEnabled,
		AccessLogStripQuery:                 c.AccessLogStripQuery,
		SuppressRouteUpdateLogs:             c.SuppressRouteUpdateLogs,

		OpenTelemetry: c.OpenTelemetry,

		// route sources:
		EtcdUrls:          eus,
		EtcdPrefix:        c.EtcdPrefix,
		EtcdWaitTimeout:   c.EtcdTimeout,
		EtcdInsecure:      c.EtcdInsecure,
		EtcdOAuthToken:    c.EtcdOAuthToken,
		EtcdUsername:      c.EtcdUsername,
		EtcdPassword:      c.EtcdPassword,
		WatchRoutesFile:   c.RoutesFile,
		RoutesURLs:        c.RoutesURLs.values,
		InlineRoutes:      c.InlineRoutes,
		ForwardBackendURL: c.ForwardBackendURL,
		DefaultFilters: &eskip.DefaultFilters{
			Prepend: c.PrependFilters.filters,
			Append:  c.AppendFilters.filters,
		},
		DisabledFilters:    c.DisabledFilters.values,
		SourcePollTimeout:  time.Duration(c.SourcePollTimeout) * time.Millisecond,
		WaitFirstRouteLoad: c.WaitFirstRouteLoad,

		// Kubernetes:
		Kubernetes:                                     c.KubernetesIngress,
		KubernetesInCluster:                            c.KubernetesInCluster,
		KubernetesURL:                                  c.KubernetesURL,
		KubernetesTokenFile:                            c.KubernetesTokenFile,
		KubernetesHealthcheck:                          c.KubernetesHealthcheck,
		KubernetesHTTPSRedirect:                        c.KubernetesHTTPSRedirect,
		KubernetesHTTPSRedirectCode:                    c.KubernetesHTTPSRedirectCode,
		KubernetesDisableCatchAllRoutes:                c.KubernetesDisableCatchAllRoutes,
		KubernetesIngressClass:                         c.KubernetesIngressClass,
		KubernetesRouteGroupClass:                      c.KubernetesRouteGroupClass,
		WhitelistedHealthCheckCIDR:                     whitelistCIDRS,
		KubernetesPathMode:                             c.KubernetesPathMode,
		KubernetesNamespace:                            c.KubernetesNamespace,
		KubernetesEnableEndpointslices:                 c.KubernetesEnableEndpointSlices,
		KubernetesEnableEastWest:                       c.KubernetesEnableEastWest,
		KubernetesEastWestDomain:                       c.KubernetesEastWestDomain,
		KubernetesEastWestRangeDomains:                 c.KubernetesEastWestRangeDomains.values,
		KubernetesEastWestRangePredicates:              c.KubernetesEastWestRangePredicates,
		KubernetesEastWestRangeAnnotationPredicates:    c.KubernetesEastWestRangeAnnotationPredicates,
		KubernetesEastWestRangeAnnotationFiltersAppend: c.KubernetesEastWestRangeAnnotationFiltersAppend,
		KubernetesAnnotationPredicates:                 c.KubernetesAnnotationPredicates,
		KubernetesAnnotationFiltersAppend:              c.KubernetesAnnotationFiltersAppend,
		EnableKubernetesExternalNames:                  c.EnableKubernetesExternalNames,
		KubernetesOnlyAllowedExternalNames:             c.KubernetesOnlyAllowedExternalNames,
		KubernetesAllowedExternalNames:                 c.KubernetesAllowedExternalNames,
		KubernetesRedisServiceNamespace:                c.KubernetesRedisServiceNamespace,
		KubernetesRedisServiceName:                     c.KubernetesRedisServiceName,
		KubernetesRedisServicePort:                     c.KubernetesRedisServicePort,
		KubernetesValkeyServiceNamespace:               c.KubernetesValkeyServiceNamespace,
		KubernetesValkeyServiceName:                    c.KubernetesValkeyServiceName,
		KubernetesValkeyServicePort:                    c.KubernetesValkeyServicePort,
		KubernetesBackendTrafficAlgorithm:              c.KubernetesBackendTrafficAlgorithm,
		KubernetesDefaultLoadBalancerAlgorithm:         c.KubernetesDefaultLoadBalancerAlgorithm,
		KubernetesForceService:                         c.KubernetesForceService,
		KubernetesIngressStatusFromService:             c.KubernetesIngressStatusFromService,

		// API Monitoring:
		ApiUsageMonitoringEnable:                c.ApiUsageMonitoringEnable,
		ApiUsageMonitoringRealmKeys:             c.ApiUsageMonitoringRealmKeys,
		ApiUsageMonitoringClientKeys:            c.ApiUsageMonitoringClientKeys,
		ApiUsageMonitoringRealmsTrackingPattern: c.ApiUsageMonitoringRealmsTrackingPattern,

		// Default filters:
		DefaultFiltersDir: c.DefaultFiltersDir,

		// Auth:
		EnableOAuth2GrantFlow:             c.EnableOAuth2GrantFlow,
		OAuth2AuthURL:                     c.Oauth2AuthURL,
		OAuth2TokenURL:                    c.Oauth2TokenURL,
		OAuth2RevokeTokenURL:              c.Oauth2RevokeTokenURL,
		OAuthTokeninfoURL:                 c.Oauth2TokeninfoURL,
		OAuthTokeninfoTimeout:             c.Oauth2TokeninfoTimeout,
		OAuthTokeninfoCacheSize:           c.Oauth2TokeninfoCacheSize,
		OAuthTokeninfoCacheTTL:            c.Oauth2TokeninfoCacheTTL,
		OAuth2SecretFile:                  c.Oauth2SecretFile,
		OAuth2ClientID:                    c.Oauth2ClientID,
		OAuth2ClientSecret:                c.Oauth2ClientSecret,
		OAuth2ClientIDFile:                c.Oauth2ClientIDFile,
		OAuth2ClientSecretFile:            c.Oauth2ClientSecretFile,
		OAuth2CallbackPath:                c.Oauth2CallbackPath,
		OAuthTokenintrospectionTimeout:    c.Oauth2TokenintrospectionTimeout,
		OAuth2AuthURLParameters:           c.Oauth2AuthURLParameters.values,
		OAuth2AccessTokenHeaderName:       c.Oauth2AccessTokenHeaderName,
		OAuth2TokeninfoSubjectKey:         c.Oauth2TokeninfoSubjectKey,
		OAuth2GrantTokeninfoKeys:          c.Oauth2GrantTokeninfoKeys.values,
		OAuth2TokenCookieName:             c.Oauth2TokenCookieName,
		OAuth2TokenCookieRemoveSubdomains: c.Oauth2TokenCookieRemoveSubdomains,
		OAuth2GrantInsecure:               c.Oauth2GrantInsecure,
		WebhookTimeout:                    c.WebhookTimeout,
		OIDCSecretsFile:                   c.OidcSecretsFile,
		OIDCCookieValidity:                c.OIDCCookieValidity,
		OIDCDistributedClaimsTimeout:      c.OidcDistributedClaimsTimeout,
		OIDCCookieRemoveSubdomains:        c.OIDCCookieRemoveSubdomains,
		CredentialsPaths:                  c.CredentialPaths.values,
		CredentialsUpdateInterval:         c.CredentialsUpdateInterval,
		ValidationWebhookEnabled:          c.ValidationWebhookEnabled,
		ValidationWebhookAddress:          c.ValidationWebhookAddress,
		ValidationWebhookCertFile:         c.ValidationWebhookCertFile,
		ValidationWebhookKeyFile:          c.ValidationWebhookKeyFile,
		EnableAdvancedValidation:          c.EnableAdvancedValidation,

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
		KeepaliveServer:              c.KeepaliveServer,
		KeepaliveRequestsServer:      c.KeepaliveRequestsServer,
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
		KubernetesEnableTLS:          c.KubernetesEnableTLS,

		// swarm:
		EnableSwarm: c.EnableSwarm,
		// redis based
		SwarmRedisURLs:               c.SwarmRedisURLs.values,
		SwarmRedisUsername:           c.SwarmRedisUsername,
		SwarmRedisPassword:           c.SwarmRedisPassword,
		SwarmRedisHashAlgorithm:      c.SwarmRedisHashAlgorithm,
		SwarmRedisDialTimeout:        c.SwarmRedisDialTimeout,
		SwarmRedisReadTimeout:        c.SwarmRedisReadTimeout,
		SwarmRedisWriteTimeout:       c.SwarmRedisWriteTimeout,
		SwarmRedisPoolTimeout:        c.SwarmRedisPoolTimeout,
		SwarmRedisMinIdleConns:       c.SwarmRedisMinConns,
		SwarmRedisMaxIdleConns:       c.SwarmRedisMaxConns,
		SwarmRedisEndpointsRemoteURL: c.SwarmRedisEndpointsRemoteURL,
		SwarmRedisUpdateInterval:     c.SwarmRedisUpdateInterval,
		SwarmRedisHeartbeatFrequency: c.SwarmRedisHeartbeatFrequency,
		// valkey based
		SwarmValkeyURLs:               c.SwarmValkeyURLs.values,
		SwarmValkeyEndpointsRemoteURL: c.SwarmValkeyEndpointsRemoteURL,
		SwarmValkeyUsername:           c.SwarmValkeyUsername,
		SwarmValkeyPassword:           c.SwarmValkeyPassword,
		SwarmValkeyConnLifetime:       c.SwarmValkeyConnLifetime,
		SwarmValkeyConnWriteTimeout:   c.SwarmValkeyConnWriteTimeout,
		SwarmValkeyUpdateInterval:     c.SwarmValkeyUpdateInterval,
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

		ClusterRatelimitMaxGroupShards: c.ClusterRatelimitMaxGroupShards,

		EnableLua:  c.EnableLua,
		LuaModules: c.LuaModules.values,
		LuaSources: c.LuaSources.values,

		EnableOpenPolicyAgent:                              c.EnableOpenPolicyAgent,
		EnableOpenPolicyAgentCustomControlLoop:             c.EnableOpenPolicyAgentCustomControlLoop,
		OpenPolicyAgentControlLoopInterval:                 c.OpenPolicyAgentControlLoopInterval,
		OpenPolicyAgentControlLoopMaxJitter:                c.OpenPolicyAgentControlLoopMaxJitter,
		EnableOpenPolicyAgentDataPreProcessingOptimization: c.EnableOpenPolicyAgentDataPreProcessingOptimization,
		EnableOpenPolicyAgentPreloading:                    c.EnableOpenPolicyAgentPreloading,
		OpenPolicyAgentConfigTemplate:                      c.OpenPolicyAgentConfigTemplate,
		OpenPolicyAgentEnvoyMetadata:                       c.OpenPolicyAgentEnvoyMetadata,
		OpenPolicyAgentCleanerInterval:                     c.OpenPolicyAgentCleanerInterval,
		OpenPolicyAgentStartupTimeout:                      c.OpenPolicyAgentStartupTimeout,
		OpenPolicyAgentMaxRequestBodySize:                  c.OpenPolicyAgentMaxRequestBodySize,
		OpenPolicyAgentRequestBodyBufferSize:               c.OpenPolicyAgentRequestBodyBufferSize,
		OpenPolicyAgentMaxMemoryBodyParsing:                c.OpenPolicyAgentMaxMemoryBodyParsing,

		PassiveHealthCheck: c.PassiveHealthCheck.values,
	}
	for _, rcci := range c.CloneRoute {
		eskipClone := eskip.NewClone(rcci.Reg, rcci.Repl)
		options.CloneRoute = append(options.CloneRoute, eskipClone)
	}
	for _, rcci := range c.EditRoute {
		eskipEdit := eskip.NewEditor(rcci.Reg, rcci.Repl)
		options.EditRoute = append(options.EditRoute, eskipEdit)
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

	if len(c.Certificates) > 0 {
		options.ClientTLS = &tls.Config{
			Certificates: c.Certificates,
			MinVersion:   c.getMinTLSVersion(),
		}
	}

	var wrappers []func(handler http.Handler) http.Handler
	options.CustomHttpHandlerWrap = func(handler http.Handler) http.Handler {
		for _, wrapper := range wrappers {
			handler = wrapper(handler)
		}
		return handler
	}

	if c.ForwardedHeaders != (net.ForwardedHeaders{}) {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.ForwardedHeadersHandler{
				Headers: c.ForwardedHeaders,
				Exclude: c.ForwardedHeadersExcludeCIDRs,
				Handler: handler,
			}
		})
	}

	if c.HostPatch != (net.HostPatch{}) {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.HostPatchHandler{
				Patch:   c.HostPatch,
				Handler: handler,
			}
		})
	}

	if len(c.RefusePayload) > 0 {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.RequestMatchHandler{
				Match:   c.RefusePayload,
				Handler: handler,
			}
		})
	}

	if c.ValidateQuery {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.ValidateQueryHandler{
				Handler: handler,
			}
		})
	}
	if c.ValidateQueryLog {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.ValidateQueryLogHandler{
				Handler: handler,
			}
		})
	}

	if c.MaxContentLength != 0 {
		wrappers = append(wrappers, func(handler http.Handler) http.Handler {
			return &net.ContentLengthHeadersHandler{
				Max:     c.MaxContentLength,
				Handler: handler,
			}
		})
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
	log.Infof("No valid minimal TLS version configured (set to '%s'), fall back to default: %s", c.TLSMinVersion, defaultMinTLSVersion)
	return tlsVersionTable[defaultMinTLSVersion]
}

func (c *Config) setTLSClientAuth(s string) error {
	var ok bool
	c.TLSClientAuth, ok = map[string]tls.ClientAuthType{
		"NoClientCert":               tls.NoClientCert,
		"RequestClientCert":          tls.RequestClientCert,
		"RequireAnyClientCert":       tls.RequireAnyClientCert,
		"VerifyClientCertIfGiven":    tls.VerifyClientCertIfGiven,
		"RequireAndVerifyClientCert": tls.RequireAndVerifyClientCert,
	}[s]
	if !ok {
		return fmt.Errorf("unsupported TLS client authentication type")
	}
	return nil
}

func (c *Config) filterCipherSuites() []uint16 {
	if !c.ExcludeInsecureCipherSuites {
		return nil
	}

	cipherSuites := make([]uint16, 0)
	for _, suite := range tls.CipherSuites() {
		cipherSuites = append(cipherSuites, suite.ID)
	}

	return cipherSuites
}

func (c *Config) parseHistogramBuckets(bucketString string, defaultBuckets []float64) ([]float64, error) {
	if bucketString == "" {
		return defaultBuckets, nil
	}

	var result []float64
	thresholds := strings.Split(bucketString, ",")
	for _, v := range thresholds {
		bucket, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("unable to parse histogram-metric-buckets: %w", err)
		}
		result = append(result, bucket)
	}
	sort.Float64s(result)
	return result, nil
}

func (c *Config) parseForwardedHeaders() error {
	for _, header := range c.ForwardedHeadersList.values {
		switch {
		case header == "X-Forwarded-For":
			c.ForwardedHeaders.For = true
		case header == "X-Forwarded-Host":
			c.ForwardedHeaders.Host = true
		case header == "X-Forwarded-Method":
			c.ForwardedHeaders.Method = true
		case header == "X-Forwarded-Uri":
			c.ForwardedHeaders.Uri = true
		case strings.HasPrefix(header, "X-Forwarded-Port="):
			c.ForwardedHeaders.Port = strings.TrimPrefix(header, "X-Forwarded-Port=")
		case header == "X-Forwarded-Proto=http":
			c.ForwardedHeaders.Proto = "http"
		case header == "X-Forwarded-Proto=https":
			c.ForwardedHeaders.Proto = "https"
		default:
			return fmt.Errorf("invalid forwarded header: %s", header)
		}
	}

	cidrs, err := net.ParseCIDRs(c.ForwardedHeadersExcludeCIDRList.values)
	if err != nil {
		return fmt.Errorf("invalid forwarded headers exclude CIDRs: %w", err)
	}
	c.ForwardedHeadersExcludeCIDRs = cidrs

	return nil
}

func (c *Config) parseEnv() {
	// Set Redis password from environment variable if not set earlier (configuration file)
	if c.SwarmRedisPassword == "" {
		c.SwarmRedisPassword = os.Getenv(redisPasswordEnv)
	}
	// Set Valkey password from environment variable if not set earlier (configuration file)
	if c.SwarmValkeyPassword == "" {
		c.SwarmValkeyPassword = os.Getenv(valkeyPasswordEnv)
	}
}

func (c *Config) checkDeprecated(configKeys map[string]interface{}, options ...string) {
	flagKeys := make(map[string]bool)
	c.Flags.Visit(func(f *flag.Flag) { flagKeys[f.Name] = true })

	for _, name := range options {
		_, ck := configKeys[name]
		_, fk := flagKeys[name]
		if ck || fk {
			f := c.Flags.Lookup(name)
			log.Warnf("%s: %s", f.Name, f.Usage)
		}
	}
}

func parseAnnotationPredicates(s []string) ([]kubernetes.AnnotationPredicates, error) {
	return parseAnnotationConfig(s, func(annotationKey, annotationValue, value string) (kubernetes.AnnotationPredicates, error) {
		predicates, err := eskip.ParsePredicates(value)
		if err != nil {
			var zero kubernetes.AnnotationPredicates
			return zero, err
		}
		return kubernetes.AnnotationPredicates{
			Key:        annotationKey,
			Value:      annotationValue,
			Predicates: predicates,
		}, nil
	})
}

func parseAnnotationFilters(s []string) ([]kubernetes.AnnotationFilters, error) {
	return parseAnnotationConfig(s, func(annotationKey, annotationValue, value string) (kubernetes.AnnotationFilters, error) {
		filters, err := eskip.ParseFilters(value)
		if err != nil {
			var zero kubernetes.AnnotationFilters
			return zero, err
		}
		return kubernetes.AnnotationFilters{
			Key:     annotationKey,
			Value:   annotationValue,
			Filters: filters,
		}, nil
	})
}

// parseAnnotationConfig parses a slice of strings in the "annotationKey=annotationValue=value" format
// by calling parseValue function to convert (annotationKey, annotationValue, value) tuple into T.
// Empty input strings are skipped and duplicate annotationKey-annotationValue pairs are rejected with error.
func parseAnnotationConfig[T any](kvvs []string, parseValue func(annotationKey, annotationValue, value string) (T, error)) ([]T, error) {
	var result []T
	seenKVs := make(map[string]struct{})
	for _, kvv := range kvvs {
		if kvv == "" {
			continue
		}

		annotationKey, rest, found := strings.Cut(kvv, "=")
		if !found {
			return nil, fmt.Errorf("invalid annotation flag: %q, failed to get annotation key", kvv)
		}

		annotationValue, value, found := strings.Cut(rest, "=")
		if !found {
			return nil, fmt.Errorf("invalid annotation flag: %q, failed to get annotation value", kvv)
		}

		v, err := parseValue(annotationKey, annotationValue, value)
		if err != nil {
			return nil, fmt.Errorf("invalid annotation flag value: %q, %w", kvv, err)
		}

		// Reject duplicate annotation key-value pairs
		kv := annotationKey + "=" + annotationValue
		if _, ok := seenKVs[kv]; ok {
			return nil, fmt.Errorf("invalid annotation flag: %q, duplicate annotation key-value %q", kvv, kv)
		} else {
			seenKVs[kv] = struct{}{}
		}

		result = append(result, v)
	}
	return result, nil
}
