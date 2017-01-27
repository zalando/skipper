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
*/
package kubernetes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/source"
)

// FEATURE:
// - provide option to limit the used namespaces?

const (
	defaultKubernetesURL = "http://localhost:8001"
	ingressesURI         = "/apis/extensions/v1beta1/ingresses"
	serviceURIFmt        = "/api/v1/namespaces/%s/services/%s"
	healthcheckRouteID   = "kube__healthz"
	healthcheckPath      = "/kube-system/healthz"
)

var internalIPs = []interface{}{
	"10.0.0.0/8",
	"192.168.0.0/16",
	"172.16.0.0/12",
	"127.0.0.1/32",
	"fd00::/8",
	"::1/32",
}

// Options is used to initialize the Kubernetes DataClient.
type Options struct {

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

	// Noop, WIP.
	ForceFullUpdatePeriod time.Duration
}

// Client is a Skipper DataClient implementation used to create routes based on Kubernetes Ingress settings.
type Client struct {
	apiURL             string
	provideHealthcheck bool
	current            map[string]*eskip.Route
}

var nonWord = regexp.MustCompile("\\W")

var (
	errServiceNotFound     = errors.New("service not found")
	errServicePortNotFound = errors.New("service port not found")
)

// New creates and initializes a Kubernetes DataClient.
func New(o Options) *Client {
	if o.KubernetesURL == "" {
		o.KubernetesURL = defaultKubernetesURL
	}

	log.Debugf("kube client initialized with api address: %s; with healthcheck: %t",
		o.KubernetesURL, o.ProvideHealthcheck)
	return &Client{
		apiURL:             o.KubernetesURL,
		provideHealthcheck: o.ProvideHealthcheck,
		current:            make(map[string]*eskip.Route),
	}
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiURL + uri
	log.Debugf("making request to: %s", url)
	rsp, err := http.Get(url)
	if err != nil {
		log.Debugf("request to %s failed: %v", url, err)
		return err
	}

	log.Debugf("request to %s succeeded", url)
	defer rsp.Body.Close()

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
	return "", errServicePortNotFound
}

// TODO: find a nicer way to autogenerate route IDs
func routeID(namespace, name, host, path string) string {
	namespace = nonWord.ReplaceAllString(namespace, "_")
	name = nonWord.ReplaceAllString(name, "_")
	host = nonWord.ReplaceAllString(host, "_")
	path = nonWord.ReplaceAllString(path, "_")
	return fmt.Sprintf("kube_%s__%s__%s__%s", namespace, name, host, path)
}

// converts the default backend if any
func (c *Client) convertDefaultBackend(i *ingressItem) (*eskip.Route, bool, error) {
	// the usage of the default backend depends on what we want
	// we can generate a hostname out of it based on shared rules
	// and instructions in annotations, if there are no rules defined

	// this is a flaw in the ingress API design, because it is not on the hosts' level, but the spec
	// tells to match if no rule matches. This means that there is no matching rule on this ingress
	// and if there are multiple ingress items, then there is a race between them.
	if i.Spec.DefaultBackend == nil {
		return nil, false, nil
	}

	address, err := c.getServiceURL(
		i.Metadata.Namespace,
		i.Spec.DefaultBackend.ServiceName,
		i.Spec.DefaultBackend.ServicePort,
	)

	if err != nil {
		return nil, false, err
	}

	r := &eskip.Route{
		Id:      routeID(i.Metadata.Namespace, i.Metadata.Name, "", ""),
		Backend: address,
	}

	return r, true, nil
}

func (c *Client) convertPathRule(ns, name, host string, prule *pathRule) (*eskip.Route, error) {
	if prule.Backend == nil {
		return nil, fmt.Errorf("invalid path rule, missing backend in: %s/%s/%s", ns, name, host)
	}

	address, err := c.getServiceURL(ns, prule.Backend.ServiceName, prule.Backend.ServicePort)
	if err != nil {
		return nil, err
	}

	var pathExpressions []string
	if prule.Path != "" {
		pathExpressions = []string{prule.Path}
	}

	r := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path),
		PathRegexps: pathExpressions,
		Backend:     address,
	}

	return r, nil
}

// logs if invalid, but proceeds with the valid ones
// should report failures in Ingress status
//
// TODO:
// - check how to set failures in ingress status
func (c *Client) ingressToRoutes(items []*ingressItem) []*eskip.Route {
	routes := make([]*eskip.Route, 0, len(items))
	for _, i := range items {
		if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" ||
			i.Spec == nil {
			log.Warn("invalid ingress item: missing metadata")
			continue
		}

		if r, ok, err := c.convertDefaultBackend(i); ok {
			routes = append(routes, r)
		} else if err != nil {
			log.Errorf("error while converting default backend: %v", err)
		}

		for _, rule := range i.Spec.Rules {
			if rule.Http == nil {
				log.Warn("invalid ingress item: rule missing http definitions")
				continue
			}

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := []string{"^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"}

			for _, prule := range rule.Http.Paths {
				r, err := c.convertPathRule(i.Metadata.Namespace, i.Metadata.Name, rule.Host, prule)
				if err != nil {
					// tolerate single rule errors
					//
					// TODO:
					// - check how to set failures in ingress status
					log.Errorf("error while getting service: %v", err)
					continue
				}

				r.HostRegexps = host
				routes = append(routes, r)
			}
		}
	}

	return routes
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

func (c *Client) loadAndConvert() ([]*eskip.Route, error) {
	var il ingressList
	log.Debugf("requesting ingresses")
	if err := c.getJSON(ingressesURI, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	r := c.ingressToRoutes(il.Items)
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

func (c *Client) LoadAll() ([]*eskip.Route, error) {
	log.Debug("loading all")
	r, err := c.loadAndConvert()
	if err != nil {
		log.Debugf("failed to load all: %v", err)
		return nil, err
	}

	if c.provideHealthcheck {
		r = append(r, healthcheckRoute(true))
	}

	c.current = mapRoutes(r)
	log.Debugf("all routes loaded and mapped")

	return r, nil
}

// TODO: implement a force reset after some time
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	log.Debugf("polling for updates")
	r, err := c.loadAndConvert()
	if err != nil {
		log.Debugf("polling for updates failed: %v", err)

		// moving the error handling decision to the data client,
		// preserving the previous state to the routing, except
		// for the healthcheck
		if c.provideHealthcheck {
			log.Error("error while receiveing updated ingress routes;", err)
			hc := healthcheckRoute(false)
			c.current[healthcheckRouteID] = hc
			return []*eskip.Route{hc}, nil, nil
		}

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
		} else if !ok && id != healthcheckRouteID {
			deletedIDs = append(deletedIDs, id)
		}
	}

	for id, r := range next {
		if _, ok := c.current[id]; !ok {
			updatedRoutes = append(updatedRoutes, r)
		}
	}

	log.Debugf("diff taken, inserts/updates: %d, deletes: %d", len(updatedRoutes), len(deletedIDs))

	if c.provideHealthcheck {
		hc := healthcheckRoute(true)
		next[healthcheckRouteID] = hc
		updatedRoutes = append(updatedRoutes, hc)
	}

	c.current = next
	return updatedRoutes, deletedIDs, nil
}
