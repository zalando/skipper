package queuelistener

import (
	"errors"
	"net"
	"sync"
	"time" // eventually, every Go file imports time

	"github.com/zalando/skipper/logging"
)

// TODO: we need to monitor the queue size and allow horizontal scaling based on it

const (
	initialBounceDelay              = 500 * time.Microsecond
	maxBounceDelay                  = 100 * time.Millisecond
	defaultActiveMemoryLimitBytes   = 150 * 1000 * 1000
	defaultActiveConnectionBytes    = 50 * 1000
	defaultInactiveMemoryLimitBytes = 150 * 1000 * 1000
	defaultInactiveConnectionBytes  = 5 * 1000
)

type connection struct {
	net     net.Conn
	release chan<- struct{}
	quit    <-chan struct{}
}

type Options struct {
	Network                  string
	Address                  string
	ActiveMemoryLimitBytes   int
	ActiveConnectionBytes    int
	InactiveMemoryLimitBytes int
	InactiveConnectionBytes  int
	Log                      logging.Logger
}

type listener struct {
	options           Options
	maxConcurrency    int
	maxQueueSize      int
	externalListener  net.Listener
	acceptExternal    chan net.Conn
	externalError     chan error
	acceptInternal    chan net.Conn
	internalError     chan error
	releaseConnection chan struct{}
	quit              chan struct{}
	closeMx           sync.Mutex
	closedHook        chan struct{} // for testing
}

var (
	token             struct{}
	errListenerClosed = errors.New("listener closed")
)

func (c connection) Read(b []byte) (n int, err error)   { return c.net.Read(b) }
func (c connection) Write(b []byte) (n int, err error)  { return c.net.Write(b) }
func (c connection) LocalAddr() net.Addr                { return c.net.LocalAddr() }
func (c connection) RemoteAddr() net.Addr               { return c.net.RemoteAddr() }
func (c connection) SetDeadline(t time.Time) error      { return c.net.SetDeadline(t) }
func (c connection) SetReadDeadline(t time.Time) error  { return c.net.SetReadDeadline(t) }
func (c connection) SetWriteDeadline(t time.Time) error { return c.net.SetWriteDeadline(t) }

func (c connection) Close() error {
	select {
	case c.release <- token:
	case <-c.quit:
	}

	return c.net.Close()
}

func Listen(o Options) (net.Listener, error) {
	nl, err := net.Listen(o.Network, o.Address)
	if err != nil {
		return nil, err
	}

	if o.ActiveMemoryLimitBytes <= 0 {
		o.ActiveMemoryLimitBytes = defaultActiveMemoryLimitBytes
	}

	if o.ActiveConnectionBytes <= 0 {
		o.ActiveConnectionBytes = defaultActiveConnectionBytes
		if o.ActiveMemoryLimitBytes < o.ActiveConnectionBytes {
			o.ActiveMemoryLimitBytes = o.ActiveConnectionBytes
		}
	}

	if o.InactiveMemoryLimitBytes <= 0 {
		o.InactiveMemoryLimitBytes = defaultInactiveMemoryLimitBytes
	}

	if o.InactiveConnectionBytes <= 0 {
		o.InactiveConnectionBytes = defaultInactiveConnectionBytes
		if o.InactiveMemoryLimitBytes < o.InactiveConnectionBytes {
			o.InactiveMemoryLimitBytes = o.InactiveConnectionBytes
		}
	}

	maxConcurrency := o.ActiveMemoryLimitBytes / o.ActiveConnectionBytes
	maxQueueSize := o.InactiveMemoryLimitBytes / o.InactiveConnectionBytes

	println("queue values:", maxConcurrency, maxQueueSize)

	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	l := &listener{
		options:           o,
		maxConcurrency:    maxConcurrency,
		maxQueueSize:      maxQueueSize,
		externalListener:  nl,
		acceptExternal:    make(chan net.Conn),
		externalError:     make(chan error),
		acceptInternal:    make(chan net.Conn),
		internalError:     make(chan error),
		releaseConnection: make(chan struct{}),
		quit:              make(chan struct{}),
	}

	go l.listenExternal()
	go l.listenInternal()
	return l, nil
}

func bounce(delay time.Duration) time.Duration {
	if delay == 0 {
		return initialBounceDelay
	}

	delay *= 2
	if delay > maxBounceDelay {
		delay = maxBounceDelay
	}

	return delay
}

// this function turns net.Listener.Accep() into a channel, so that we can use select{} while it is blocked
func (l *listener) listenExternal() {
	var (
		c              net.Conn
		err            error
		delay          time.Duration
		acceptExternal chan<- net.Conn
		externalError  chan<- error
		retry          <-chan time.Time
	)

	for {
		c, err = l.externalListener.Accept()
		if err != nil {
			// based on net/http.Server.Serve():
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				delay = bounce(delay)
				l.options.Log.Errorf(
					"Queue listener: accept error: %v, retrying in %v.",
					err,
					delay,
				)

				err = nil
				acceptExternal = nil
				externalError = nil
				retry = time.After(delay)
			} else {
				acceptExternal = nil
				externalError = l.externalError
				retry = nil
				delay = 0
			}
		} else {
			println("net received")
			acceptExternal = l.acceptExternal
			externalError = nil
			retry = nil
			delay = 0
		}

		select {
		case acceptExternal <- c:
		case externalError <- err:
			// we cannot accept anymore, but we have returned the permanent error
			return
		case <-retry:
		case <-l.quit:
			return
		}
	}
}

func (l *listener) listenInternal() {
	var (
		concurrency    int
		queue          []net.Conn
		conn           net.Conn
		nextConn       net.Conn
		err            error
		acceptExternal <-chan net.Conn
		acceptInternal chan<- net.Conn
		internalError  chan<- error
	)

	queue = make([]net.Conn, 0, l.maxQueueSize)
	for {
		// TODO: timeout in the queue. What is the right and expected value?

		if len(queue) < l.maxQueueSize {
			acceptExternal = l.acceptExternal
		} else {
			acceptExternal = nil
		}

		if len(queue) > 0 && concurrency < l.maxConcurrency {
			println("can send", concurrency, l.maxConcurrency)
			acceptInternal = l.acceptInternal
			nextConn = queue[len(queue)-1]
		} else {
			acceptInternal = nil
		}

		if err != nil && len(queue) == 0 {
			internalError = l.internalError
		} else {
			internalError = nil
		}

		select {
		case conn = <-acceptExternal:
			println("received", len(queue))
			queue = append(queue, connection{
				net:     conn,
				release: l.releaseConnection,
				quit:    l.quit,
			})
		case err = <-l.externalError:
		case acceptInternal <- nextConn:
			queue = queue[:len(queue)-1]
			concurrency++
		case internalError <- err:
			// we cannot accept anymore, but we returned the permanent error
			err = nil
			l.Close()
		case <-l.releaseConnection:
			concurrency--
		case <-l.quit:
			for _, c := range queue {
				c.Close()
			}

			// Closing the real listener in a separate goroutine is based on inspecting the
			// stdlib. It's fair to just log the errors.
			if err := l.externalListener.Close(); err != nil {
				l.options.Log.Errorf("Failed to close network listener: %v.", err)
			}

			if l.closedHook != nil {
				close(l.closedHook)
			}

			return
		}
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.acceptInternal:
		return c, nil
	case err := <-l.internalError:
		return nil, err
	case <-l.quit:
		return nil, errListenerClosed
	}
}

func (l *listener) Addr() net.Addr {
	return l.externalListener.Addr()
}

func (l *listener) Close() error {
	// allow closing concurrently as net/http.Server may or may not close it and avoid panic on
	// close(l.quit)

	l.closeMx.Lock()
	defer l.closeMx.Unlock()

	select {
	case <-l.quit:
	default:
		close(l.quit)
	}

	return nil
}
