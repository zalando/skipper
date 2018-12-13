package swarm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"time"

	"github.com/zalando/skipper/metrics"

	"github.com/hashicorp/memberlist"
	log "github.com/sirupsen/logrus"
)

type swarmType int

const (
	swarmKubernetes swarmType = iota
	swarmFake
	swarmUnknown
)

func (st swarmType) String() string {
	switch st {
	case swarmKubernetes:
		return "kubernetes Swarm"
	case swarmFake:
		return "fake Swarm"
	}
	return "unkwown Swarm"
}

func getSwarmType(o Options) swarmType {
	if o.FakeSwarm {
		return swarmFake
	}
	if o.KubernetesOptions != nil {
		return swarmKubernetes
	}
	return swarmUnknown
}

const (
	// DefaultMaxMessageBuffer is the default maximum size of the
	// exchange packets send out to peers.
	DefaultMaxMessageBuffer = 1 << 22
	// DefaultPort is used as default to connect to other
	// known swarm peers.
	DefaultPort = 9990
	// DefaultLeaveTimeout is the default timeout to wait for responses
	// for a leave message send by this instance to other peers.
	DefaultLeaveTimeout = time.Duration(5 * time.Second)

	metricsPrefix = "swarm.messages."
)

var (
	ErrUnknownSwarm = errors.New("unknown swarm type")
)

// Options configure swarm objects.
type Options struct {
	swarm swarmType

	// MaxMessageBuffer is the maximum size of the exchange
	// packets send out to peers.
	MaxMessageBuffer int

	// LeaveTimeout is the timeout to wait for responses for a
	// leave message send by this instance to other peers.
	LeaveTimeout time.Duration

	// SwarmPort port to listen for incoming swarm packets.
	SwarmPort uint16

	// KubernetesOptions are options required to find your peers in Kubernetes
	KubernetesOptions *KubernetesOptions

	// FakeSwarm enable a test swarm
	FakeSwarm bool

	// FakeSwarmLocalNode is the node name of the local node
	// joining a fakeSwarm to have better log output
	FakeSwarmLocalNode string

	// Debug enables swarm debug logs and also enables memberlist logs
	Debug bool
}

// Swarm is the main type for exchanging low latency, weakly
// consistent information with other skipper peers.
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

	metrics metrics.Metrics

	cleanupF func()
}

// NewSwarm creates a Swarm for given Options.
func NewSwarm(o Options) (*Swarm, error) {
	switch getSwarmType(o) {
	case swarmKubernetes:
		return newKubernetesSwarm(o)
	case swarmFake:
		return newFakeSwarm(o)
	default:
		return nil, ErrUnknownSwarm
	}
}

func newFakeSwarm(o Options) (*Swarm, error) {
	o.swarm = swarmFake
	return Start(o)
}

func newKubernetesSwarm(o Options) (*Swarm, error) {
	o.swarm = swarmKubernetes

	u, err := buildAPIURL(o.KubernetesOptions.KubernetesInCluster, o.KubernetesOptions.KubernetesAPIBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes API url from url %s running in cluster %v: %v", o.KubernetesOptions.KubernetesAPIBaseURL, o.KubernetesOptions.KubernetesInCluster, err)
	}
	o.KubernetesOptions.KubernetesAPIBaseURL = u

	if o.SwarmPort <= 0 || o.SwarmPort >= 65535 {
		log.Errorf("Wrong SwarmPort %d, set to default %d instead", o.SwarmPort, DefaultMaxMessageBuffer)
		o.SwarmPort = DefaultPort
	}

	if o.KubernetesOptions.Namespace == "" {
		log.Errorf("Namespace is empty set to default %s instead", DefaultNamespace)
		o.KubernetesOptions.Namespace = DefaultNamespace
	}

	if o.KubernetesOptions.LabelSelectorKey == "" {
		log.Errorf("LabelSelectorKey is empty, set to default %s instead", DefaultLabelSelectorKey)
		o.KubernetesOptions.LabelSelectorKey = DefaultLabelSelectorKey
	}

	if o.KubernetesOptions.LabelSelectorValue == "" {
		log.Errorf("LabelSelectorValue is empty, set to default %s instead", DefaultLabelSelectorValue)
		o.KubernetesOptions.LabelSelectorValue = DefaultLabelSelectorValue
	}

	if o.MaxMessageBuffer <= 0 {
		log.Errorf("MaxMessageBuffer <= 0, setting to default %d instead", DefaultMaxMessageBuffer)
		o.MaxMessageBuffer = DefaultMaxMessageBuffer
	}

	if o.LeaveTimeout <= 0 {
		log.Errorf("LeaveTimeout <= 0, setting to default %d instead", DefaultLeaveTimeout)
		o.LeaveTimeout = DefaultLeaveTimeout
	}

	return Start(o)
}

// Start will find Swarm peers based on the chosen swarm type and join
// the Swarm.
func Start(o Options) (*Swarm, error) {
	knownEntryPoint, cleanupF := newKnownEntryPoint(o)
	log.Debugf("knownEntryPoint: %s, %v", knownEntryPoint.Node(), knownEntryPoint.Nodes())
	return Join(o, knownEntryPoint.Node(), knownEntryPoint.Nodes(), cleanupF)
}

// Join will join given Swarm peers and return an initialiazed Swarm
// object if successful.
func Join(o Options, self *NodeInfo, nodes []*NodeInfo, cleanupF func()) (*Swarm, error) {
	log.Infof("SWARM: %s is going to join swarm of %d nodes (%v)", self, len(nodes), nodes)
	cfg := memberlist.DefaultLocalConfig()
	if !o.Debug {
		cfg.LogOutput = ioutil.Discard
	}

	if self.Name == "" {
		self.Name = cfg.Name
	} else {
		cfg.Name = self.Name
	}
	if self.Addr == nil {
		self.Addr = net.ParseIP(cfg.BindAddr)
	} else {
		cfg.BindAddr = self.Addr.String()
		cfg.AdvertiseAddr = cfg.BindAddr
	}
	if self.Port == 0 {
		self.Port = uint16(cfg.BindPort)
	} else {
		cfg.BindPort = int(self.Port)
		cfg.AdvertisePort = cfg.BindPort
	}

	getOutgoing := make(chan reqOutgoing)
	outgoing := make(chan *outgoingMessage)
	incoming := make(chan []byte)
	getValues := make(chan *valueReq)
	listeners := make(map[string]chan<- *Message)
	leave := make(chan struct{})
	shared := make(sharedValues)

	cfg.Delegate = &mlDelegate{
		outgoing: getOutgoing,
		incoming: incoming,
	}
	ml, err := memberlist.Create(cfg)
	if err != nil {
		log.Errorf("SWARM: failed to create memberlist: %v", err)
		return nil, err
	}
	cfg.Delegate.(*mlDelegate).meta = ml.LocalNode().Meta

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
		maxMessageBuffer: o.MaxMessageBuffer,
		leaveTimeout:     o.LeaveTimeout,
		getOutgoing:      getOutgoing,
		outgoing:         outgoing,
		incoming:         incoming,
		getValues:        getValues,
		listeners:        listeners,
		leave:            leave,
		shared:           shared,
		mlist:            ml,
		cleanupF:         cleanupF,
		metrics:          metrics.Default,
	}

	go s.control()

	return s, nil
}

// control is the control loop of a Swarm member.
func (s *Swarm) control() {
	for {
		select {
		case req := <-s.getOutgoing:
			s.messages = takeMaxLatest(s.messages, req.overhead, req.limit)
			if len(s.messages) <= 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			req.ret <- s.messages
		case m := <-s.outgoing:
			s.messages = append(s.messages, m.encoded)
			s.metrics.UpdateGauge(metricsPrefix+"outgoing.queue", float64(len(s.messages)))
			s.messages = takeMaxLatest(s.messages, 0, s.maxMessageBuffer)
			if m.message.Type == sharedValue {
				log.Debugf("SWARM: %s shares value: %s: %v", s.Local().Name, m.message.Key, m.message.Value)
				s.shared.set(s.Local().Name, m.message.Key, m.message.Value)
				s.metrics.IncCounter(metricsPrefix + "outgoing.shared")
			}
		case b := <-s.incoming:
			s.metrics.IncCounter(metricsPrefix + "incoming.all")
			m, err := decodeMessage(b)
			if err != nil {
				log.Errorf("SWARM: Failed to decode message: %v", err)
			} else if m.Type == sharedValue {
				s.metrics.IncCounter(metricsPrefix + "incoming.shared")
				log.Debugf("SWARM: %s got shared value from %s: %s: %v", s.Local().Name, m.Source, m.Key, m.Value)
				s.shared.set(m.Source, m.Key, m.Value)
			} else if m.Type == broadcast {
				s.metrics.IncCounter(metricsPrefix + "incoming.broadcast")
				log.Debugf("SWARM: got broadcast value: %s %s: %v", m.Source, m.Key, m.Value)
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
			log.Debugf("SWARM: getValues for key: %s", req.key)
			req.ret <- s.shared[req.key]
		case <-s.leave:
			log.Debugf("SWARM: %s got leave signal", s.Local())
			if s.mlist == nil {
				log.Warningf("SWARM: Leave called, but %s already seem to be left", s.Local())
				return
			}
			if err := s.mlist.Leave(s.leaveTimeout); err != nil {
				log.Errorf("SWARM: Failed to leave mlist: %v", err)
			}
			if err := s.mlist.Shutdown(); err != nil {
				log.Errorf("SWARM: Failed to shutdown mlist: %v", err)
			}
			log.Infof("SWARM: %s left", s.Local())
			return
		}
	}
}

// Local is a getter to the local member of a swarm.
func (s *Swarm) Local() *NodeInfo {
	if s == nil {
		log.Errorf("swarm is nil")
		return nil
	}
	if s.mlist == nil {
		log.Warningf("deprecated way of getting local node")
		return s.local
	}
	mlNode := s.mlist.LocalNode()
	return &NodeInfo{
		Name: mlNode.Name,
		Addr: mlNode.Addr,
		Port: mlNode.Port,
	}
}

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

// Broadcast sends a broadcast message with a value to all peers.
func (s *Swarm) Broadcast(m interface{}) error {
	return s.broadcast(&message{Type: broadcast, Value: m})
}

// ShareValue sends a broadcast message with a sharedValue to all
// peers. It implements the ratelimit.Swarmer interface.
func (s *Swarm) ShareValue(key string, value interface{}) error {
	return s.broadcast(&message{Type: sharedValue, Key: key, Value: value})
}

// Values sends a request and wait blocking for a response. It
// implements the ratelimit.Swarmer interface.
func (s *Swarm) Values(key string) map[string]interface{} {
	req := &valueReq{
		key: key,
		ret: make(chan map[string]interface{}),
	}
	s.getValues <- req
	return <-req.ret
}

// Leave sends a signal for the local node to leave the Swarm.
func (s *Swarm) Leave() {
	close(s.leave)
	s.cleanupF()
}
