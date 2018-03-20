package swarm

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/memberlist"
)

type NodeState int

const (
	Initial NodeState = iota
	Connected
	Disconnected
)

const (
	DefaultMaxMessageBuffer = 1 << 22
	DefaultLeaveTimeout     = 180 * time.Millisecond
)

type NodeInfo struct {
	Name string
	Addr net.IP
	Port int
}

type Self interface {
	Node() (*NodeInfo, error)
}

// Join returns a current node that a node can use to join to a swarm.
type EntryPoint interface {
	Nodes() ([]*NodeInfo, error)
}

type KnownEPoint struct {
	self  *NodeInfo
	nodes []*NodeInfo
}

type messageType int

const (
	sharedValue messageType = iota
	broadcast
)

type message struct {
	Type   messageType
	Source string
	Key    string
	Value  interface{}
}

type outgoingMessage struct {
	message *message
	encoded []byte
}

type Message struct {
	Source string
	Value  interface{}
}

type reqOutgoing struct {
	overhead int
	limit    int
	ret      chan [][]byte
}

type mlDelegate struct {
	meta     []byte
	outgoing chan<- reqOutgoing
	incoming chan<- []byte
}

type Options struct {
	// defaults from the underlying implementation
	SelfSpec Self

	// leaky, expected to be buffered, or errors are lost
	Errors chan<- error

	MaxMessageBuffer int

	LeaveTimeout time.Duration
}

type sharedValues map[string]map[string]interface{}

type valueReq struct {
	key string
	ret chan map[string]interface{}
}

type Swarm struct {
	local            *NodeInfo
	errors           chan<- error
	maxMessageBuffer int
	leaveTimeout     time.Duration

	getOutgoing <-chan reqOutgoing
	outgoing    chan *outgoingMessage
	incoming    <-chan []byte
	listeners   map[string]chan<- *Message
	leave       chan struct{}
	getValues   chan *valueReq

	messages [][]byte
	shared   sharedValues
	mlist    *memberlist.Memberlist
}

func NewSwarm() (*Swarm, error) {
	return Start(Options{
		Errors:           make(chan<- error), // FIXME - do WE have to read this, or...?
		MaxMessageBuffer: 100,
		LeaveTimeout:     time.Duration(5 * time.Second),
	})
}

func KnownEntryPoint(self *NodeInfo, n ...*NodeInfo) *KnownEPoint {
	return &KnownEPoint{self: self, nodes: n}
}

func (e *KnownEPoint) Node() (*NodeInfo, error) {
	return e.self, nil
}

func (e *KnownEPoint) Nodes() ([]*NodeInfo, error) {
	return e.nodes, nil
}

func (d *mlDelegate) NodeMeta(limit int) []byte {
	if len(d.meta) > limit {
		// TODO: would nil better here?
		// documentation is unclear
		return d.meta[:limit]
	}

	return d.meta
}

func (d *mlDelegate) NotifyMsg(m []byte) {
	d.incoming <- m
}

// TODO: verify over TCP-only
func (d *mlDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	req := reqOutgoing{
		overhead: overhead,
		limit:    limit,
		ret:      make(chan [][]byte),
	}
	d.outgoing <- req
	return <-req.ret
}

func (d *mlDelegate) LocalState(bool) []byte                 { return nil }
func (d *mlDelegate) MergeRemoteState(buf []byte, join bool) {}

func mapNodesToAddresses(n []*NodeInfo) []string {
	var s []string
	for i := range n {
		s = append(s, fmt.Sprintf("%v:%d", n[i].Addr, n[i].Port))
	}

	return s
}

// the top level map is used internally, we can use it as mutable
// the leaf maps are shared, we need to clone those
func (sv sharedValues) set(source, key string, value interface{}) {
	prev := sv[key]
	sv[key] = make(map[string]interface{})
	for s, v := range prev {
		sv[key][s] = v
	}

	sv[key][source] = value
}

func Start(o Options) (*Swarm, error) {
	return Join(o, nil)
}

func Join(o Options, e EntryPoint) (*Swarm, error) {
	c := memberlist.DefaultLocalConfig()
	if o.SelfSpec == nil {
		o.SelfSpec = KnownEntryPoint(&NodeInfo{})
	}

	nodeSpec, err := o.SelfSpec.Node()
	if err != nil {
		return nil, err
	}

	if nodeSpec.Name == "" {
		nodeSpec.Name = c.Name
	} else {
		c.Name = nodeSpec.Name
	}

	if nodeSpec.Addr == nil {
		nodeSpec.Addr = net.ParseIP(c.BindAddr)
	} else {
		c.BindAddr = nodeSpec.Addr.String()
		c.AdvertiseAddr = c.BindAddr
	}
	println("nodespec", nodeSpec.Port)
	if nodeSpec.Port == 0 {
		nodeSpec.Port = c.BindPort
	} else {
		c.BindPort = nodeSpec.Port
		c.AdvertisePort = c.BindPort
	}

	if o.MaxMessageBuffer <= 0 {
		o.MaxMessageBuffer = DefaultMaxMessageBuffer
	}

	if o.LeaveTimeout <= 0 {
		o.LeaveTimeout = DefaultLeaveTimeout
	}

	getOutgoing := make(chan reqOutgoing)
	outgoing := make(chan *outgoingMessage)
	incoming := make(chan []byte)
	getValues := make(chan *valueReq)
	listeners := make(map[string]chan<- *Message)
	leave := make(chan struct{})
	shared := make(sharedValues)

	c.Delegate = &mlDelegate{
		outgoing: getOutgoing,
		incoming: incoming,
	}

	ml, err := memberlist.Create(c)
	if err != nil {
		return nil, err
	}

	c.Delegate.(*mlDelegate).meta = ml.LocalNode().Meta

	var entryNodes []*NodeInfo
	if e != nil {
		entryNodes, err = e.Nodes()
		if err != nil {
			// TODO: retry?
			return nil, err
		}
	}

	if len(entryNodes) > 0 {
		addresses := mapNodesToAddresses(entryNodes)
		_, err := ml.Join(addresses)
		if err != nil {
			// TODO: retry?
			return nil, err
		}
	}

	s := &Swarm{
		local:            nodeSpec,
		errors:           o.Errors,
		maxMessageBuffer: o.MaxMessageBuffer,
		leaveTimeout:     o.LeaveTimeout,
		getOutgoing:      getOutgoing,
		outgoing:         outgoing,
		incoming:         incoming,
		getValues:        getValues,
		listeners:        listeners,
		leave:            leave,
		shared:           shared,
	}

	go s.control()
	return s, nil
}

func reverse(b [][]byte) [][]byte {
	for i := range b[:len(b)/2] {
		b[i], b[len(b)-1-i] = b[len(b)-1-i], b[i]
	}

	return b
}

func takeMaxLatest(b [][]byte, overhead, max int) [][]byte {
	var (
		bb   [][]byte
		size int
	)

	for i := range b {
		bli := b[len(b)-i-1]

		if size+len(bli)+overhead > max {
			break
		}

		bb = append(bb, bli)
		size += len(bli) + overhead
	}

	return reverse(bb)
}

func encodeMessage(m *message) ([]byte, error) {
	// we're not saving the encoder together with the connections, because
	// even if the reflection info would be cached, it's very fragile and
	// complicated. These messages should be small, it should be OK to pay
	// this cost.

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(m)
	return buf.Bytes(), err
}

func decodeMessage(b []byte) (*message, error) {
	var m message
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	err := dec.Decode(&m)
	return &m, err
}

func (s *Swarm) control() {
	for {
		// TODO: regularly check the available instances

		select {
		case req := <-s.getOutgoing:
			s.messages = takeMaxLatest(s.messages, req.overhead, req.limit)
			req.ret <- s.messages
		case m := <-s.outgoing:
			s.messages = append(s.messages, m.encoded)
			s.messages = takeMaxLatest(s.messages, 0, s.maxMessageBuffer)
			if m.message.Type == sharedValue {
				s.shared.set(s.local.Name, m.message.Key, m.message.Value)
			}
		case b := <-s.incoming:
			m, err := decodeMessage(b)
			if err != nil {
				// assuming buffered error channels
				select {
				case s.errors <- err:
				default:
				}
			} else if m.Type == sharedValue {
				s.shared.set(m.Source, m.Key, m.Value)
			} else if m.Type == broadcast {
				for k, l := range s.listeners {
					if k == m.Key {
						// assuming buffered listener channels
						select {
						case l <- &Message{
							Source: m.Source,
							Value:  m.Value,
						}:
						default:
						}
					}
				}
			}
		case req := <-s.getValues:
			req.ret <- s.shared[req.key]
		case <-s.leave:
			// TODO: call shutdown
			if err := s.mlist.Leave(s.leaveTimeout); err != nil {
				select {
				case s.errors <- err:
				default:
				}
			}

			return
		}
	}
}

// TODO: memberlist has support for this, less redundant to use that
func (s *Swarm) Local() *NodeInfo { return s.local }

func (s *Swarm) broadcast(m *message) error {
	m.Source = s.Local().Name
	b, err := encodeMessage(m)
	if err != nil {
		return err
	}

	s.outgoing <- &outgoingMessage{
		message: m,
		encoded: b,
	}
	return nil
}

func (s *Swarm) Broadcast(m interface{}) error {
	return s.broadcast(&message{Type: broadcast, Value: m})
}

func (s *Swarm) ShareValue(key string, value interface{}) error {
	return s.broadcast(&message{Type: sharedValue, Key: key, Value: value})
}

func (s *Swarm) DeleteValue(string) error { return nil }

func (s *Swarm) Values(key string) map[string]interface{} {
	req := &valueReq{
		key: key,
		ret: make(chan map[string]interface{}),
	}
	s.getValues <- req
	return <-req.ret
}

func (s *Swarm) Members() []*NodeInfo            { return nil }
func (s *Swarm) State() NodeState                { return Initial }
func (s *Swarm) Instances() map[string]NodeState { return nil }

// assumed to buffered or may drop
func (s *Swarm) Listen(key string, c chan<- *Message) {}

func (s *Swarm) Leave() {
	close(s.leave)
}
