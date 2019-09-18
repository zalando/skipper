package kubernetes

import (
	"github.com/zalando/skipper/eskip"
)

type routeGroups struct{}

func newRouteGroups(o Options) *routeGroups {
	return nil
}

func (r *routeGroups) convert(s *clusterState, defaultFilters map[resourceId]string) ([]*eskip.Route, error) {
	return nil, nil
}

// --

type apiClient interface {
	loadRouteGroups() ([]byte, error)
}

type httpAPIClient struct {
	apiURL string
}

type RouteGroupsOptions struct {

	// a way to share the common definitions
	Kubernetes Options

	// allow mocking while WIP, remove once possible to mock the API itself
	apiClient apiClient
}

type RouteGroupClient struct {
	options RouteGroupsOptions
}

func (h *httpAPIClient) loadRouteGroups() ([]byte, error) {
	return nil, nil
}

func newHTTPAPIClient(apiURL string) *httpAPIClient {
	return &httpAPIClient{apiURL: apiURL}
}

func NewRouteGroupClient(o RouteGroupsOptions) (*RouteGroupClient, error) {
	if o.apiClient == nil {
		o.apiClient = newHTTPAPIClient(o.Kubernetes.KubernetesURL)
	}

	return &RouteGroupClient{options: o}, nil
}

func (c *RouteGroupClient) LoadAll() ([]*eskip.Route, error) {
	doc, err := c.options.apiClient.loadRouteGroups()
	if err != nil {
		return nil, err
	}

	return transformRouteGroups(doc)
}

func (c *RouteGroupClient) LoadUpdate() (upsert []*eskip.Route, deletedIDs []string, err error) {
	return nil, nil, nil
}
