package kubernetes

import (
	"time"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func (c *Client) SetLoggingInterval(d time.Duration) {
	c.loggingInterval = d
}

type ClusterClient = clusterClient

func NewClusterClient(o Options, apiURL, ingCls, rgCls string, quit <-chan struct{}) (*clusterClient, error) {
	return newClusterClient(o, apiURL, ingCls, rgCls, quit)
}

func (c *clusterClient) LoadIngressesV1() ([]*definitions.IngressV1Item, error) {
	return c.loadIngressesV1()
}

func (c *clusterClient) LoadServices() (map[definitions.ResourceID]*service, error) {
	return c.loadServices()
}

func (c *clusterClient) LoadEndpoints() (map[definitions.ResourceID]*endpoint, error) {
	return c.loadEndpoints()
}

func (c *clusterClient) LoadSecrets() (map[definitions.ResourceID]*secret, error) {
	return c.loadSecrets()
}

func (c *clusterClient) LoadEndpointSlices() (map[definitions.ResourceID]*skipperEndpointSlice, error) {
	return c.loadEndpointSlices()
}
