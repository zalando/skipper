package swarm

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

type nodeInfoClient interface {
	// GetNodeInfo returns a list of peers to join from an
	// external service discovery source.
	GetNodeInfo() ([]*NodeInfo, error)
	// Self returns NodeInfo about itself
	Self() *NodeInfo
}

func NewNodeInfoClient(o Options) (nodeInfoClient, func()) {
	log.Infof("swarm type: %s", o.swarm)
	switch o.swarm {
	case swarmKubernetes:
		cli := NewNodeInfoClientKubernetes(o)
		return cli, cli.client.Stop
	case swarmStatic:
		return o.StaticSwarm, func() {
			log.Infof("%s left swarm", o.StaticSwarm.Self())
		}
	case swarmFake:
		return NewNodeInfoClientFake(o), func() {
			// reset fakePeers to cleanup swarm nodes for tests
			fakePeers = make([]*NodeInfo, 0)
		}
	default:
		log.Errorf("unknown swarm type: %s", o.swarm)
		return nil, func() {}
	}
}

var fakePeers []*NodeInfo = make([]*NodeInfo, 0)

type nodeInfoClientFake struct {
	self  *NodeInfo
	peers map[string]*NodeInfo
}

func NewNodeInfoClientFake(o Options) *nodeInfoClientFake {
	ni := NewFakeNodeInfo(o.FakeSwarmLocalNode, []byte{127, 0, 0, 1}, o.SwarmPort)
	nic := &nodeInfoClientFake{
		self: ni,
		peers: map[string]*NodeInfo{
			o.FakeSwarmLocalNode: ni,
		},
	}
	for _, peer := range fakePeers {
		nic.peers[peer.Name] = peer
	}
	fakePeers = append(fakePeers, ni)
	return nic
}

func (nic *nodeInfoClientFake) GetNodeInfo() ([]*NodeInfo, error) {
	allKnown := []*NodeInfo{}
	for _, v := range nic.peers {
		allKnown = append(allKnown, v)
	}
	return allKnown, nil
}

func (nic *nodeInfoClientFake) Self() *NodeInfo {
	return nic.self
}

type nodeInfoClientKubernetes struct {
	kubernetesInCluster bool
	kubeAPIBaseURL      string
	client              *ClientKubernetes
	namespace           string
	labelKey            string
	labelVal            string
	port                uint16
}

func NewNodeInfoClientKubernetes(o Options) *nodeInfoClientKubernetes {
	log.Debug("SWARM: NewnodeInfoClient")
	cli, err := NewClientKubernetes(o.KubernetesOptions.KubernetesInCluster, o.KubernetesOptions.KubernetesAPIBaseURL)
	if err != nil {
		log.Fatalf("SWARM: failed to create kubernetes client: %v", err)
	}

	return &nodeInfoClientKubernetes{
		client:              cli,
		kubernetesInCluster: o.KubernetesOptions.KubernetesInCluster,
		kubeAPIBaseURL:      o.KubernetesOptions.KubernetesAPIBaseURL,
		namespace:           o.KubernetesOptions.Namespace,
		labelKey:            o.KubernetesOptions.LabelSelectorKey,
		labelVal:            o.KubernetesOptions.LabelSelectorValue,
		port:                o.SwarmPort,
	}
}

func (c *nodeInfoClientKubernetes) Self() *NodeInfo {
	nodes, err := c.GetNodeInfo()
	if err != nil {
		log.Errorf("Failed to get self: %v", err)
		return nil
	}
	return getSelf(nodes)
}

// GetNodeInfo returns a list of peers to join from Kubernetes API
// server.
func (c *nodeInfoClientKubernetes) GetNodeInfo() ([]*NodeInfo, error) {
	s, err := c.nodeInfoURL()
	if err != nil {
		log.Debugf("SWARM: failed to build request url for %s %s=%s: %s", c.namespace, c.labelKey, c.labelVal, err)
		return nil, err
	}

	rsp, err := c.client.Get(s)
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
			log.Errorf("SWARM: failed to parse the ip %s", i.Status.PodIP)
			continue
		}
		nodes = append(nodes, &NodeInfo{Name: i.Metadata.Name, Addr: addr, Port: c.port})
	}
	log.Debugf("SWARM: got nodeinfo %d", len(nodes))
	return nodes, nil
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

func (c *nodeInfoClientKubernetes) nodeInfoURL() (string, error) {
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
