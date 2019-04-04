package kubernetes

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/traffic"
)

const (
	defaultKubernetesURL               = "http://localhost:8001"
	ingressesClusterURI                = "/apis/extensions/v1beta1/ingresses"
	ingressesNamespaceFmt              = "/apis/extensions/v1beta1/namespaces/%s/ingresses"
	ingressClassKey                    = "kubernetes.io/ingress.class"
	defaultIngressClass                = "skipper"
	defaultLoadbalancerAlgorithm       = "roundRobin"
	ingressRouteIDPrefix               = "kube"
	defaultEastWestDomainRegexpPostfix = "[.]skipper[.]cluster[.]local"
	endpointsClusterURI                = "/api/v1/endpoints"
	endpointsNamespaceFmt              = "/api/v1/namespaces/%s/endpoints"
	servicesClusterURI                 = "/api/v1/services"
	servicesNamespaceFmt               = "/api/v1/namespaces/%s/services"
	serviceAccountDir                  = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey             = "token"
	serviceAccountRootCAKey            = "ca.crt"
	serviceHostEnvVar                  = "KUBERNETES_SERVICE_HOST"
	servicePortEnvVar                  = "KUBERNETES_SERVICE_PORT"
	healthcheckRouteID                 = "kube__healthz"
	httpRedirectRouteID                = "kube__redirect"
	healthcheckPath                    = "/kube-system/healthz"
	backendWeightsAnnotationKey        = "zalando.org/backend-weights"
	ratelimitAnnotationKey             = "zalando.org/ratelimit"
	skipperfilterAnnotationKey         = "zalando.org/skipper-filter"
	skipperpredicateAnnotationKey      = "zalando.org/skipper-predicate"
	skipperRoutesAnnotationKey         = "zalando.org/skipper-routes"
	skipperLoadbalancerAnnotationKey   = "zalando.org/skipper-loadbalancer"
	pathModeAnnotationKey              = "zalando.org/skipper-ingress-path-mode"
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
}

// Client is a Skipper DataClient implementation used to create routes based on Kubernetes Ingress settings.
type Client struct {
	httpClient                  *http.Client
	provideHealthcheck          bool
	healthy                     bool
	provideHTTPSRedirect        bool
	termReceived                bool
	reverseSourcePredicate      bool
	kubernetesEnableEastWest    bool
	httpsRedirectCode           int
	apiURL                      string
	token                       string
	eastWestDomainRegexpPostfix string
	ingressesURI                string
	servicesURI                 string
	endpointsURI                string
	current                     map[string]*eskip.Route
	sigs                        chan os.Signal
	ingressClass                *regexp.Regexp
	pathMode                    PathMode
	quit                        chan struct{}
	defaultFiltersDir           string
}

var (
	nonWord = regexp.MustCompile(`\W`)

	errServiceNotFound      = errors.New("service not found")
	errEndpointNotFound     = errors.New("endpoint not found")
	errAPIServerURLNotFound = errors.New("kubernetes API server URL could not be constructed from env vars")
	errInvalidCertificate   = errors.New("invalid CA")
)

// New creates and initializes a Kubernetes DataClient.
func New(o Options) (*Client, error) {
	quit := make(chan struct{})
	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, o.KubernetesInCluster, quit)
	if err != nil {
		return nil, err
	}

	apiURL, err := buildAPIURL(o)
	if err != nil {
		return nil, err
	}

	token, err := readServiceAccountToken(serviceAccountDir+serviceAccountTokenKey, o.KubernetesInCluster)
	if err != nil {
		return nil, err
	}

	ingCls := defaultIngressClass
	if o.IngressClass != "" {
		ingCls = o.IngressClass
	}

	ingClsRx, err := regexp.Compile(ingCls)
	if err != nil {
		return nil, err
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

	eastWestDomainRegexpPostfix := defaultEastWestDomainRegexpPostfix
	if o.KubernetesEastWestDomain != "" {
		if strings.HasPrefix(o.KubernetesEastWestDomain, ".") {
			o.KubernetesEastWestDomain = o.KubernetesEastWestDomain[1:len(o.KubernetesEastWestDomain)]
		}
		if strings.HasSuffix(o.KubernetesEastWestDomain, ".") {
			o.KubernetesEastWestDomain = o.KubernetesEastWestDomain[:len(o.KubernetesEastWestDomain)-1]
		}
		eastWestDomainRegexpPostfix = "[.]" + strings.Replace(o.KubernetesEastWestDomain, ".", "[.]", -1)
	}

	result := &Client{
		httpClient:                  httpClient,
		apiURL:                      apiURL,
		provideHealthcheck:          o.ProvideHealthcheck,
		provideHTTPSRedirect:        o.ProvideHTTPSRedirect,
		httpsRedirectCode:           httpsRedirectCode,
		current:                     make(map[string]*eskip.Route),
		token:                       token,
		sigs:                        sigs,
		ingressClass:                ingClsRx,
		reverseSourcePredicate:      o.ReverseSourcePredicate,
		pathMode:                    o.PathMode,
		quit:                        quit,
		kubernetesEnableEastWest:    o.KubernetesEnableEastWest,
		eastWestDomainRegexpPostfix: eastWestDomainRegexpPostfix,
		ingressesURI:                ingressesClusterURI,
		servicesURI:                 servicesClusterURI,
		endpointsURI:                endpointsClusterURI,
		defaultFiltersDir:           o.DefaultFiltersDir,
	}
	if o.KubernetesNamespace != "" {
		result.setNamespace(o.KubernetesNamespace)
	}
	return result, nil
}

func (c *Client) setNamespace(namespace string) {
	c.ingressesURI = fmt.Sprintf(ingressesNamespaceFmt, namespace)
	c.servicesURI = fmt.Sprintf(servicesNamespaceFmt, namespace)
	c.endpointsURI = fmt.Sprintf(endpointsNamespaceFmt, namespace)
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

func readServiceAccountToken(tokenFilePath string, inCluster bool) (string, error) {
	if !inCluster {
		return "", nil
	}

	bToken, err := ioutil.ReadFile(tokenFilePath)
	if err != nil {
		return "", err
	}

	return string(bToken), nil
}

func buildHTTPClient(certFilePath string, inCluster bool, quit chan struct{}) (*http.Client, error) {
	if !inCluster {
		return http.DefaultClient, nil
	}

	rootCA, err := ioutil.ReadFile(certFilePath)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(rootCA) {
		return nil, errInvalidCertificate
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 30 * time.Second,
		MaxIdleConns:          5,
		MaxIdleConnsPerHost:   5,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certPool,
		},
	}

	// regularly force closing idle connections
	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				transport.CloseIdleConnections()
			case <-quit:
				return
			}
		}
	}()

	return &http.Client{
		Transport: transport,
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

func (c *Client) createRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req, nil
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiURL + uri
	log.Debugf("making request to: %s", url)

	req, err := c.createRequest("GET", url, nil)
	if err != nil {
		return err
	}

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("request to %s failed: %v", url, err)
		return err
	}

	log.Debugf("request to %s succeeded", url)
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusNotFound {
		return errServiceNotFound
	}

	if rsp.StatusCode != http.StatusOK {
		log.Debugf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return fmt.Errorf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err = io.Copy(b, rsp.Body); err != nil {
		log.Debugf("reading response body failed: %v", err)
		return err
	}

	err = json.Unmarshal(b.Bytes(), a)
	if err != nil {
		log.Debugf("invalid response format: %v", err)
	}

	return err
}

func (c *Client) getServiceURL(svc *service, port backendPort) (string, error) {
	if p, ok := port.number(); ok {
		log.Debugf("service port as number: %d", p)
		return fmt.Sprintf("http://%s:%d", svc.Spec.ClusterIP, p), nil
	}

	pn, _ := port.name()
	for _, pi := range svc.Spec.Ports {
		if pi.Name == pn {
			log.Debugf("service port found by name: %s -> %d", pn, pi.Port)
			return fmt.Sprintf("http://%s:%d", svc.Spec.ClusterIP, pi.Port), nil
		}
	}

	log.Debugf("service port not found by name: %s", pn)
	return "", errServiceNotFound
}

// TODO: find a nicer way to autogenerate route IDs
func routeID(namespace, name, host, path, backend string) string {
	namespace = nonWord.ReplaceAllString(namespace, "_")
	name = nonWord.ReplaceAllString(name, "_")
	host = nonWord.ReplaceAllString(host, "_")
	path = nonWord.ReplaceAllString(path, "_")
	backend = nonWord.ReplaceAllString(backend, "_")
	return fmt.Sprintf("%s_%s__%s__%s__%s__%s", ingressRouteIDPrefix, namespace, name, host, path, backend)
}

// routeIDForCustom generates a route id for a custom route of an ingress
// resource.
func routeIDForCustom(namespace, name, id, host string, index int) string {
	name = name + "_" + id + "_" + strconv.Itoa(index)
	return routeID(namespace, name, host, "", "")
}

func patchRouteID(rid string) string {
	return "kubeew" + rid[len(ingressRouteIDPrefix):]
}

func getLoadBalancerAlgorithm(m *metadata) string {
	algorithm := defaultLoadbalancerAlgorithm
	if algorithmAnnotationValue, ok := m.Annotations[skipperLoadbalancerAnnotationKey]; ok {
		algorithm = algorithmAnnotationValue
	}

	return algorithm
}

// converts the default backend if any
func (c *Client) convertDefaultBackend(state *clusterState, i *ingressItem) (*eskip.Route, bool, error) {
	// the usage of the default backend depends on what we want
	// we can generate a hostname out of it based on shared rules
	// and instructions in annotations, if there are no rules defined

	// this is a flaw in the ingress API design, because it is not on the hosts' level, but the spec
	// tells to match if no rule matches. This means that there is no matching rule on this ingress
	// and if there are multiple ingress items, then there is a race between them.
	if i.Spec.DefaultBackend == nil {
		return nil, false, nil
	}

	var (
		eps     []string
		err     error
		ns      = i.Metadata.Namespace
		name    = i.Metadata.Name
		svcName = i.Spec.DefaultBackend.ServiceName
		svcPort = i.Spec.DefaultBackend.ServicePort
	)

	svc, err := state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertDefaultBackend: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, false, err
	}
	targetPort, err := svc.GetTargetPort(svcPort)
	if err != nil {
		err = nil
		log.Errorf("Failed to find target port %v, %s, fallback to service", svc.Spec.Ports, svcPort)
	} else {
		// TODO(aryszka): check docs that service name is always good for requesting the endpoints
		log.Debugf("Found target port %v, for service %s", targetPort, svcName)
		eps, err = state.getEndpoints(
			ns,
			svcName,
			svcPort.String(),
			targetPort,
		)
		log.Debugf("convertDefaultBackend: Found %d endpoints for %s: %v", len(eps), svcName, err)
	}
	if len(eps) == 0 || err == errEndpointNotFound {
		// TODO(sszuecs): https://github.com/zalando/skipper/issues/549
		// dispatch by service type to implement type externalname, which has no ServicePort (could be ignored from ingress).
		// We should then implement a redirect route for that.
		// Example spec:
		//
		//     spec:
		//       type: ExternalName
		//       externalName: my.database.example.com
		address, err2 := c.getServiceURL(svc, svcPort)
		if err2 != nil {
			return nil, false, err2
		}

		return &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: address,
		}, true, nil
	} else if len(eps) == 1 {
		return &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: eps[0],
		}, true, nil
	} else if err != nil {
		return nil, false, err
	}

	return &eskip.Route{
		Id:          routeID(ns, name, "", "", ""),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(i.Metadata),
	}, true, nil
}

func setPath(m PathMode, r *eskip.Route, p string) {
	if p == "" {
		return
	}

	switch m {
	case PathPrefix:
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: "PathSubtree",
			Args: []interface{}{p},
		})
	case PathRegexp:
		r.PathRegexps = []string{p}
	default:
		r.PathRegexps = []string{"^" + p}
	}
}

func (c *Client) convertPathRule(
	state *clusterState,
	metadata *metadata,
	host string,
	prule *pathRule,
	pathMode PathMode,
) (*eskip.Route, error) {

	ns := metadata.Namespace
	name := metadata.Name

	if prule.Backend == nil {
		return nil, fmt.Errorf("invalid path rule, missing backend in: %s/%s/%s", ns, name, host)
	}

	var (
		eps []string
		err error
		svc *service
	)

	var hostRegexp []string
	if host != "" {
		hostRegexp = []string{"^" + strings.Replace(host, ".", "[.]", -1) + "$"}
	}
	svcPort := prule.Backend.ServicePort
	svcName := prule.Backend.ServiceName

	svc, err = state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertPathRule: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, err
	}

	targetPort, err := svc.GetTargetPort(svcPort)
	if err != nil {
		// fallback to service, but service definition is wrong or no pods
		log.Debugf("Failed to find target port for service %s, fallback to service: %v", svcName, err)
		err = nil
	} else {
		// err handled below
		eps, err = state.getEndpoints(ns, svcName, svcPort.String(), targetPort)
		log.Debugf("convertPathRule: Found %d endpoints %s for %s", len(eps), targetPort, svcName)
	}
	if len(eps) == 0 || err == errEndpointNotFound {
		// TODO(sszuecs): https://github.com/zalando/skipper/issues/549
		// dispatch by service type to implement type externalname, which has no ServicePort (could be ignored from ingress).
		// We should then implement a redirect route for that.
		// Example spec:
		//
		//     spec:
		//       type: ExternalName
		//       externalName: my.database.example.com
		address, err2 := c.getServiceURL(svc, svcPort)
		if err2 != nil {
			return nil, err2
		}
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     address,
			HostRegexps: hostRegexp,
		}

		setPath(pathMode, r, prule.Path)

		if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
			r.Predicates = append([]*eskip.Predicate{{
				Name: traffic.PredicateName,
				Args: []interface{}{prule.Backend.Traffic},
			}}, r.Predicates...)
			log.Debugf("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, svcName)
		}
		return r, nil

	} else if err != nil {
		return nil, err
	}
	log.Debugf("%d routes for %s/%s/%s", len(eps), ns, svcName, svcPort)

	if len(eps) == 1 {
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     eps[0],
			HostRegexps: hostRegexp,
		}

		setPath(pathMode, r, prule.Path)

		// add traffic predicate if traffic weight is between 0.0 and 1.0
		if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
			r.Predicates = append([]*eskip.Predicate{{
				Name: traffic.PredicateName,
				Args: []interface{}{prule.Backend.Traffic},
			}}, r.Predicates...)
			log.Debugf("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, svcName)
		}
		return r, nil
	}

	if len(eps) == 0 {
		return nil, nil
	}

	r := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path, prule.Backend.ServiceName),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(metadata),
		HostRegexps: hostRegexp,
	}
	setPath(pathMode, r, prule.Path)

	// add traffic predicate if traffic weight is between 0.0 and 1.0
	if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
		r.Predicates = append([]*eskip.Predicate{{
			Name: traffic.PredicateName,
			Args: []interface{}{prule.Backend.Traffic},
		}}, r.Predicates...)
		log.Debugf("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, svcName)
	}

	return r, nil
}

func applyAnnotationPredicates(m PathMode, r *eskip.Route, annotation string) error {
	if annotation == "" {
		return nil
	}

	predicates, err := eskip.ParsePredicates(annotation)
	if err != nil {
		return err
	}

	// to avoid conflict, give precedence for those predicates that come
	// from annotations
	if m == PathPrefix {
		for _, p := range predicates {
			if p.Name != "Path" && p.Name != "PathSubtree" {
				continue
			}

			r.Path = ""
			for i, p := range r.Predicates {
				if p.Name != "PathSubtree" && p.Name != "Path" {
					continue
				}

				copy(r.Predicates[i:], r.Predicates[i+1:])
				r.Predicates[len(r.Predicates)-1] = nil
				r.Predicates = r.Predicates[:len(r.Predicates)-1]
				break
			}
		}
	}

	r.Predicates = append(r.Predicates, predicates...)
	return nil
}

// ingressToRoutes logs if an invalid found, but proceeds with the
// valid ones.  Reporting failures in Ingress status is not possible,
// because Ingress status field is v1.LoadBalancerIngress that only
// supports IP and Hostname as string.
func (c *Client) ingressToRoutes(state *clusterState, defaultFilters map[resourceId]string) ([]*eskip.Route, error) {
	routes := make([]*eskip.Route, 0, len(state.ingresses))
	hostRoutes := make(map[string][]*eskip.Route)
	redirect := createRedirectInfo(c.provideHTTPSRedirect, c.httpsRedirectCode)
	for _, i := range state.ingresses {
		if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" ||
			i.Spec == nil {
			log.Error("invalid ingress item: missing metadata")
			continue
		}

		logger := log.WithFields(log.Fields{
			"ingress": fmt.Sprintf("%s/%s", i.Metadata.Namespace, i.Metadata.Name),
		})

		redirect.initCurrent(i.Metadata)

		if r, ok, err := c.convertDefaultBackend(state, i); ok {
			routes = append(routes, r)
		} else if err != nil {
			logger.Errorf("error while converting default backend: %v", err)
		}

		// parse filter and ratelimit annotation
		var annotationFilter string
		if ratelimitAnnotationValue, ok := i.Metadata.Annotations[ratelimitAnnotationKey]; ok {
			annotationFilter = ratelimitAnnotationValue
		}
		if val, ok := i.Metadata.Annotations[skipperfilterAnnotationKey]; ok {
			if annotationFilter != "" {
				annotationFilter += " -> "
			}
			annotationFilter += val
		}

		// parse predicate annotation
		var annotationPredicate string
		if val, ok := i.Metadata.Annotations[skipperpredicateAnnotationKey]; ok {
			annotationPredicate = val
		}

		// parse routes annotation
		var extraRoutes []*eskip.Route
		annotationRoutes := i.Metadata.Annotations[skipperRoutesAnnotationKey]
		if annotationRoutes != "" {
			var err error
			extraRoutes, err = eskip.Parse(annotationRoutes)
			if err != nil {
				logger.Errorf("failed to parse routes from %s, skipping: %v", skipperRoutesAnnotationKey, err)
			}
		}

		// parse backend-weights annotation if it exists
		var backendWeights map[string]float64
		if backends, ok := i.Metadata.Annotations[backendWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(backends), &backendWeights)
			if err != nil {
				logger.Errorf("error while parsing backend-weights annotation: %v", err)
			}
		}

		// parse pathmode from annotation or fallback to global default
		pathMode := c.pathMode
		if pathModeString, ok := i.Metadata.Annotations[pathModeAnnotationKey]; ok {
			if p, err := ParsePathMode(pathModeString); err != nil {
				log.Errorf("Failed to get path mode for ingress %s/%s: %v", i.Metadata.Namespace, i.Metadata.Name, err)
			} else {
				log.Debugf("Set pathMode to %s", p)
				pathMode = p
			}
		}

		for _, rule := range i.Spec.Rules {
			if rule.Http == nil {
				logger.Warn("invalid ingress item: rule missing http definitions")
				continue
			}

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := []string{"^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"}

			// update Traffic field for each backend
			computeBackendWeights(backendWeights, rule)

			for _, prule := range rule.Http.Paths {
				// add extra routes from optional annotation
				for extraIndex, r := range extraRoutes {
					route := *r
					route.HostRegexps = host
					route.Id = routeIDForCustom(
						i.Metadata.Namespace,
						i.Metadata.Name,
						route.Id,
						rule.Host+strings.Replace(prule.Path, "/", "_", -1),
						extraIndex)
					setPath(pathMode, &route, prule.Path)
					if n := countPathRoutes(&route); n <= 1 {
						hostRoutes[rule.Host] = append(hostRoutes[rule.Host], &route)
						redirect.updateHost(rule.Host)
					} else {
						log.Errorf("Failed to add route having %d path routes: %v", n, r)
					}
				}

				if prule.Backend.Traffic > 0 {
					endpointsRoute, err := c.convertPathRule(
						state,
						i.Metadata,
						rule.Host,
						prule,
						pathMode,
					)
					if err != nil {
						// if the service is not found the route should be removed
						if err == errServiceNotFound {
							continue
						}
						// Ingress status field does not support errors
						return nil, fmt.Errorf("error while getting service: %v", err)
					}

					if annotationFilter != "" {
						annotationFilters, err := eskip.ParseFilters(annotationFilter)
						if err != nil {
							logger.Errorf("Can not parse annotation filters: %v", err)
						} else {
							endpointsRoute.Filters = append(annotationFilters, endpointsRoute.Filters...)
						}
					}

					// add pre-configured default filters
					if defFilter, ok := defaultFiltersOf(prule.Backend.ServiceName, i.Metadata.Namespace, defaultFilters); ok {
						defaultFilters, err := eskip.ParseFilters(defFilter)
						if err != nil {
							logger.Errorf("Can not parse default filters: %v", err)
						} else {
							endpointsRoute.Filters = append(defaultFilters, endpointsRoute.Filters...)
						}
					}

					err = applyAnnotationPredicates(pathMode, endpointsRoute, annotationPredicate)
					if err != nil {
						logger.Errorf("failed to apply annotation predicates: %v", err)
					}

					hostRoutes[rule.Host] = append(hostRoutes[rule.Host], endpointsRoute)

					if redirect.enable || redirect.override {
						hostRoutes[rule.Host] = append(
							hostRoutes[rule.Host],
							createIngressEnableHTTPSRedirect(
								endpointsRoute,
								redirect.code,
							),
						)
						redirect.setHost(rule.Host)
					}

					if redirect.disable {
						hostRoutes[rule.Host] = append(
							hostRoutes[rule.Host],
							createIngressDisableHTTPSRedirect(
								endpointsRoute,
							),
						)
						redirect.setHostDisabled(rule.Host)
					}
				}
			}
		}

		if c.kubernetesEnableEastWest {
			for _, rule := range i.Spec.Rules {
				if rs, ok := hostRoutes[rule.Host]; ok {
					rs = append(rs, createEastWestRoutes(c.eastWestDomainRegexpPostfix, i.Metadata.Name, i.Metadata.Namespace, rs)...)
					hostRoutes[rule.Host] = rs
				}
			}
		}
	}

	for host, rs := range hostRoutes {
		if len(rs) == 0 {
			continue
		}

		routes = append(routes, rs...)

		// if routes were configured, but there is no catchall route
		// defined for the host name, create a route which returns 404
		if !catchAllRoutes(rs) {
			catchAll := &eskip.Route{
				Id:          routeID("", "catchall", host, "", ""),
				HostRegexps: rs[0].HostRegexps,
				BackendType: eskip.ShuntBackend,
			}
			routes = append(routes, catchAll)

			if c.kubernetesEnableEastWest {
				if r := createEastWestRoute(c.eastWestDomainRegexpPostfix, rs[0].Name, rs[0].Namespace, catchAll); r != nil {
					routes = append(routes, r)
				}
			}

			if code, ok := redirect.setHostCode[host]; ok {
				routes = append(routes, createIngressEnableHTTPSRedirect(catchAll, code))
			}
			if redirect.disableHost[host] {
				routes = append(routes, createIngressDisableHTTPSRedirect(catchAll))
			}
		}
	}

	return routes, nil
}

func defaultFiltersOf(service string, namespace string, defaultFilters map[resourceId]string) (string, bool) {
	if filters, ok := defaultFilters[resourceId{name: service, namespace: namespace}]; ok {
		return filters, true
	}
	return "", false
}

func createEastWestRoute(eastWestDomainRegexpPostfix, name, ns string, r *eskip.Route) *eskip.Route {
	if strings.HasPrefix(r.Id, "kubeew") || ns == "" || name == "" {
		return nil
	}
	ewR := *r
	ewR.HostRegexps = []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ewR.Id = patchRouteID(r.Id)
	return &ewR
}

func createEastWestRoutes(eastWestDomainRegexpPostfix, name, ns string, routes []*eskip.Route) []*eskip.Route {
	ewroutes := make([]*eskip.Route, 0)
	newHostRegexps := []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ingressAlreadyHandled := false

	for _, r := range routes {
		// TODO(sszuecs) we have to rethink how to handle eastwest routes in more complex cases
		n := countPathRoutes(r)
		// FIX memory leak in route creation
		if strings.HasPrefix(r.Id, "kubeew") || (n == 0 && ingressAlreadyHandled) {
			continue
		}
		r.Namespace = ns // store namespace
		r.Name = name    // store name
		ewR := *r
		ewR.HostRegexps = newHostRegexps
		ewR.Id = patchRouteID(r.Id)
		ewroutes = append(ewroutes, &ewR)
		ingressAlreadyHandled = true
	}
	return ewroutes
}

func countPathRoutes(r *eskip.Route) int {
	i := 0
	for _, p := range r.Predicates {
		if p.Name == "PathSubtree" || p.Name == "Path" {
			i++
		}
	}
	if r.Path != "" {
		i++
	}
	return i
}

// catchAllRoutes returns true if one of the routes in the list has a catchAll
// path expression.
func catchAllRoutes(routes []*eskip.Route) bool {
	for _, route := range routes {
		if len(route.PathRegexps) == 0 {
			return true
		}

		for _, exp := range route.PathRegexps {
			if exp == "^/" {
				return true
			}
		}
	}

	return false
}

// computeBackendWeights computes and sets the backend traffic weights on the
// rule backends.
// The traffic is calculated based on the following rules:
//
// * if no weight is defined for a backend it will get weight 0.
// * if no weights are specified for all backends of a path, then traffic will
//   be distributed equally.
//
// Each traffic weight is relative to the number of backends per path. If there
// are multiple backends per path the weight will be relative to the number of
// remaining backends for the path e.g. if the weight is specified as
//
//      backend-1: 0.2
//      backend-2: 0.6
//      backend-3: 0.2
//
// then the weight will be calculated to:
//
//      backend-1: 0.2
//      backend-2: 0.75
//      backend-3: 1.0
//
// where for a weight of 1.0 no Traffic predicate will be generated.
func computeBackendWeights(backendWeights map[string]float64, rule *rule) {
	type pathInfo struct {
		sum        float64
		lastActive *backend
		count      int
	}

	// get backend weight sum and count of backends for all paths
	pathInfos := make(map[string]*pathInfo)
	for _, path := range rule.Http.Paths {
		sc, ok := pathInfos[path.Path]
		if !ok {
			sc = &pathInfo{}
			pathInfos[path.Path] = sc
		}

		if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
			sc.sum += weight
			if weight > 0 {
				sc.lastActive = path.Backend
			}
		} else {
			sc.count++
		}
	}

	// calculate traffic weight for each backend
	for _, path := range rule.Http.Paths {
		if sc, ok := pathInfos[path.Path]; ok {
			if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
				// force a weight of 1.0 for the last backend with a non-zero weight to avoid rounding issues
				if sc.lastActive == path.Backend {
					path.Backend.Traffic = 1.0
					continue
				}

				path.Backend.Traffic = weight / sc.sum
				// subtract weight from the sum in order to
				// give subsequent backends a higher relative
				// weight.
				sc.sum -= weight
			} else if sc.sum == 0 && sc.count > 0 {
				path.Backend.Traffic = 1.0 / float64(sc.count)
			}
			// reduce count by one in order to give subsequent
			// backends for the path a higher relative weight.
			sc.count--
		}
	}
}

func mapRoutes(r []*eskip.Route) map[string]*eskip.Route {
	m := make(map[string]*eskip.Route)
	for _, ri := range r {
		m[ri.Id] = ri
	}

	return m
}

// filterIngressesByClass will filter only the ingresses that have the valid class, these are
// the defined one, empty string class or not class at all
func (c *Client) filterIngressesByClass(items []*ingressItem) []*ingressItem {
	validIngs := []*ingressItem{}

	for _, ing := range items {
		// No metadata is the same as no annotations for us
		if ing.Metadata != nil {
			cls, ok := ing.Metadata.Annotations[ingressClassKey]
			// Skip loop iteration if not valid ingress (non defined, empty or non defined one)
			if ok && cls != "" && !c.ingressClass.MatchString(cls) {
				continue
			}
		}
		validIngs = append(validIngs, ing)
	}

	return validIngs
}

func (c *Client) loadIngresses() ([]*ingressItem, error) {
	var il ingressList
	if err := c.getJSON(c.ingressesURI, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	fItems := c.filterIngressesByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))

	sort.Slice(fItems, func(i, j int) bool {
		mI := fItems[i].Metadata
		mJ := fItems[j].Metadata
		if mI == nil && mJ != nil {
			return true
		} else if mJ == nil {
			return false
		}
		nsI := mI.Namespace
		nsJ := mJ.Namespace
		if nsI != nsJ {
			return nsI < nsJ
		}
		return mI.Name < mJ.Name
	})

	return fItems, nil
}

func (c *Client) loadServices() (map[resourceId]*service, error) {
	var services serviceList
	if err := c.getJSON(c.servicesURI, &services); err != nil {
		log.Debugf("requesting all services failed: %v", err)
		return nil, err
	}

	log.Debugf("all services received: %d", len(services.Items))
	result := make(map[resourceId]*service)
	for _, service := range services.Items {
		result[service.Meta.toResourceId()] = service
	}
	return result, nil
}

func (c *Client) loadEndpoints() (map[resourceId]*endpoint, error) {
	var endpoints endpointList
	if err := c.getJSON(c.endpointsURI, &endpoints); err != nil {
		log.Debugf("requesting all endpoints failed: %v", err)
		return nil, err
	}

	log.Debugf("all endpoints received: %d", len(endpoints.Items))
	result := make(map[resourceId]*endpoint)
	for _, endpoint := range endpoints.Items {
		result[endpoint.Meta.toResourceId()] = endpoint
	}
	return result, nil
}

func (c *Client) fetchClusterState() (*clusterState, error) {
	ingresses, err := c.loadIngresses()
	if err != nil {
		return nil, err
	}
	services, err := c.loadServices()
	if err != nil {
		return nil, err
	}
	endpoints, err := c.loadEndpoints()
	if err != nil {
		return nil, err
	}

	return &clusterState{
		ingresses:       ingresses,
		services:        services,
		endpoints:       endpoints,
		cachedEndpoints: make(map[endpointId][]string),
	}, nil
}

func (c *Client) loadAndConvert() ([]*eskip.Route, error) {
	state, err := c.fetchClusterState()
	if err != nil {
		return nil, err
	}

	defaultFilters := c.fetchDefaultFilterConfigs()
	log.Debugf("got default filter configurations for %d services", len(defaultFilters))

	r, err := c.ingressToRoutes(state, defaultFilters)
	if err != nil {
		log.Debugf("converting ingresses to routes failed: %v", err)
		return nil, err
	}
	log.Debugf("all routes created: %d", len(r))

	return r, nil
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

func (c *Client) LoadAll() ([]*eskip.Route, error) {
	log.Debug("loading all")
	r, err := c.loadAndConvert()
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

	return r, nil
}

// LoadUpdate returns all known eskip.Route, a list of route IDs
// scheduled for delete and an error.
//
// TODO: implement a force reset after some time.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	log.Debugf("polling for updates")
	r, err := c.loadAndConvert()
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

	c.current = next
	return updatedRoutes, deletedIDs, nil
}

func (c *Client) Close() {
	if c != nil && c.quit != nil {
		close(c.quit)
	}
}

func (c *Client) fetchDefaultFilterConfigs() map[resourceId]string {
	if c.defaultFiltersDir == "" {
		log.Debug("default filters are disabled")
		return make(map[resourceId]string)
	}

	filters, err := c.getDefaultFilterConfigurations()

	if err != nil {
		log.WithError(err).Error("could not fetch default filter configurations")
		return make(map[resourceId]string)
	}

	log.WithField("#configs", len(filters)).Debug("default filter configurations loaded")

	return filters
}

func (c *Client) getDefaultFilterConfigurations() (map[resourceId]string, error) {
	files, err := ioutil.ReadDir(c.defaultFiltersDir)
	if err != nil {
		return nil, err
	}

	filters := make(map[resourceId]string)
	for _, f := range files {
		r := strings.Split(f.Name(), ".") // format: {service}.{namespace}
		if len(r) != 2 || notRegularFile(f) || f.Size() > maxFileSize {
			log.WithError(err).WithField("file", f.Name()).Debug("incompatible file")
			continue
		}

		file := filepath.Join(c.defaultFiltersDir, f.Name())
		config, err := ioutil.ReadFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Debug("could not read file")
			continue
		}

		filters[resourceId{name: r[0], namespace: r[1]}] = string(config)
	}

	return filters, nil
}

func notRegularFile(f os.FileInfo) bool {
	mode := f.Mode()
	return f.IsDir() || mode == os.ModeIrregular || mode == os.ModeDevice || mode == os.ModeNamedPipe || mode == os.ModeSocket
}
