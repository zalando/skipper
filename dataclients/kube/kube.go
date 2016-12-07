package kube

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

// TODO: support both integer and string port

const (
	defaultAPIAddress = "http://localhost:8001"
	ingressesURI      = "/apis/extensions/v1beta1/ingresses"
)

type Client struct {
	apiAddress string
	current    map[string]*eskip.Route
}

var nonWord = regexp.MustCompile("\\W")

var (
	errServiceNotFound     = errors.New("service not found")
	errServicePortNotFound = errors.New("service port not found")
)

func New(apiAddress string) *Client {
	if apiAddress == "" {
		apiAddress = defaultAPIAddress
	}

	log.Debugf("kube client initialized with api address: %s", apiAddress)
	return &Client{apiAddress: apiAddress}
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiAddress + uri
	log.Debugf("making request to: %s", url)
	rsp, err := http.Get(url)
	if err != nil {
		log.Debugf("request to %s failed: %v", url, err)
		return err
	}

	log.Debugf("request to %s succeeded", url)
	defer rsp.Body.Close()

	b := bytes.NewBuffer(nil)
	if _, err := io.Copy(b, rsp.Body); err != nil {
		return err
	}

	return json.Unmarshal(b.Bytes(), a)
}

// TODO:
// - check if it can be batched
// - check the existing controllers for cases when hunting for cluster ip
func (c *Client) getServiceAddress(namespace, name string, port backendPort) (string, error) {
	url := fmt.Sprintf("/api/v1/namespaces/%s/services/%s", namespace, name)
	var s service
	if err := c.getJSON(url, &s); err != nil {
		return "", err
	}

	if s.Spec == nil {
		return "", errServiceNotFound
	}

	if p, ok := port.number(); ok {
		return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, p), nil
	}

	pn, _ := port.name()
	for _, pi := range s.Spec.Ports {
		if pi.Name == pn {
			return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, pi.Port), nil
		}
	}

	return "", errServicePortNotFound
}

// TODO: use charcode based escaping
func routeID(namespace, name, host, path string) string {
	namespace = nonWord.ReplaceAllString(namespace, "_")
	name = nonWord.ReplaceAllString(name, "_")
	host = nonWord.ReplaceAllString(host, "_")
	path = nonWord.ReplaceAllString(path, "_")
	return fmt.Sprintf("kube_%s__%s__%s__%s", namespace, name, host, path)
}

// logs if invalid, but proceeds with the valid ones
// should report failures in Ingress status
func (c *Client) ingressToRoutes(items []*ingressItem) []*eskip.Route {
	routes := make([]*eskip.Route, 0, len(items))
	for _, i := range items {
		// the usage of the default backend depends on what we want
		// we can generate a hostname out of it based on shared rules
		// and instructions in annotations, if there are no rules defined

		// this is flaw in the ingress API design, because it is not on the hosts' level, but the spec
		// tells to match if no rule matches. This means that there is no matching rule on this ingress
		// and if there are multiple ingress items, then there is a race between them.
		// TODO: don't crash when no Spec
		if i.Spec.DefaultBackend != nil {
			// TODO:
			// - check how to set failures in ingress status
			// - don't crash when no Metadata
			if address, err := c.getServiceAddress(
				i.Metadata.Namespace,
				i.Spec.DefaultBackend.ServiceName,
				i.Spec.DefaultBackend.ServicePort,
			); err == nil {
				routes = append(routes, &eskip.Route{
					Id:      routeID(i.Metadata.Namespace, i.Metadata.Name, "", ""),
					Backend: address,
				})
			} else {
				// tolerate single rule errors
				log.Errorf("error while getting service for default backend: %v", err)
			}
		}

		for _, rule := range i.Spec.Rules {

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := "^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"

			// TODO: don't crash when no Http
			for _, prule := range rule.Http.Paths {
				// TODO: figure the ingress port
				address, err := c.getServiceAddress(i.Metadata.Namespace, prule.Backend.ServiceName, prule.Backend.ServicePort)
				if err != nil {

					// tolerate single rule errors
					// TODO:
					// - check how to set failures in ingress status
					log.Errorf("error while getting service: %v", err)
					continue
				}

				var pathExpressions []string
				if prule.Path != "" {
					pathExpressions = []string{prule.Path}
				}

				routes = append(routes, &eskip.Route{
					Id:          routeID(i.Metadata.Namespace, i.Metadata.Name, rule.Host, prule.Path),
					HostRegexps: []string{host},
					PathRegexps: pathExpressions,
					Backend:     address,
				})
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

func (c *Client) LoadAll() ([]*eskip.Route, error) {
	// TODO:
	// - how to get all namespaces
	// - provide option for the used namespace

	var il ingressList
	log.Debugf("requesting ingresses")
	if err := c.getJSON(ingressesURI, &il); err != nil {
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	r := c.ingressToRoutes(il.Items)
	log.Debugf("all routes created: %d", len(r))
	c.current = mapRoutes(r)
	log.Debugf("all routes mapped: %d", len(c.current))
	return r, nil
}

// TODO: implement a force reset after some time
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	var il ingressList
	if err := c.getJSON(ingressesURI, &il); err != nil {
		return nil, nil, err
	}

	log.Debugf("ingress definitions received: %d", len(il.Items))
	r := c.ingressToRoutes(il.Items)
	next := mapRoutes(r)

	var (
		updatedRoutes []*eskip.Route
		deletedIDs    []string
	)

	for id := range c.current {
		if r, ok := next[id]; ok && r.String() != c.current[id].String() {
			updatedRoutes = append(updatedRoutes, r)
		} else if !ok {
			deletedIDs = append(deletedIDs, id)
		}
	}

	for id, r := range next {
		if _, ok := c.current[id]; !ok {
			updatedRoutes = append(updatedRoutes, r)
		}
	}

	c.current = next
	return updatedRoutes, deletedIDs, nil
}
