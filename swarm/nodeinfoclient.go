package swarm

import (
	"net"
	"net/url"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
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
		return cli, cli.client.Close
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
	client    *kubernetes.Client
	namespace string
	name      string
	port      uint16
}

func NewNodeInfoClientKubernetes(o Options) *nodeInfoClientKubernetes {
	log.Debug("SWARM: NewnodeInfoClient")

	return &nodeInfoClientKubernetes{
		client:    o.KubernetesOptions.KubernetesClient,
		namespace: o.KubernetesOptions.Namespace,
		name:      o.KubernetesOptions.Name,
		port:      o.SwarmPort,
	}
}

func (c *nodeInfoClientKubernetes) Self() *NodeInfo {
	nodes, err := c.GetNodeInfo()
	if err != nil {
		log.Errorf("Failed to get node info: %v", err)
		return nil
	}
	return getSelf(nodes)
}

// GetNodeInfo returns a list of peers to join from Kubernetes API
// server.
func (c *nodeInfoClientKubernetes) GetNodeInfo() ([]*NodeInfo, error) {
	list := c.client.GetEndpointAddresses(c.namespace, c.name)

	nodes := make([]*NodeInfo, 0, len(list))
	for _, s := range list {
		u, err := url.Parse(s)
		if err != nil {
			log.Errorf("SWARM: failed to parse url '%s': %v", s, err)
			continue
		}
		addr := net.ParseIP(u.Hostname())
		port, err := parsetUint16(u.Port())
		if err != nil {
			log.Errorf("SWARM: failed to parse port to int: %v", err)
			continue
		}
		n := &NodeInfo{Name: s, Addr: addr, Port: port}
		log.Debugf("SWARM: got nodeinfo %v", n)
		nodes = append(nodes, n)
	}
	log.Debugf("SWARM: got nodeinfo with %d members", len(nodes))
	return nodes, nil
}

func parsetUint16(s string) (uint16, error) {
	u64, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(u64), err
}
