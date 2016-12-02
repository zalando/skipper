package kube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

const defaultAPIURL = "http://localhost:8080"

// potential feature:
// - custom validation rules

type Client struct {
	apiURL  string
	current map[string]*eskip.Route
}

var nonWord = regexp.MustCompile("\\W")

func New(apiURL string) *Client {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	return &Client{apiURL: apiURL}
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiURL + uri
	rsp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()

	// something learned from Raffo:
	b := bytes.NewBuffer(nil)
	if _, err := io.Copy(b, rsp.Body); err != nil {
		return err
	}

	return json.Unmarshal(b.Bytes(), a)
}

// TODO:
// - check if it can be batched
// - check the existing controllers for cases when hunting for cluster ip
func (c *Client) getServiceAddress(name string, port int) (string, error) {
	url := "/api/v1/namespaces/default/services?labelSelector=app%3D" + name
	var sl serviceList
	if err := c.getJSON(url, &sl); err != nil {
		return "", err
	}

	if len(sl.Items) == 0 {
		return "", nil
	}

	return fmt.Sprintf("http://%s:%d", sl.Items[0].Spec.ClusterIP, port), nil
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

		for _, rule := range i.Spec.Rules {

			// it is a regexp, would be better to have exact host, needs to be added in skipper
			// this wrapping is temporary and escaping is not the right thing to do
			// currently handled as mandatory
			host := "^" + strings.Replace(rule.Host, ".", "[.]", -1) + "$"

			for _, prule := range rule.Http.Paths {
				// TODO: figure the ingress port
				address, err := c.getServiceAddress(prule.Backend.ServiceName, prule.Backend.ServicePort)
				if err != nil {

					// tolerate single rule errors
					// TODO:
					// - check how to set failures in ingress status
					log.Errorf("error while getting service cluster ip: %v\n", err)
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
	if err := c.getJSON("/apis/extensions/v1beta1/namespaces/default/ingresses", &il); err != nil {
		return nil, err
	}

	r := c.ingressToRoutes(il.Items)
	c.current = mapRoutes(r)
	return r, nil
}

func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	var il ingressList
	if err := c.getJSON("/apis/extensions/v1beta1/namespaces/default/ingresses", &il); err != nil {
		return nil, nil, err
	}

	r := c.ingressToRoutes(il.Items)
	next := mapRoutes(r)

	var (
		updatedRoutes []*eskip.Route
		deletedIDs    []string
	)

	for id := range c.current {
		if r, ok := next[id]; ok && r.String() != next[id].String() {
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
