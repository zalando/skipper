package swarm

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

// Self can return itself as NodeInfo
type Self interface {
	Node() *NodeInfo
}

// EntryPoint knows its peers of nodes which contains itself
type EntryPoint interface {
	Nodes() []*NodeInfo
}

// NodeInfo is a value object tat contains information about swarm
// cluster nodes, that is required to access member nodes.
type NodeInfo struct {
	Name string
	Addr net.IP
	Port int
}

// NewFakeNodeInfo used to create a FakeSwarm
func NewFakeNodeInfo(name string, addr net.IP, port int) *NodeInfo {
	return &NodeInfo{
		Name: name,
		Addr: addr,
		Port: port,
	}
}

func (ni NodeInfo) String() string {
	return fmt.Sprintf("NodeInfo{%s, %s, %d}", ni.Name, ni.Addr, ni.Port)
}

// knownEntryPoint is the Kubernetes based implementation of the
// interfaces Self and Entrypoint. It stores an initial list of peers
// to connect to at the start.
//
// TODO(sszuecs): is this really Kubernetes related?
type knownEntryPoint struct {
	self  *NodeInfo
	nodes []*NodeInfo
}

// newKnownEntryPoint returns a new knownEntryPoint that knows all
// initial peers and itself. If it can not get a list of peers it will
// fail fast.
func newKnownEntryPoint(o Options) *knownEntryPoint {
	nic := NewNodeInfoClient(o)
	nodes, err := nic.GetNodeInfo()
	if err != nil {
		log.Fatalf("SWARM: Failed to get nodeinfo: %v", err)
	}
	self := getSelf(nodes)
	log.Infof("SWARM: Join swarm self=%s, nodes=%v", self, nodes)
	return &knownEntryPoint{self: self, nodes: nodes}
}

// Node return its self
func (e *knownEntryPoint) Node() *NodeInfo {
	if e == nil {
		return nil
	}
	return e.self
}

// Nodes return the list of known peers including self
func (e *knownEntryPoint) Nodes() []*NodeInfo {
	if e == nil {
		return nil
	}
	return e.nodes
}
