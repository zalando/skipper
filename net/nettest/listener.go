package nettest

import (
	"errors"
	"net"
	"sync"
	"time"
)

var (
	ErrListenerClosed = errors.New("failed to listen, closed")
)

type SlowAcceptListenerOptions struct {
	Network string
	Address string
	Delay   time.Duration
}

type SlowAcceptListener struct {
	Network string
	Address string

	mu    sync.Mutex
	delay time.Duration

	l    net.Listener
	once sync.Once
	quit chan struct{}
}

var _ net.Listener = &SlowAcceptListener{}

func (lo *SlowAcceptListener) listen(l net.Listener) error {
	lo.l = l
	return nil
}

func (lo *SlowAcceptListener) Delay(d time.Duration) {
	lo.mu.Lock()
	lo.delay = d
	lo.mu.Unlock()
}
func (lo *SlowAcceptListener) Accept() (net.Conn, error) {
	select {
	case <-lo.quit:
		return nil, ErrListenerClosed
	default:
	}

	conn, err := lo.l.Accept()
	if err != nil {
		return nil, err
	}

	lo.mu.Lock()
	time.Sleep(lo.delay) // slow accept
	lo.mu.Unlock()
	return conn, nil
}

func (lo *SlowAcceptListener) Addr() net.Addr {
	return lo.l.Addr()
}

func (lo *SlowAcceptListener) Close() error {
	if lo.l != nil {
		lo.l.Close()
	}
	lo.once.Do(func() { close(lo.quit) })

	return nil
}

var _ net.Listener = &SlowAcceptListener{}

func NewSlowAcceptListener(opt *SlowAcceptListenerOptions) (*SlowAcceptListener, error) {
	lo := &SlowAcceptListener{
		Network: opt.Network,
		Address: opt.Address,
		delay:   opt.Delay,
	}

	nl, err := net.Listen(lo.Network, lo.Address)
	if err != nil {
		return nil, err
	}

	lo.listen(nl)

	lo.quit = make(chan struct{})
	return lo, nil
}
