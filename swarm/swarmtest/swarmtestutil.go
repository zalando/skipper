package swarmtest

import (
	"errors"
	"fmt"
	"net"
	"strconv"
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

/*
use of memberlist funcs
*/

func (node *TestNode) GetHealthScore() int {
	return node.list.GetHealthScore()
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

// Leave will broadcast a leave message but will not shutdown the background
// listeners, meaning the node will continue participating in gossip and state
// updates.
//
// This will block until the leave message is successfully broadcasted to
// a member of the cluster, if any exist or until a specified timeout
// is reached.
//
// This method is safe to call multiple times, but must not be called
// after the cluster is already shut down.
func (node *TestNode) Leave(timeout time.Duration) error {
	return node.list.Leave(timeout)
}

// NumMembers returns the number of alive nodes currently known. Between
// the time of calling this and calling Members, the number of alive nodes
// may have changed, so this shouldn't be used to determine how many
// members will be returned by Members.
func (node *TestNode) NumMembers() int {
	return node.list.NumMembers()
}

// Ping initiates a ping to the node with the specified name.
func (node *TestNode) Ping(n string, addr net.Addr) (time.Duration, error) {
	return node.list.Ping(n, addr)
}

// likely not useful
// func (node *TestNode) ProtocolVersion() uint8 {
// 	return node.list.ProtocolVersion()
// }

// SendBestEffort uses the unreliable packet-oriented interface of the transport
// to target a user message at the given node (this does not use the gossip
// mechanism). The maximum size of the message depends on the configured
// UDPBufferSize for this memberlist instance.
func (node *TestNode) SendBestEffort(to *memberlist.Node, msg []byte) error {
	return node.list.SendBestEffort(to, msg)
}

// SendReliable uses the reliable stream-oriented interface of the transport to
// target a user message at the given node (this does not use the gossip
// mechanism). Delivery is guaranteed if no error is returned, and there is no
// limit on the size of the message.
func (node *TestNode) SendReliable(to *memberlist.Node, msg []byte) error {
	return node.list.SendReliable(to, msg)
}

// SendToAddress use rawMessage sent like SendBestEffort
func (node *TestNode) SendToAddress(a memberlist.Address, msg []byte) error {
	return node.list.SendToAddress(a, msg)
}

// Shutdown will stop any background maintenance of network activity
// for this memberlist, causing it to appear "dead". A leave message
// will not be broadcasted prior, so the cluster being left will have
// to detect this node's shutdown using probing. If you wish to more
// gracefully exit the cluster, call Leave prior to shutting down.
//
// This method is safe to call multiple times.
func (node *TestNode) Shutdown() error {
	return node.list.Shutdown()
}

// UpdateNode updates local metadata
// likely not useful
// func (node *TestNode) UpdateNode(timeout time.Duration) {
// 	node.list.UpdateNode(timeout)
// }

//
// use of CustomTransport
//

func (node *TestNode) Exit() error {
	if node.state != alive {
		return fmt.Errorf("cannot exit a node from %s state", node.state)
	}
	node.transport.Exit()
	return nil
}

// func (node *TestNode) ShutDown() error {
// 	return node.transport.Shutdown()
// }

//
// our funcs
//

// ListMembers is a debugging tool
func (node *TestNode) ListMembers() error {
	if node.state != alive {
		return fmt.Errorf("cannot list members of a node with %s state", node.state)
	}

	for _, mem := range node.list.Members() {
		log.Infof(fmt.Sprintf("Node:%s Name: %s, IP:%s", node.name, mem.Name, mem.Addr))
	}
	return nil
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
	transport, err := NewCustomNetTransport(nc, memberlist.Address{
		Addr: addr + ":" + strconv.Itoa(port),
		Name: hostname,
	})
	if err != nil {
		panic("failed to create memberlist config" + err.Error())
	}
	config.Transport = transport
	config.DisableTcpPings = true
	return config
}
