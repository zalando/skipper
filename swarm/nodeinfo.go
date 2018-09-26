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
	Port uint16
}

// NewFakeNodeInfo used to create a FakeSwarm
func NewFakeNodeInfo(name string, addr net.IP, port uint16) *NodeInfo {
	return &NodeInfo{
		Name: name,
		Addr: addr,
		Port: port,
	}
}

// String will only show initial peers when created this peer
func (ni NodeInfo) String() string {
	return fmt.Sprintf("NodeInfo{%s, %s, %d}", ni.Name, ni.Addr, ni.Port)
}

// initial peers when created this peer, only nic is up to date
type knownEntryPoint struct {
	self  *NodeInfo
	nodes []*NodeInfo
	nic   nodeInfoClient
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

	self := nic.Self()
	return &knownEntryPoint{self: self, nodes: nodes, nic: nic}
}

// Node return its self
func (e *knownEntryPoint) Node() *NodeInfo {
	return e.nic.Self()
}

// Nodes return the list of known peers including self
func (e *knownEntryPoint) Nodes() []*NodeInfo {
	nodes, err := e.nic.GetNodeInfo()
	if err != nil {
		log.Errorf("Failed to get nodeinfo: %v", err)
		return nil
	}

	return nodes
}
