package swarm

import (
	"fmt"
	"net"
	"strconv"

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

// NodeInfo is a value object that contains information about swarm
// cluster nodes, that is required to access member nodes.
type NodeInfo struct {
	Name string
	Addr net.IP
	Port uint16
}

func NewStaticNodeInfo(name, addr string) (*NodeInfo, error) {
	ipString, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(ipString)
	portInt, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port in addr '%s': %w", portString, err)
	}

	return &NodeInfo{
		Name: name,
		Addr: ip,
		Port: uint16(portInt),
	}, nil
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
	return fmt.Sprintf("NodeInfo{name: %s, %s:%d}", ni.Name, ni.Addr, ni.Port)
}

// initial peers when created this peer, only nic is up to date
type knownEntryPoint struct {
	self  *NodeInfo
	nodes []*NodeInfo
	nic   nodeInfoClient
}

// newKnownEntryPoint returns a new knownEntryPoint that knows all
// initial peers and itself. If it cannot get a list of peers it will
// fail fast.
func newKnownEntryPoint(o Options) (*knownEntryPoint, func()) {
	nic, cleanupF := NewNodeInfoClient(o)
	nodes, err := nic.GetNodeInfo()
	if err != nil {
		log.Fatalf("SWARM: Failed to get nodeinfo: %v", err)
	}

	self := nic.Self()
	return &knownEntryPoint{self: self, nodes: nodes, nic: nic}, cleanupF
}

// Node return its self
func (e *knownEntryPoint) Node() *NodeInfo {
	if e.nic == nil {
		return e.self
	}
	return e.nic.Self()
}

// Nodes return the list of known peers including self
func (e *knownEntryPoint) Nodes() []*NodeInfo {
	if e.nic == nil {
		return e.nodes
	}

	nodes, err := e.nic.GetNodeInfo()
	if err != nil {
		log.Errorf("Failed to get nodeinfo: %v", err)
		return nil
	}

	return nodes
}
