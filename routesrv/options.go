package routesrv

import (
	"regexp"
	"time"

	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
)

// Options for initializing/running RouteServer
type Options struct {
	// Network address that routesrv should listen on.
	Address string

	// Polling timeout of the routing data source
	SourcePollTimeout time.Duration

	// WaitForHealthcheckInterval sets the time that skipper waits
	// for the loadbalancer in front to become unhealthy. Defaults
	// to 0.
	WaitForHealthcheckInterval time.Duration

	// OpenTracing enables tracing
	OpenTracing []string

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
	// skipper load only the ingress resources that have a matching
	// kubernetes.io/ingress.class annotation. For backwards compatibility,
	// the ingresses without an annotation, or an empty annotation, will
	// be loaded, too.
	KubernetesIngressClass string

	// KubernetesRouteGroupClass is a regular expression, that will make skipper
	// load only the RouteGroup resources that have a matching
	// zalando.org/routegroup.class annotation. Any RouteGroups without the
	// annotation, or which an empty annotation, will be loaded too.
	KubernetesRouteGroupClass string

	// PathMode controls the default interpretation of ingress paths in cases
	// when the ingress doesn't specify it with an annotation.
	KubernetesPathMode kubernetes.PathMode

	// KubernetesNamespace is used to switch between monitoring ingresses in the cluster-scope or limit
	// the ingresses to only those in the specified namespace. Defaults to "" which means monitor ingresses
	// in the cluster-scope.
	KubernetesNamespace string

	// *DEPRECATED* KubernetesEnableEastWest enables cluster internal service to service communication, aka east-west traffic
	KubernetesEnableEastWest bool

	// *DEPRECATED* KubernetesEastWestDomain sets the cluster internal domain used to create additional routes in skipper, defaults to skipper.cluster.local
	KubernetesEastWestDomain string

	// KubernetesEastWestRangeDomains set the the cluster internal domains for
	// east west traffic. Identified routes to such domains will include
	// the KubernetesEastWestRangePredicates.
	KubernetesEastWestRangeDomains []string

	// KubernetesEastWestRangePredicates set the Predicates that will be
	// appended to routes identified as to KubernetesEastWestRangeDomains.
	KubernetesEastWestRangePredicates []*eskip.Predicate

	// KubernetesOnlyAllowedExternalNames will enable validation of ingress external names and route groups network
	// backend addresses, explicit LB endpoints validation agains the list of patterns in
	// AllowedExternalNames.
	KubernetesOnlyAllowedExternalNames bool

	// KubernetesAllowedExternalNames contains regexp patterns of those domain names that are allowed to be
	// used with external name services (type=ExternalName).
	KubernetesAllowedExternalNames []*regexp.Regexp

	// WhitelistedHealthcheckCIDR appends the whitelisted IP Range to the inernalIPS range for healthcheck purposes
	WhitelistedHealthCheckCIDR []string

	// ReverseSourcePredicate enables the automatic use of IP
	// whitelisting in different places to use the reversed way of
	// identifying a client IP within the X-Forwarded-For
	// header. Amazon's ALB for example writes the client IP to
	// the last item of the string list of the X-Forwarded-For
	// header, in this case you want to set this to true.
	ReverseSourcePredicate bool

	// Default filters directory enables default filters mechanism and sets the directory where the filters are located
	DefaultFiltersDir string

	// OriginMarker is *deprecated* and not used anymore. It will be deleted in v1.
	OriginMarker bool

	// OpenTracingBackendNameTag enables an additional tracing tag containing a backend name
	// for a route when it's available (e.g. for RouteGroups)
	OpenTracingBackendNameTag bool
}
