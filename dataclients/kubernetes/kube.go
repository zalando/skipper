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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/traffic"
)

// FEATURE:
// - provide option to limit the used namespaces?

const (
	defaultKubernetesURL          = "http://localhost:8001"
	ingressesClusterURI           = "/apis/extensions/v1beta1/ingresses"
	ingressesNamespaceFmt         = "/apis/extensions/v1beta1/namespaces/%s/ingresses"
	ingressClassKey               = "kubernetes.io/ingress.class"
	defaultIngressClass           = "skipper"
	endpointURIFmt                = "/api/v1/namespaces/%s/endpoints/%s"
	serviceURIFmt                 = "/api/v1/namespaces/%s/services/%s"
	serviceAccountDir             = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey        = "token"
	serviceAccountRootCAKey       = "ca.crt"
	serviceHostEnvVar             = "KUBERNETES_SERVICE_HOST"
	servicePortEnvVar             = "KUBERNETES_SERVICE_PORT"
	healthcheckRouteID            = "kube__healthz"
	httpRedirectRouteID           = "kube__redirect"
	healthcheckPath               = "/kube-system/healthz"
	backendWeightsAnnotationKey   = "zalando.org/backend-weights"
	ratelimitAnnotationKey        = "zalando.org/ratelimit"
	skipperfilterAnnotationKey    = "zalando.org/skipper-filter"
	skipperpredicateAnnotationKey = "zalando.org/skipper-predicate"
	skipperRoutesAnnotationKey    = "zalando.org/skipper-routes"
	pathModeAnnotationKey         = "zalando.org/skipper-ingress-path-mode"
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
}

// Client is a Skipper DataClient implementation used to create routes based on Kubernetes Ingress settings.
type Client struct {
	httpClient             *http.Client
	apiURL                 string
	provideHealthcheck     bool
	healthy                bool
	provideHTTPSRedirect   bool
	token                  string
	current                map[string]*eskip.Route
	termReceived           bool
	sigs                   chan os.Signal
	ingressClass           *regexp.Regexp
	reverseSourcePredicate bool
	pathMode               PathMode
	quit                   chan struct{}
	namespace              string
}

var nonWord = regexp.MustCompile("\\W")

var (
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

	return &Client{
		httpClient:             httpClient,
		apiURL:                 apiURL,
		provideHealthcheck:     o.ProvideHealthcheck,
		provideHTTPSRedirect:   o.ProvideHTTPSRedirect,
		current:                make(map[string]*eskip.Route),
		token:                  token,
		sigs:                   sigs,
		ingressClass:           ingClsRx,
		reverseSourcePredicate: o.ReverseSourcePredicate,
		pathMode:               o.PathMode,
		quit:                   quit,
		namespace:              o.KubernetesNamespace,
	}, nil
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
	if _, err := io.Copy(b, rsp.Body); err != nil {
		log.Debugf("reading response body failed: %v", err)
		return err
	}

	err = json.Unmarshal(b.Bytes(), a)
	if err != nil {
		log.Debugf("invalid response format: %v", err)
	}

	return err
}

// TODO:
// - check if it can be batched
// - check the existing controllers for cases when hunting for cluster ip
func (c *Client) getService(namespace, name string) (*service, error) {
	log.Debugf("requesting service: %s/%s", namespace, name)
	url := fmt.Sprintf(serviceURIFmt, namespace, name)
	var s service
	if err := c.getJSON(url, &s); err != nil {
		return nil, err
	}

	if s.Spec == nil {
		log.Debug("invalid service datagram, missing spec")
		return nil, errServiceNotFound
	}
	return &s, nil
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
	return fmt.Sprintf("kube_%s__%s__%s__%s__%s", namespace, name, host, path, backend)
}

// routeIDForCustom generates a route id for a custom route of an ingress
// resource.
func routeIDForCustom(namespace, name, id, host string, index int) string {
	name = name + "_" + id + "_" + strconv.Itoa(index)
	return routeID(namespace, name, host, "", "")
}

// converts the default backend if any
func (c *Client) convertDefaultBackend(i *ingressItem) ([]*eskip.Route, bool, error) {
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
		routes  []*eskip.Route
		ns      = i.Metadata.Namespace
		name    = i.Metadata.Name
		svcName = i.Spec.DefaultBackend.ServiceName
		svcPort = i.Spec.DefaultBackend.ServicePort
	)

	svc, err := c.getService(ns, svcName)
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
		log.Infof("Found target port %v, for service %s", targetPort, svcName)
		eps, err = c.getEndpoints(
			ns,
			svcName,
			svcPort.String(),
			targetPort,
		)
		log.Infof("convertDefaultBackend: Found %d endpoints for %s: %v", len(eps), svcName, err)
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

		r := &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: address,
		}
		routes = append(routes, r)
	} else if err != nil {
		return nil, false, err
	}

	group := routeID(ns, name, "", "", "")

	// TODO:
	// - don't do load balancing if there's only a single endpoint
	// - better: cleanup single route load balancer groups in the routing package before applying the next
	// routing table

	if len(eps) == 0 {
		return routes, true, nil
	}

	if len(eps) == 1 {
		r := &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: eps[0],
		}
		routes = append(routes, r)
		return routes, true, nil
	}

	for idx, ep := range eps {
		r := &eskip.Route{
			Id:      routeID(ns, name, "", "", strconv.Itoa(idx)),
			Backend: ep,
			Predicates: []*eskip.Predicate{{
				Name: loadbalancer.MemberPredicateName,
				Args: []interface{}{
					group,
					idx, // index within the group
				},
			}},
			Filters: []*eskip.Filter{{
				Name: builtin.DropRequestHeaderName,
				Args: []interface{}{
					loadbalancer.DecisionHeader,
				},
			}},
		}
		routes = append(routes, r)
	}

	decisionRoute := &eskip.Route{
		Id:          routeID(ns, name, "", "", "") + "__lb_group",
		BackendType: eskip.LoopBackend,
		Predicates: []*eskip.Predicate{{
			Name: loadbalancer.GroupPredicateName,
			Args: []interface{}{
				group,
			},
		}},
		Filters: []*eskip.Filter{{
			Name: loadbalancer.DecideFilterName,
			Args: []interface{}{
				group,
				len(eps), // number of member routes
			},
		}},
	}

	routes = append(routes, decisionRoute)
	return routes, true, nil
}

func (c *Client) getEndpoints(ns, name, servicePort, targetPort string) ([]string, error) {
	log.Debugf("requesting endpoint: %s/%s", ns, name)
	url := fmt.Sprintf(endpointURIFmt, ns, name)
	var ep endpoint
	if err := c.getJSON(url, &ep); err != nil {
		return nil, err
	}

	if ep.Subsets == nil {
		return nil, errEndpointNotFound
	}

	targets := ep.Targets(servicePort, targetPort)
	if len(targets) == 0 {
		return nil, errEndpointNotFound
	}
	sort.Strings(targets)
	return targets, nil
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
	ns, name, host string,
	prule *pathRule,
	pathMode PathMode,
	endpointsURLs map[string][]string,
) ([]*eskip.Route, error) {
	if prule.Backend == nil {
		return nil, fmt.Errorf("invalid path rule, missing backend in: %s/%s/%s", ns, name, host)
	}

	var (
		eps    []string
		err    error
		routes []*eskip.Route
		svc    *service
	)

	svcPort := prule.Backend.ServicePort
	svcName := prule.Backend.ServiceName
	endpointKey := ns + svcName + svcPort.String()

	if val, ok := endpointsURLs[endpointKey]; !ok {
		svc, err = c.getService(ns, svcName)
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
			eps, err = c.getEndpoints(ns, svcName, svcPort.String(), targetPort)
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
				Id:      routeID(ns, name, host, prule.Path, svcName),
				Backend: address,
			}

			setPath(pathMode, r, prule.Path)

			if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
				r.Predicates = append([]*eskip.Predicate{{
					Name: traffic.PredicateName,
					Args: []interface{}{prule.Backend.Traffic},
				}}, r.Predicates...)
				log.Infof("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, svcName)
			}
			routes = append(routes, r)
		} else if err != nil {
			return nil, err
		} else {
			endpointsURLs[endpointKey] = eps
		}
		log.Debugf("%d new routes for %s/%s/%s", len(eps), ns, svcName, svcPort)
	} else {
		eps = val
		log.Debugf("%d routes for %s/%s/%s already known", len(eps), ns, svcName, svcPort)
	}

	if len(eps) == 1 {
		r := &eskip.Route{
			Id:      routeID(ns, name, host, prule.Path, svcName),
			Backend: eps[0],
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
		routes = append(routes, r)
		return routes, nil
	}

	if len(eps) == 0 {
		return routes, nil
	}

	group := routeID(ns, name, host, prule.Path, prule.Backend.ServiceName)
	for idx, ep := range eps {
		r := &eskip.Route{
			Id:      routeID(ns, name, host, prule.Path, svcName+fmt.Sprintf("_%d", idx)),
			Backend: ep,
			Predicates: []*eskip.Predicate{{
				Name: loadbalancer.MemberPredicateName,
				Args: []interface{}{
					group,
					idx, // index within the group
				},
			}},
			Filters: []*eskip.Filter{{
				Name: builtin.DropRequestHeaderName,
				Args: []interface{}{
					loadbalancer.DecisionHeader,
				},
			}},
		}

		setPath(pathMode, r, prule.Path)
		routes = append(routes, r)
	}

	decisionRoute := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path, svcName) + "__lb_group",
		BackendType: eskip.LoopBackend,
		Predicates: []*eskip.Predicate{{
			Name: loadbalancer.GroupPredicateName,
			Args: []interface{}{
				group,
			},
		}},
		Filters: []*eskip.Filter{{
			Name: loadbalancer.DecideFilterName,
			Args: []interface{}{
				group,
				len(eps), // number of member routes
			},
		}},
	}

	setPath(pathMode, decisionRoute, prule.Path)

	// add traffic predicate if traffic weight is between 0.0 and 1.0
	if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
		decisionRoute.Predicates = append([]*eskip.Predicate{{
			Name: traffic.PredicateName,
			Args: []interface{}{prule.Backend.Traffic},
		}}, decisionRoute.Predicates...)
		log.Debugf("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, svcName)
	}

	routes = append(routes, decisionRoute)
	return routes, nil
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
func (c *Client) ingressToRoutes(items []*ingressItem) ([]*eskip.Route, error) {
	// TODO: apply the laod balancing by using the loadbalancer.BalanceRoute() function

	routes := make([]*eskip.Route, 0, len(items))
	hostRoutes := make(map[string][]*eskip.Route)
	for _, i := range items {
		if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" ||
			i.Spec == nil {
			log.Warn("invalid ingress item: missing metadata")
			continue
		}

		logger := log.WithFields(log.Fields{
			"ingress": fmt.Sprintf("%s/%s", i.Metadata.Namespace, i.Metadata.Name),
		})

		if r, ok, err := c.convertDefaultBackend(i); ok {
			routes = append(routes, r...)
		} else if err != nil {
			logger.Errorf("error while converting default backend: %v", err)
		}

		// TODO: only apply the filters from the annotations if it
		// is not an LB decision route

		// parse filter and ratelimit annotation
		var annotationFilter string
		if ratelimitAnnotationValue, ok := i.Metadata.Annotations[ratelimitAnnotationKey]; ok {
			annotationFilter = ratelimitAnnotationValue
		}
		if val, ok := i.Metadata.Annotations[skipperfilterAnnotationKey]; ok {
			if annotationFilter != "" {
				annotationFilter = annotationFilter + " -> "
			}
			annotationFilter = annotationFilter + val
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

		// parse backend-weihgts annotation if it exists
		var backendWeights map[string]float64
		if backends, ok := i.Metadata.Annotations[backendWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(backends), &backendWeights)
			if err != nil {
				logger.Errorf("error while parsing backend-weights annotation: %v", err)
			}
		}

		// We need this to avoid asking the k8s API for the same services
		endpointsURLs := make(map[string][]string)
		for _, rule := range i.Spec.Rules {
			if rule.Http == nil {
				logger.Warn("invalid ingress item: rule missing http definitions")
				continue
			}

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := []string{"^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"}

			// add extra routes from optional annotation
			for idx, route := range extraRoutes {
				route.HostRegexps = host
				route.Id = routeIDForCustom(i.Metadata.Namespace, i.Metadata.Name, route.Id, rule.Host, idx)
				hostRoutes[rule.Host] = append(hostRoutes[rule.Host], route)
			}

			// update Traffic field for each backend
			computeBackendWeights(backendWeights, rule)

			for _, prule := range rule.Http.Paths {
				if prule.Backend.Traffic > 0 {
					pathMode := c.pathMode
					pathModeString := i.Metadata.Annotations[pathModeAnnotationKey]
					if pathModeString != "" {
						var err error
						if pathMode, err = ParsePathMode(pathModeString); err != nil {
							return nil, err
						}
					}

					endpoints, err := c.convertPathRule(
						i.Metadata.Namespace,
						i.Metadata.Name,
						rule.Host,
						prule,
						pathMode,
						endpointsURLs,
					)
					if err != nil {
						// if the service is not found the route should be removed
						if err == errServiceNotFound {
							continue
						}
						// Ingress status field does not support errors
						return nil, fmt.Errorf("error while getting service: %v", err)
					}

					for _, r := range endpoints {
						r.HostRegexps = host

						var isLBDecisionRoute bool
						for _, p := range r.Predicates {
							if p.Name == loadbalancer.GroupPredicateName {
								isLBDecisionRoute = true
								break
							}
						}

						if !isLBDecisionRoute && annotationFilter != "" {
							annotationFilters, err := eskip.ParseFilters(annotationFilter)
							if err != nil {
								logger.Errorf("Can not parse annotation filters: %v", err)
							} else {
								sav := r.Filters[:]
								r.Filters = append(annotationFilters, sav...)
							}
						}

						err := applyAnnotationPredicates(pathMode, r, annotationPredicate)
						if err != nil {
							logger.Errorf("failed to apply annotation predicates: %v", err)
						}

						hostRoutes[rule.Host] = append(hostRoutes[rule.Host], r)
					}
				}
			}
		}
	}
	for host, rs := range hostRoutes {
		routes = append(routes, rs...)

		// if routes were configured, but there is no catchall route
		// defined for the host name, create a route which returns 404
		if len(rs) > 0 && !catchAllRoutes(rs) {
			catchAll := &eskip.Route{
				Id:          routeID("", "catchall", host, "", ""),
				HostRegexps: rs[0].HostRegexps,
				BackendType: eskip.ShuntBackend,
			}
			routes = append(routes, catchAll)
		}
	}

	return routes, nil
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

func (c *Client) getIngressesURI() string {
	if c.namespace == "" {
		return ingressesClusterURI
	}
	return fmt.Sprintf(ingressesNamespaceFmt, c.namespace)
}

func (c *Client) loadAndConvert() ([]*eskip.Route, error) {
	var il ingressList
	ingressesURI := c.getIngressesURI()
	if err := c.getJSON(ingressesURI, &il); err != nil {
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

	r, err := c.ingressToRoutes(fItems)
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

func httpRedirectRoute() *eskip.Route {
	// the forwarded port and any-path (.*) is set to make sure that
	// the redirect route has a higher priority during matching than
	// the normal routes that may have max 2 predicates: path regexp
	// and host.
	return &eskip.Route{
		Id: httpRedirectRouteID,
		Headers: map[string]string{
			"X-Forwarded-Proto": "http",
		},
		HeaderRegexps: map[string][]string{
			"X-Forwarded-Port": {".*"},
		},
		PathRegexps: []string{".*"},
		Filters: []*eskip.Filter{{
			Name: "redirectTo",
			Args: []interface{}{float64(308), "https:"},
		}},
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
		r = append(r, httpRedirectRoute())
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

	log.Debugf("diff taken, inserts/updates: %d, deletes: %d", len(updatedRoutes), len(deletedIDs))

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
