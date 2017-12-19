package swarmtest

import (
	"errors"
	"github.com/hashicorp/memberlist"
	"net"
	"time"
)

type CustomNetTransport struct {
	NetTransport *memberlist.NetTransport
	shutdown     chan struct{}
}

func NewCustomNetTransport(config *memberlist.NetTransportConfig) (*CustomNetTransport, error) {
	transp, err := memberlist.NewNetTransport(config)
	if err != nil {
		return nil, err
	}
	return &CustomNetTransport{
		NetTransport: transp,
		shutdown:     make(chan struct{}),
	}, nil
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
	close(t.shutdown)
}
