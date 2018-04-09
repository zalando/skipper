package swarm

// TODO: remove me - file is used, no dead code

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type nodeInfoClient struct {
	kubernetesInCluster bool
	kubeAPIBaseURL      string
	client              *Client
	namespace           string
	labelKey            string
	labelVal            string
	port                int
}

func NewnodeInfoClient(o Options) *nodeInfoClient {
	log.Debug("SWARM: NewnodeInfoClient")
	cli, err := NewClient(o.KubernetesInCluster, o.KubernetesAPIBaseURL)
	if err != nil {
		log.Fatalf("SWARM: failed to create kubernetes client: %v", err)
	}

	return &nodeInfoClient{
		client:              cli,
		kubernetesInCluster: o.KubernetesInCluster,
		kubeAPIBaseURL:      o.KubernetesAPIBaseURL,
		namespace:           o.Namespace,
		labelKey:            o.LabelSelectorKey,
		labelVal:            o.LabelSelectorValue,
		port:                o.SwarmPort,
	}
}

type metadata struct {
	Name string `json:"name"`
}

type status struct {
	PodIP string `json:"podIP"`
}

type item struct {
	Metadata metadata `json:"metadata"`
	Status   status   `json:"status"`
}

type itemList struct {
	Items []*item `json:"items"`
}

func (c *nodeInfoClient) nodeInfoURL() (string, error) {
	u, err := url.Parse(c.kubeAPIBaseURL)
	if err != nil {
		return "", err
	}
	u.Path = "/api/v1/namespaces/" + url.PathEscape(c.namespace) + "/pods"
	a := make(url.Values)
	a.Add(c.labelKey, c.labelVal)
	ls := make(url.Values)
	ls.Add("labelSelector", a.Encode())
	u.RawQuery = ls.Encode()

	return u.String(), nil
}

// GetNodeInfo returns a list of peers to join from an external
// service discovery source. Right now, the only source is hardcoded
// to be Kubernetes.
func (c *nodeInfoClient) GetNodeInfo() ([]*NodeInfo, error) {
	u, err := c.nodeInfoURL()
	if err != nil {
		log.Debugf("SWARM: failed to build request url for %s %s=%s: %s", c.namespace, c.labelKey, c.labelVal, err)
		return nil, err
	}

	rsp, err := c.client.Get(u)
	if err != nil {
		log.Debugf("SWARM: request to %s %s=%s failed: %v", c.namespace, c.labelKey, c.labelVal, err)
		return nil, err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode > http.StatusBadRequest {
		log.Debugf("SWARM: request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return nil, fmt.Errorf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err := io.Copy(b, rsp.Body); err != nil {
		log.Debugf("SWARM: reading response body failed: %v", err)
		return nil, err
	}

	var il itemList
	err = json.Unmarshal(b.Bytes(), &il)
	if err != nil {
		return nil, err
	}
	nodes := make([]*NodeInfo, 0)
	for _, i := range il.Items {
		addr := net.ParseIP(i.Status.PodIP)
		if addr == nil {
			log.Warn(fmt.Sprintf("SWARM: failed to parse the ip %s", i.Status.PodIP))
			continue
		}
		nodes = append(nodes, &NodeInfo{Name: i.Metadata.Name, Addr: addr, Port: c.port})
	}
	log.Debugf("SWARM: got nodeinfo %d", len(nodes))
	return nodes, nil
}
