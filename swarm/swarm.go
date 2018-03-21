package swarm

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/memberlist"
	log "github.com/sirupsen/logrus"
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
	DefaultSwarmPort        = 9990
)

type NodeInfo struct {
	Name string
	Addr net.IP
	Port int
}

func (ni *NodeInfo) String() string {
	return fmt.Sprintf("NodeInfo{%s, %s, %d}", ni.Name, ni.Addr, ni.Port)
}

// Self can return itself as NodeInfo
type Self interface {
	Node() *NodeInfo
}

// EntryPoint knows its peers of nodes which contains itself
type EntryPoint interface {
	Nodes() []*NodeInfo
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
	// leaky, expected to be buffered, or errors are lost
	Errors chan<- error

	MaxMessageBuffer int

	LeaveTimeout time.Duration

	KubernetesAPIBaseURL string
	Namespace            string
	LabelSelectorKey     string
	LabelSelectorValue   string
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
		Errors:               make(chan<- error), // FIXME - do WE have to read this, or...?
		MaxMessageBuffer:     100,
		LeaveTimeout:         time.Duration(5 * time.Second),
		KubernetesAPIBaseURL: "https://10.3.0.1",
		Namespace:            "kube-system",
		LabelSelectorKey:     "application",
		LabelSelectorValue:   "skipper-ingress",
	})
}

func NewKnownEntryPoint(o Options) *KnownEPoint {
	nic := NewNodeInfoClient(o.KubernetesAPIBaseURL, o.Namespace, o.LabelSelectorKey, o.LabelSelectorValue)
	nodes, err := nic.GetNodeInfo()
	if err != nil {
		log.Fatalf("SWARM: Failed to get nodeinfo: %v", err)
	}
	self := getSelf(nodes)
	log.Infof("SWARM: Join swarm self=%s, nodes=%v", self, nodes)
	return &KnownEPoint{self: self, nodes: nodes}
}

func getSelf(nodes []*NodeInfo) *NodeInfo {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("SWARM: Failed to get addr: %v", err)
	}

	for _, ni := range nodes {
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				log.Errorf("SWARM: could not parse cidr: %v", err)
				continue
			}
			if ip.Equal(ni.Addr) {
				return ni
			}
		}
	}
	return nil
}

func (e *KnownEPoint) Node() *NodeInfo {
	if e == nil {
		return nil
	}
	return e.self
}

func (e *KnownEPoint) Nodes() []*NodeInfo {
	if e == nil {
		return nil
	}
	return e.nodes
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
	knownEntryPoint := NewKnownEntryPoint(o)
	return Join(o, knownEntryPoint.Node(), knownEntryPoint.Nodes())
}

func Join(o Options, self *NodeInfo, nodes []*NodeInfo) (*Swarm, error) {
	log.Infof("SWARM: Going to join swarm of %d nodes, self=%s", len(nodes), self)
	c := memberlist.DefaultLocalConfig()

	if self.Name == "" {
		self.Name = c.Name
	} else {
		c.Name = self.Name
	}

	if self.Addr == nil {
		self.Addr = net.ParseIP(c.BindAddr)
	} else {
		c.BindAddr = self.Addr.String()
		c.AdvertiseAddr = c.BindAddr
	}
	if self.Port == 0 {
		self.Port = c.BindPort
	} else {
		c.BindPort = self.Port
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
		log.Errorf("SWARM: failed to create memberlist: %v", err)
		return nil, err
	}

	c.Delegate.(*mlDelegate).meta = ml.LocalNode().Meta

	if len(nodes) > 0 {
		addresses := mapNodesToAddresses(nodes)
		_, err := ml.Join(addresses)
		if err != nil {
			log.Errorf("SWARM: failed to join: %v", err)
			return nil, err
		}
	}

	s := &Swarm{
		local:            self,
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
			if len(s.messages) > 0 {
				log.Infof("SWARM: getOutgoing %d messages", len(s.messages))
			} else {
				log.Debug("SWARM: getOutgoing with 0 messages")
			}
			req.ret <- s.messages
		case m := <-s.outgoing:
			s.messages = append(s.messages, m.encoded)
			s.messages = takeMaxLatest(s.messages, 0, s.maxMessageBuffer)
			log.Infof("SWARM: outgoing %d messages", len(s.messages))
			if m.message.Type == sharedValue {
				log.Infof("SWARM: share value: %s %s: %v", s.local.Name, m.message.Key, m.message.Value)
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
				log.Infof("SWARM: got shared value: %s %s: %v", m.Source, m.Key, m.Value)
				s.shared.set(m.Source, m.Key, m.Value)
			} else if m.Type == broadcast {
				log.Infof("SWARM: got broadcast value: %s %s: %v", m.Source, m.Key, m.Value)
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
			log.Infof("SWARM: getValues for key: %s", req.key)
			req.ret <- s.shared[req.key]
		case <-s.leave:
			log.Infof("SWARM: leaving %s", s.local)
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
