package swarmtest

import (
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/hashicorp/memberlist"
)

type nodeStateType int

const (
	alive nodeStateType = iota
	stateSuspect
	exit
)

func (nst nodeStateType) String() string {
	switch nst {
	case alive:
		return "alive"
	case stateSuspect:
		return "suspect"
	case exit:
		return "exit"
	}
	return "unknown"
}

type TestNode struct {
	name          string
	addr          string
	port          int
	state         nodeStateType
	ShutDownAfter time.Duration
	transport     *CustomNetTransport
	list          *memberlist.Memberlist
}

func NewTestNode(name string, addr string, port int) (*TestNode, error) {
	node := TestNode{
		name: name,
		addr: addr,
		port: port,
	}
	config := createConfig(name, addr, port)
	ctransport, found := config.Transport.(*CustomNetTransport)
	if !found {
		return nil, errors.New("failed to launch the node")
	}
	node.transport = ctransport
	list, err := memberlist.Create(config)
	if err != nil {
		return nil, err
	}
	node.list = list
	node.state = alive
	return &node, nil
}

func (node *TestNode) Join(nodesToJoin []string) error {
	n, err := node.list.Join(nodesToJoin)
	if err != nil {
		return err
	}
	if len(nodesToJoin) != n {
		log.Infof("failed to join %d nodes from the given list", len(nodesToJoin)-n)
	}
	return nil
}

func (node *TestNode) ListMembers() error {
	if node.state != alive {
		return fmt.Errorf("cannot list members of a node with %s state", node.state)
	}

	for _, mem := range node.list.Members() {
		log.Infof("Node:%s Name: %s, IP:%s", node.name, mem.Name, mem.Addr)
	}
	return nil
}

func (node *TestNode) Exit() error {
	if node.state != alive {
		return fmt.Errorf("cannot exit a node from %s state", node.state)
	}
	node.transport.Exit()
	return nil
}

func (node *TestNode) ShutDown() error {
	return node.transport.Shutdown()
}

func (node *TestNode) Addr() string {
	return fmt.Sprintf("%s:%d", node.addr, node.port)
}

func createConfig(hostname string, addr string, port int) *memberlist.Config {
	config := memberlist.DefaultLocalConfig()
	nc := &memberlist.NetTransportConfig{
		BindAddrs: []string{addr},
		BindPort:  port,
	}
	config.BindAddr = addr
	config.BindPort = port
	config.Name = hostname
	transport, err := NewCustomNetTransport(nc)
	if err != nil {
		panic("failed to create memberlist config" + err.Error())
	}
	config.Transport = transport
	config.DisableTcpPings = true
	return config
}
