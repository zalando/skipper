/*
Package kubernetes implements Kubernetes Ingress support for Skipper.

See: http://kubernetes.io/docs/user-guide/ingress/

The package provides a Skipper DataClient implementation that can be used to access the Kubernetes API for
ingress resources and generate routes based on them. The client polls for the ingress settings, and there is no
need for a separate controller. On the other hand, it doesn't provide a full Ingress solution alone, because it
doesn't do any load balancer configuration or DNS updates. For a full Ingress solution, it is possible to use
Skipper together with Kube-ingress-aws-controller, which targets AWS and takes care of the load balancer setup
for Kubernetes Ingress.

See: https://github.com/zalando-incubator/kube-ingress-aws-controller

Both Kube-ingress-aws-controller and Skipper Kubernetes are part of the larger project, Kubernetes On AWS:

https://github.com/zalando-incubator/kubernetes-on-aws/

Ingress shutdown by healthcheck

The Kubernetes ingress client catches TERM signals when the ProvideHealthcheck option is enabled, and reports
failing healthcheck after the signal was received. This means that, when the Ingress client is responsible for
the healthcheck of the cluster, and the Skipper process receives the TERM signal, it won't exit by itself
immediately, but will start reporting failures on healthcheck requests. Until it gets killed by the kubelet,
Skipper keeps serving the requests in this case.

Example - Ingress

A basic ingress specification:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with ratelimiting

The example shows 50 calls per minute are allowed to each skipper
instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: ratelimit(50, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with client based ratelimiting

The example shows 3 calls per minute per client, based on
X-Forwarded-For header or IP incase there is no X-Forwarded-For header
set, are allowed to each skipper instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: localRatelimit(3, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

The example shows 500 calls per hour per client, based on
Authorization header set, are allowed to each skipper instance for the
given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: localRatelimit(500, "1h", "auth")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with custom skipper filter configuration

The example shows the use of 2 filters from skipper for the implicitly
defined route in ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: localRatelimit(50, "10m") -> requestCookie("test-session", "abc")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with custom skipper Predicate configuration

The example shows the use of a skipper predicates for the implicitly
defined route in ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-predicate: QueryParam("query", "^example$")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with shadow traffic

This will send production traffic to app-default.example.org and
copies incoming requests to https://app.shadow.example.org, but drops
responses from shadow URL. This is helpful to test your next
generation software with production workload. See also
https://godoc.org/github.com/zalando/skipper/filters/tee for details.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: tee("https://app.shadow.example.org")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

*/
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
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/loadbalancer"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/traffic"
)

// FEATURE:
// - provide option to limit the used namespaces?

const (
	defaultKubernetesURL          = "http://localhost:8001"
	ingressesURI                  = "/apis/extensions/v1beta1/ingresses"
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

	// Noop, WIP.
	ForceFullUpdatePeriod time.Duration
}

// Client is a Skipper DataClient implementation used to create routes based on Kubernetes Ingress settings.
type Client struct {
	httpClient           *http.Client
	apiURL               string
	provideHealthcheck   bool
	provideHTTPSRedirect bool
	loadBalanced         bool
	token                string
	current              map[string]*eskip.Route
	termReceived         bool
	sigs                 chan os.Signal
	ingressClass         *regexp.Regexp
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
	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, o.KubernetesInCluster)
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

	log.Debugf("running in-cluster: %t. api server url: %s. provide health check: %t. ingress.class filter: %s", o.KubernetesInCluster, apiURL, o.ProvideHealthcheck, ingCls)

	var sigs chan os.Signal
	if o.ProvideHealthcheck {
		log.Info("register sigterm handler")
		sigs = make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM)
	}

	return &Client{
		httpClient:           httpClient,
		apiURL:               apiURL,
		provideHealthcheck:   o.ProvideHealthcheck,
		provideHTTPSRedirect: o.ProvideHTTPSRedirect,
		loadBalanced:         true, // TODO(sszuecs): parameterize
		current:              make(map[string]*eskip.Route),
		token:                token,
		sigs:                 sigs,
		ingressClass:         ingClsRx,
	}, nil
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

func buildHTTPClient(certFilePath string, inCluster bool) (*http.Client, error) {
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

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

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
func (c *Client) getServiceURL(namespace, name string, port backendPort) (string, error) {
	log.Debugf("requesting service: %s/%s", namespace, name)
	url := fmt.Sprintf(serviceURIFmt, namespace, name)
	var s service
	if err := c.getJSON(url, &s); err != nil {
		return "", err
	}

	if s.Spec == nil {
		log.Debug("invalid service datagram, missing spec")
		return "", errServiceNotFound
	}

	if p, ok := port.number(); ok {
		log.Debugf("service port as number: %d", p)
		return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, p), nil
	}

	pn, _ := port.name()
	for _, pi := range s.Spec.Ports {
		if pi.Name == pn {
			log.Debugf("service port found by name: %s -> %d", pn, pi.Port)
			return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, pi.Port), nil
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

	var routes []*eskip.Route

	eps, err := c.getEndpoints(
		i.Metadata.Namespace,
		i.Spec.DefaultBackend.ServiceName,
	)
	if err == errEndpointNotFound {
		address, err2 := c.getServiceURL(
			i.Metadata.Namespace,
			i.Spec.DefaultBackend.ServiceName,
			i.Spec.DefaultBackend.ServicePort,
		)
		if err2 != nil {
			return nil, false, err2
		}

		r := &eskip.Route{
			Id:      routeID(i.Metadata.Namespace, i.Metadata.Name, "", "", ""),
			Backend: address,
		}
		routes = append(routes, r)
	} else if err != nil {
		return nil, false, err
	}

	for idx, ep := range eps {
		r := &eskip.Route{
			Id:      routeID(i.Metadata.Namespace, i.Metadata.Name, "", "", string(idx)),
			Backend: ep,
			Predicates: []*eskip.Predicate{{
				Name: loadbalancer.PredicateName,
				Args: []interface{}{
					routeID(i.Metadata.Namespace, i.Metadata.Name, "", "", ""), // group
					idx,      // index of the group
					len(eps), // number of items in the group
				},
			}},
		}
		routes = append(routes, r)
	}
	return routes, true, nil
}

// https://kube-aws-test-1.teapot.zalan.do/api/v1/namespaces/default/endpoints/skipper-test
func (c *Client) getEndpoints(ns, name string) ([]string, error) {
	log.Debugf("requesting endpoint: %s/%s", ns, name)
	url := fmt.Sprintf(endpointURIFmt, ns, name)
	var ep endpoint
	if err := c.getJSON(url, &ep); err != nil {
		return nil, err
	}

	if ep.Subsets == nil {
		log.Debug("invalid endpoint datagram, missing subsets")
		return nil, errEndpointNotFound
	}

	return ep.Targets(), nil
}

func (c *Client) convertPathRule(ns, name, host string, prule *pathRule, endpointsURLs map[string][]string) ([]*eskip.Route, error) {
	if prule.Backend == nil {
		return nil, fmt.Errorf("invalid path rule, missing backend in: %s/%s/%s", ns, name, host)
	}

	endpointKey := ns + prule.Backend.ServiceName
	var (
		eps    []string
		err    error
		routes []*eskip.Route
	)

	if val, ok := endpointsURLs[endpointKey]; !ok {
		eps, err = c.getEndpoints(ns, prule.Backend.ServiceName)
		if err == errEndpointNotFound {
			address, err2 := c.getServiceURL(
				ns,
				prule.Backend.ServiceName,
				prule.Backend.ServicePort,
			)
			if err2 != nil {
				return nil, err2
			}
			r := &eskip.Route{
				Id:      routeID(ns, prule.Backend.ServiceName, "", "", ""),
				Backend: address,
			}
			routes = append(routes, r)
		} else if err != nil {
			return nil, err
		}
		endpointsURLs[endpointKey] = eps
		log.Debugf("%d new routes for %s/%s/%s", len(eps), ns, prule.Backend.ServiceName, prule.Backend.ServicePort)
	} else {
		eps = val
		log.Debugf("%d routes for %s/%s/%s already known", len(eps), ns, prule.Backend.ServiceName, prule.Backend.ServicePort)
	}

	var pathExpressions []string
	if prule.Path != "" {
		pathExpressions = []string{"^" + prule.Path}
	}

	for idx, ep := range eps {
		group := routeID(ns, name, host, prule.Path, prule.Backend.ServiceName)
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, prule.Backend.ServiceName+fmt.Sprintf("_%d", idx)),
			PathRegexps: pathExpressions,
			Backend:     ep,
			Predicates: []*eskip.Predicate{{
				Name: loadbalancer.PredicateName,
				Args: []interface{}{
					group,    // group
					idx,      // index of the group
					len(eps), // number of items in the group
				},
			}},
			Group: group,
			Idx:   idx,
			Size:  len(eps),
			State: eskip.Pending,
		}

		// add traffic predicate if traffic weight is between 0.0 and 1.0
		if 0.0 < prule.Backend.Traffic && prule.Backend.Traffic < 1.0 {
			r.Predicates = append([]*eskip.Predicate{{
				Name: traffic.PredicateName,
				Args: []interface{}{prule.Backend.Traffic},
			}}, r.Predicates...)
			log.Debugf("Traffic weight %.2f for backend '%s'", prule.Backend.Traffic, prule.Backend.ServiceName)
		}
		routes = append(routes, r)
	}

	return routes, nil
}

// ingressToRoutes logs if an invalid found, but proceeds with the
// valid ones.  Reporting failures in Ingress status is not possible,
// because Ingress status field is v1.LoadBalancerIngress that only
// supports IP and Hostname as string.
func (c *Client) ingressToRoutes(items []*ingressItem) ([]*eskip.Route, error) {
	routes := make([]*eskip.Route, 0, len(items))
	hostRoutes := make(map[string][]*eskip.Route)
	for _, i := range items {
		if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" ||
			i.Spec == nil {
			log.Warn("invalid ingress item: missing metadata")
			continue
		}

		if r, ok, err := c.convertDefaultBackend(i); ok {
			routes = append(routes, r...)
		} else if err != nil {
			log.Errorf("error while converting default backend: %v", err)
		}

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

		// parse backend-weihgts annotation if it exists
		var backendWeights map[string]float64
		if backends, ok := i.Metadata.Annotations[backendWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(backends), &backendWeights)
			if err != nil {
				log.Errorf("error while parsing backend-weights annotation: %v", err)
			}
		}

		// We need this to avoid asking the k8s API for the same services
		endpointsURLs := make(map[string][]string)
		for _, rule := range i.Spec.Rules {
			if rule.Http == nil {
				log.Warn("invalid ingress item: rule missing http definitions")
				continue
			}

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := []string{"^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"}

			// update Traffic field for each backend
			computeBackendWeights(backendWeights, rule)

			for _, prule := range rule.Http.Paths {
				if prule.Backend.Traffic > 0 {
					endpoints, err := c.convertPathRule(i.Metadata.Namespace, i.Metadata.Name, rule.Host, prule, endpointsURLs)
					if err != nil {
						// if the service is not found the route should be removed
						if err == errServiceNotFound {
							continue
						}
						// Ingress status field does not support errors
						return nil, fmt.Errorf("error while getting service: %v", err)
					}

					// TODO(sszuecs): somehow filter unhealthy/dead endpoints..
					for _, r := range endpoints {
						r.HostRegexps = host
						if annotationFilter != "" {
							annotationFilters, err := eskip.ParseFilters(annotationFilter)
							if err != nil {
								log.Errorf("Can not parse annotation filters: %v", err)
							} else {
								sav := r.Filters[:]
								r.Filters = append(annotationFilters, sav...)
							}
						}

						if annotationPredicate != "" {
							predicates, err := eskip.ParsePredicates(annotationPredicate)
							if err != nil {
								log.Errorf("Can not parse annotation predicate: %v", err)
							} else {
								r.Predicates = append(r.Predicates, predicates...)
							}
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
	type sumCount struct {
		sum   float64
		count int
	}

	// get backend weight sum and count of backends for all paths
	pathSumCount := make(map[string]*sumCount)
	for _, path := range rule.Http.Paths {
		sc, ok := pathSumCount[path.Path]
		if !ok {
			sc = &sumCount{}
			pathSumCount[path.Path] = sc
		}

		if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
			sc.sum += weight
		} else {
			sc.count++
		}
	}

	// calculate traffic weight for each backend
	for _, path := range rule.Http.Paths {
		if sc, ok := pathSumCount[path.Path]; ok {
			if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
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

func (c *Client) listRoutes() []*eskip.Route {
	l := make([]*eskip.Route, 0, len(c.current))
	for _, r := range c.current {
		l = append(l, r)
	}

	return l
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

func (c *Client) loadAndConvert() ([]*eskip.Route, error) {
	var il ingressList
	log.Debugf("requesting ingresses")
	if err := c.getJSON(ingressesURI, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	fItems := c.filterIngressesByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))
	r, err := c.ingressToRoutes(fItems)
	if err != nil {
		log.Debugf("converting ingresses to routes failed: %v", err)
		return nil, err
	}
	log.Debugf("all routes created: %d", len(r))
	return r, nil
}

func healthcheckRoute(healthy bool) *eskip.Route {
	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	return &eskip.Route{
		Id: healthcheckRouteID,
		Predicates: []*eskip.Predicate{{
			Name: source.Name,
			Args: internalIPs,
		}},
		Path: healthcheckPath,
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
			Args: []interface{}{float64(301), "https:"},
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
		healthy := !c.hasReceivedTerm()
		r = append(r, healthcheckRoute(healthy))
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
		hc := healthcheckRoute(healthy)
		next[healthcheckRouteID] = hc
		updatedRoutes = append(updatedRoutes, hc)
	}

	c.current = next
	return updatedRoutes, deletedIDs, nil
}
