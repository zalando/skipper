package kube

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

const defaultAPIURL = "http://localhost:8080"

// potential feature:
// - custom validation rules

type Client struct {
	apiURL string
}

func New(apiURL string) *Client {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	return &Client{apiURL: apiURL}
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiURL + uri
	println("requesting", url)
	rsp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	return json.NewDecoder(rsp.Body).Decode(a)
}

// TODO:
// - check if it can be batched
// - check the existing controllers for cases when hunting for cluster ip
func (c *Client) getServiceAddress(name string) (string, error) {
	url := "/api/v1/namespaces/default/services?labelSelector=app%3D" + name
	var sl serviceList
	if err := c.getJSON(url, &sl); err != nil {
		return "", err
	}

	log.Println("found services", sl)

	if len(sl.Items) == 0 {
		return "", nil
	}

	return "http://" + sl.Items[0].Spec.ClusterIP, nil
}

// logs if invalid, but proceeds with the valid ones
func (c *Client) ingressToRoutes(items []*ingressItem) []*eskip.Route {
	println("converting ingress rules", len(items))
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
				address, err := c.getServiceAddress(prule.Backend.ServiceName)
				log.Println("got address or error", address, err)
				if err != nil {

					// tolerate single rule errors
					log.Errorf("error while getting service cluster ip: %v\n", err)
					continue
				}

				var pathExpressions []string
				if prule.Path != "" {
					pathExpressions = []string{prule.Path}
				}

				routes = append(routes, &eskip.Route{
					HostRegexps: []string{host},
					PathRegexps: pathExpressions,
					Backend:     address,
				})
			}
		}
	}

	return routes
}

func (c *Client) LoadAll() ([]*eskip.Route, error) {
	// TODO:
	// - how to get all namespaces
	// - provide option for the used namespace
	var il ingressList
	if err := c.getJSON("/apis/extensions/v1beta1/namespaces/default/ingresses", &il); err != nil {
		return nil, err
	}

	return c.ingressToRoutes(il.Items), nil
}

func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}
