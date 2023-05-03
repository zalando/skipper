package swarmtest

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

type CustomNetTransport struct {
	NetTransport *memberlist.NetTransport
	addr         memberlist.Address
	shutdown     chan struct{}
	once         sync.Once
}

func NewCustomNetTransport(config *memberlist.NetTransportConfig, addr memberlist.Address) (*CustomNetTransport, error) {
	transp, err := memberlist.NewNetTransport(config)
	if err != nil {
		return nil, err
	}
	return &CustomNetTransport{
		addr:         addr,
		NetTransport: transp,
		shutdown:     make(chan struct{}),
		once:         sync.Once{},
	}, nil
}

func (t *CustomNetTransport) SelfAddress() memberlist.Address {
	return t.addr
}

func (t *CustomNetTransport) DialAddressTimeout(addr memberlist.Address, timeout time.Duration) (net.Conn, error) {
	select {
	case <-t.shutdown:
		return nil, errors.New("shut down the node on request")
	default:
		return t.NetTransport.DialAddressTimeout(addr, timeout)
	}

}

func (t *CustomNetTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	select {
	case <-t.shutdown:
		return nil, errors.New("shut down the node on request")
	default:
		return t.NetTransport.DialTimeout(addr, timeout)
	}
}

func (t *CustomNetTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	select {
	case <-t.shutdown:
		return nil, -1, errors.New("shut down the node on request")
	default:
		return t.NetTransport.FinalAdvertiseAddr(ip, port)
	}
}

// WriteToAddress is using memberlist transport to implement a NodeAwareTransport
func (t *CustomNetTransport) WriteToAddress(b []byte, addr memberlist.Address) (time.Time, error) {
	select {
	case <-t.shutdown:
		return time.Now(), errors.New("shut down the node on request" + addr.String())
	default:
		return t.NetTransport.WriteToAddress(b, addr)
	}
}

// WriteTo will call WriteToAddress via memberlist lib func
func (t *CustomNetTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	select {
	case <-t.shutdown:
		return time.Now(), errors.New("shut down the node on request" + addr)
	default:

		return t.NetTransport.WriteTo(b, addr)
	}
}

func (t *CustomNetTransport) PacketCh() <-chan *memberlist.Packet {
	return t.NetTransport.PacketCh()
}

func (t *CustomNetTransport) StreamCh() <-chan net.Conn {
	return t.NetTransport.StreamCh()
}

func (t *CustomNetTransport) Shutdown() error {
	return t.NetTransport.Shutdown()
}

func (t *CustomNetTransport) Exit() {
	t.once.Do(func() {
		close(t.shutdown)
	})
}
