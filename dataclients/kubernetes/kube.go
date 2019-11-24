package kubernetes

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/source"
)

const (
	defaultIngressClass          = "skipper"
	serviceHostEnvVar            = "KUBERNETES_SERVICE_HOST"
	servicePortEnvVar            = "KUBERNETES_SERVICE_PORT"
	healthcheckRouteID           = "kube__healthz"
	httpRedirectRouteID          = "kube__redirect"
	healthcheckPath              = "/kube-system/healthz"
	defaultLoadBalancerAlgorithm = "roundRobin"
	defaultEastWestDomain        = "skipper.cluster.local"
)

// PathMode values are used to control the ingress path interpretation. The path mode can
// be set globally for all ingress paths, and it can be overruled by the individual ingress
// rules using the zalando.org/skipper-ingress-path-mode annotation. When path mode is not
// set, the Kubernetes ingress specification is used, accepting regular expressions with a
// mandatory leading "/", automatically prepended by the "^" control character.
//
// When PathPrefix is used, the path matching becomes deterministic when
// a request could match more than one ingress routes otherwise.
type PathMode int

const (
	// KubernetesIngressMode is the default path mode. Expects regular expressions
	// with a mandatory leading "/". The expressions are automatically prepended by
	// the "^" control character.
	KubernetesIngressMode PathMode = iota

	// PathRegexp is like KubernetesIngressMode but is not prepended by the "^"
	// control character.
	PathRegexp

	// PathPrefix is like the PathSubtree predicate. E.g. "/foo/bar" will match
	// "/foo/bar" or "/foo/bar/baz", but won't match "/foo/barooz".
	//
	// In this mode, when a Path or a PathSubtree predicate is set in an annotation,
	// the value from the annotation has precedence over the standard ingress path.
	PathPrefix
)

const (
	kubernetesIngressModeString = "kubernetes-ingress"
	pathRegexpString            = "path-regexp"
	pathPrefixString            = "path-prefix"
)

const maxFileSize = 1024 * 1024 // 1MB

var internalIPs = []interface{}{
	"10.0.0.0/8",
	"192.168.0.0/16",
	"172.16.0.0/12",
	"127.0.0.1/8",
	"fd00::/8",
	"::1/128",
}

// Options is used to initialize the Kubernetes DataClient.
type Options struct {
	// KubernetesInCluster defines if skipper is deployed and running in the kubernetes cluster
	// this would make authentication with API server happen through the service account, rather than
	// running along side kubectl proxy
	KubernetesInCluster bool

	// KubernetesURL is used as the base URL for Kubernetes API requests. Defaults to http://localhost:8001.
	// (TBD: support in-cluster operation by taking the address and certificate from the standard Kubernetes
	// environment variables.)
	KubernetesURL string

	// KubernetesNamespace is used to switch between finding ingresses in the cluster-scope or limit
	// the ingresses to only those in the specified namespace. Defaults to "" which means monitor ingresses
	// in the cluster-scope.
	KubernetesNamespace string

	// KubernetesEnableEastWest if set adds automatically routes
	// with "%s.%s.skipper.cluster.local" domain pattern
	KubernetesEnableEastWest bool

	// ProvideHealthcheck, when set, tells the data client to append a healthcheck route to the ingress
	// routes in case of successfully receiving the ingress items from the API (even if individual ingress
	// items may be invalid), or a failing healthcheck route when the API communication fails. The
	// healthcheck endpoint can be accessed from internal IPs on any hostname, with the path
	// /kube-system/healthz.
	//
	// When used in a custom configuration, the current filter registry needs to include the status()
	// filter, and the available predicates need to include the Source() predicate.
	ProvideHealthcheck bool

	// ProvideHTTPSRedirect, when set, tells the data client to append an HTTPS redirect route to the
	// ingress routes. This route will detect the X-Forwarded-Proto=http and respond with a 301 message
	// to the HTTPS equivalent of the same request (using the redirectTo(301, "https:") filter). The
	// X-Forwarded-Proto and X-Forwarded-Port is expected to be set by the load balancer.
	//
	// (See also https://github.com/zalando-incubator/kube-ingress-aws-controller as part of the
	// https://github.com/zalando-incubator/kubernetes-on-aws project.)
	ProvideHTTPSRedirect bool

	// HTTPSRedirectCode, when set defines which redirect code to use for redirecting from HTTP to HTTPS.
	// By default, 308 StatusPermanentRedirect is used.
	HTTPSRedirectCode int

	// IngressClass is a regular expression to filter only those ingresses that match. If an ingress does
	// not have a class annotation or the annotation is an empty string, skipper will load it. The default
	// value for the ingress class is 'skipper'.
	//
	// For further information see:
	//		https://github.com/nginxinc/kubernetes-ingress/tree/master/examples/multiple-ingress-controllers
	IngressClass string

	// ReverseSourcePredicate set to true will do the Source IP
	// whitelisting for the heartbeat endpoint correctly in AWS.
	// Amazon's ALB writes the client IP to the last item of the
	// string list of the X-Forwarded-For header, in this case you
	// want to set this to true.
	ReverseSourcePredicate bool

	// Noop, WIP.
	ForceFullUpdatePeriod time.Duration

	// WhitelistedHealthcheckCIDR to be appended to the default iprange
	WhitelistedHealthCheckCIDR []string

	// PathMode controls the default interpretation of ingress paths in cases when the ingress doesn't
	// specify it with an annotation.
	PathMode PathMode

	// KubernetesEastWestDomain sets the DNS domain to be used for east west traffic, defaults to "skipper.cluster.local"
	KubernetesEastWestDomain string

	// DefaultFiltersDir enables default filters mechanism and sets the location of the default filters.
	// The provided filters are then applied to all routes.
	DefaultFiltersDir string

	// If the OriginMarker should be added as a filter
	OriginMarker bool
}

// Client is a Skipper DataClient implementation used to create routes based on Kubernetes Ingress settings.
type Client struct {
	clusterClient          *clusterClient
	ingress                *ingress
	routeGroups            *routeGroups
	provideHealthcheck     bool
	healthy                bool
	provideHTTPSRedirect   bool
	termReceived           bool
	reverseSourcePredicate bool
	httpsRedirectCode      int
	current                map[string]*eskip.Route
	sigs                   chan os.Signal
	quit                   chan struct{}
	defaultFiltersDir      string
	originMarker           bool
}

// New creates and initializes a Kubernetes DataClient.
func New(o Options) (*Client, error) {
	quit := make(chan struct{})

	apiURL, err := buildAPIURL(o)
	if err != nil {
		return nil, err
	}

	ingCls := defaultIngressClass
	if o.IngressClass != "" {
		ingCls = o.IngressClass
	}

	log.Debugf(
		"running in-cluster: %t. api server url: %s. provide health check: %t. ingress.class filter: %s. namespace: %s",
		o.KubernetesInCluster, apiURL, o.ProvideHealthcheck, ingCls, o.KubernetesNamespace,
	)

	var sigs chan os.Signal
	if o.ProvideHealthcheck {
		log.Info("register sigterm handler")
		sigs = make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM)
	}

	if len(o.WhitelistedHealthCheckCIDR) > 0 {
		whitelistCIDRS := make([]interface{}, len(o.WhitelistedHealthCheckCIDR))
		for i, v := range o.WhitelistedHealthCheckCIDR {
			whitelistCIDRS[i] = v
		}
		internalIPs = append(internalIPs, whitelistCIDRS...)
		log.Debugf("new internal ips are: %s", internalIPs)
	}

	httpsRedirectCode := http.StatusPermanentRedirect
	if o.HTTPSRedirectCode != 0 {
		httpsRedirectCode = o.HTTPSRedirectCode
	}

	if o.KubernetesEnableEastWest {
		if o.KubernetesEastWestDomain == "" {
			o.KubernetesEastWestDomain = defaultEastWestDomain
		} else {
			o.KubernetesEastWestDomain = strings.Trim(o.KubernetesEastWestDomain, ".")
		}
	}

	clusterClient, err := newClusterClient(o, apiURL, ingCls, quit)
	if err != nil {
		return nil, err
	}

	ing := newIngress(o, httpsRedirectCode)
	rg := newRouteGroups(o)

	return &Client{
		clusterClient:          clusterClient,
		ingress:                ing,
		routeGroups:            rg,
		provideHealthcheck:     o.ProvideHealthcheck,
		provideHTTPSRedirect:   o.ProvideHTTPSRedirect,
		httpsRedirectCode:      httpsRedirectCode,
		current:                make(map[string]*eskip.Route),
		sigs:                   sigs,
		reverseSourcePredicate: o.ReverseSourcePredicate,
		quit:                   quit,
		defaultFiltersDir:      o.DefaultFiltersDir,
		originMarker:           o.OriginMarker,
	}, nil
}

func buildAPIURL(o Options) (string, error) {
	if !o.KubernetesInCluster {
		if o.KubernetesURL == "" {
			return defaultKubernetesURL, nil
		}
		return o.KubernetesURL, nil
	}

	host, port := os.Getenv(serviceHostEnvVar), os.Getenv(servicePortEnvVar)
	if host == "" || port == "" {
		return "", errAPIServerURLNotFound
	}

	return "https://" + net.JoinHostPort(host, port), nil
}

// String returns the string representation of the path mode, the same
// values that are used in the path mode annotation.
func (m PathMode) String() string {
	switch m {
	case PathRegexp:
		return pathRegexpString
	case PathPrefix:
		return pathPrefixString
	default:
		return kubernetesIngressModeString
	}
}

// ParsePathMode parses the string representations of the different
// path modes.
func ParsePathMode(s string) (PathMode, error) {
	switch s {
	case kubernetesIngressModeString:
		return KubernetesIngressMode, nil
	case pathRegexpString:
		return PathRegexp, nil
	case pathPrefixString:
		return PathPrefix, nil
	default:
		return 0, fmt.Errorf("invalid path mode string: %s", s)
	}
}

func mapRoutes(r []*eskip.Route) map[string]*eskip.Route {
	m := make(map[string]*eskip.Route)
	for _, ri := range r {
		m[ri.Id] = ri
	}

	return m
}

func (c *Client) loadAndConvert() (*clusterState, []*eskip.Route, error) {
	state, err := c.clusterClient.fetchClusterState()
	if err != nil {
		return nil, nil, err
	}

	defaultFilters := c.fetchDefaultFilterConfigs()

	ri, err := c.ingress.convert(state, defaultFilters)
	if err != nil {
		return nil, nil, err
	}

	rg, err := c.routeGroups.convert(state, defaultFilters)
	if err != nil {
		return nil, nil, err
	}

	return state, append(ri, rg...), nil
}

func healthcheckRoute(healthy, reverseSourcePredicate bool) *eskip.Route {
	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	var p []*eskip.Predicate
	if reverseSourcePredicate {
		p = []*eskip.Predicate{{
			Name: source.NameLast,
			Args: internalIPs,
		}}
	} else {
		p = []*eskip.Predicate{{
			Name: source.Name,
			Args: internalIPs,
		}}
	}

	return &eskip.Route{
		Id:         healthcheckRouteID,
		Predicates: p,
		Path:       healthcheckPath,
		Filters: []*eskip.Filter{{
			Name: builtin.StatusName,
			Args: []interface{}{status}},
		},
		Shunt: true,
	}
}

func (c *Client) hasReceivedTerm() bool {
	select {
	case s := <-c.sigs:
		log.Infof("shutdown, caused by %s, set health check to be unhealthy", s)
		c.termReceived = true
	default:
	}

	return c.termReceived
}

func setOriginMarker(s *clusterState, r []*eskip.Route) []*eskip.Route {
	if len(r) == 0 {
		return nil
	}

	rr := make([]*eskip.Route, len(r))
	copy(rr, r)

	// it doesn't matter which route the marker is added to
	// we also copy the route, to avoid storing it for the next diff comparison
	r0 := *rr[0]
	rr[0] = &r0

	for _, i := range s.ingresses {
		r0.Filters = append(r0.Filters, builtin.NewOriginMarker(ingressOriginName, i.Metadata.Uid, i.Metadata.Created))
	}

	return rr
}

func (c *Client) LoadAll() ([]*eskip.Route, error) {
	log.Debug("loading all")
	clusterState, r, err := c.loadAndConvert()
	if err != nil {
		log.Errorf("failed to load all: %v", err)
		return nil, err
	}

	// teardown handling: always healthy unless SIGTERM received
	if c.provideHealthcheck {
		c.healthy = !c.hasReceivedTerm()
		r = append(r, healthcheckRoute(c.healthy, c.reverseSourcePredicate))
	}

	if c.provideHTTPSRedirect {
		r = append(r, globalRedirectRoute(c.httpsRedirectCode))
	}

	c.current = mapRoutes(r)
	log.Debugf("all routes loaded and mapped")

	if c.originMarker {
		r = setOriginMarker(clusterState, r)
	}

	return r, nil
}

// LoadUpdate returns all known eskip.Route, a list of route IDs
// scheduled for delete and an error.
//
// TODO: implement a force reset after some time.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	log.Debugf("polling for updates")
	clusterState, r, err := c.loadAndConvert()
	if err != nil {
		log.Errorf("polling for updates failed: %v", err)
		return nil, nil, err
	}

	next := mapRoutes(r)
	log.Debugf("next version of routes loaded and mapped")

	var (
		updatedRoutes []*eskip.Route
		deletedIDs    []string
	)

	for id := range c.current {
		// TODO: use eskip.Eq()
		if r, ok := next[id]; ok && r.String() != c.current[id].String() {
			updatedRoutes = append(updatedRoutes, r)
		} else if !ok && id != healthcheckRouteID && id != httpRedirectRouteID {
			deletedIDs = append(deletedIDs, id)
		}
	}

	for id, r := range next {
		if _, ok := c.current[id]; !ok {
			updatedRoutes = append(updatedRoutes, r)
		}
	}

	if len(updatedRoutes) > 0 || len(deletedIDs) > 0 {
		log.Infof("diff taken, inserts/updates: %d, deletes: %d", len(updatedRoutes), len(deletedIDs))
	}

	// teardown handling: always healthy unless SIGTERM received
	if c.provideHealthcheck {
		healthy := !c.hasReceivedTerm()
		if healthy != c.healthy {
			c.healthy = healthy
			hc := healthcheckRoute(c.healthy, c.reverseSourcePredicate)
			next[healthcheckRouteID] = hc
			updatedRoutes = append(updatedRoutes, hc)
		}
	}

	if c.originMarker {
		updatedRoutes = setOriginMarker(clusterState, updatedRoutes)
	}

	c.current = next
	return updatedRoutes, deletedIDs, nil
}

func (c *Client) Close() {
	if c != nil && c.quit != nil {
		close(c.quit)
	}
}

func (c *Client) fetchDefaultFilterConfigs() defaultFilters {
	if c.defaultFiltersDir == "" {
		log.Debug("default filters are disabled")
		return nil
	}

	filters, err := readDefaultFilters(c.defaultFiltersDir)
	if err != nil {
		log.WithError(err).Error("could not fetch default filter configurations")
		return nil
	}

	log.WithField("#configs", len(filters)).Debug("default filter configurations loaded")
	return filters
}
