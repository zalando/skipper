package kubernetes

import "github.com/zalando/skipper/eskip"

type apiClient interface {
	loadRouteGroups() ([]byte, error)
}

type httpAPIClient struct {
	apiURL string
}

type CRDOptions struct {

	// a way to share the common definitions
	Kubernetes Options

	// allow mocking while WIP, remove once possible to mock the API itself
	apiClient apiClient
}

// naming??? RouteGroupClient, CRDClient, other?
type CRDClient struct {
	options CRDOptions
}

func (h *httpAPIClient) loadRouteGroups() ([]byte, error) {
	return nil, nil
}

func newHTTPAPIClient(apiURL string) *httpAPIClient {
	return &httpAPIClient{apiURL: apiURL}
}

func NewCRDSource(o CRDOptions) (*CRDClient, error) {
	if o.apiClient == nil {
		o.apiClient = newHTTPAPIClient(o.Kubernetes.KubernetesURL)
	}

	return &CRDClient{options: o}, nil
}

func (c *CRDClient) LoadAll() ([]*eskip.Route, error) {
	doc, err := c.options.apiClient.loadRouteGroups()
	if err != nil {
		return nil, err
	}

	return transformCRD(doc)
}

func (c *CRDClient) LoadUpdate() (upsert []*eskip.Route, deletedIDs []string, err error) {
	return nil, nil, nil
}
