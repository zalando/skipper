package queuelistener

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
)

// TODO: we need to monitor the queue size and allow horizontal scaling based on it

const (
	initialBounceDelay              = 500 * time.Microsecond
	maxBounceDelay                  = 100 * time.Millisecond
	defaultMemoryLimitBytes         = 150 * 1000 * 1000
	defaultConnectionBytes          = 50 * 1000
	queueTimeoutPrecisionPercentage = 5
	maxCalculatedQueueSize          = 50_000
	acceptedConnectionsKey          = "listener.accepted.connections"
	queuedConnectionsKey            = "listener.queued.connections"
)

type connection struct {
	net           net.Conn
	queueDeadline time.Time
	release       chan<- struct{}
	quit          <-chan struct{}
}

type Options struct {
	Network          string
	Address          string
	MaxConcurrency   int
	MaxQueueSize     int
	MemoryLimitBytes int
	ConnectionBytes  int
	QueueTimeout     time.Duration
	Metrics          metrics.Metrics
	Log              logging.Logger

	testQueueChangeHook chan struct{}
}

func (o Options) maxConcurrency() int {
	if o.MaxConcurrency > 0 {
		return o.MaxConcurrency
	}

	maxConcurrency := o.MemoryLimitBytes / o.ConnectionBytes
	// TODO:
	// - should be probably also based on "nofiles" value (check Go runtime, if they are based on it)

	// theoretical minimum, but rather only for testing. When the max concurrency is not set, then the
	// TCP-LIFO should not be used, at all.
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}

	return maxConcurrency
}

func (o Options) maxQueueSize() int {
	if o.MaxQueueSize > 0 {
		return o.MaxQueueSize
	}

	maxQueueSize := 10 * o.maxConcurrency()
	if maxQueueSize > maxCalculatedQueueSize {
		maxQueueSize = maxCalculatedQueueSize
	}

	return maxQueueSize
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

func listenWith(nl net.Listener, o Options) (net.Listener, error) {
	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	if o.MemoryLimitBytes <= 0 {
		o.MemoryLimitBytes = defaultMemoryLimitBytes
	}

	if o.ConnectionBytes <= 0 {
		o.ConnectionBytes = defaultConnectionBytes
	}

	l := &listener{
		options:           o,
		maxConcurrency:    o.maxConcurrency(),
		maxQueueSize:      o.maxQueueSize(),
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

func Listen(o Options) (net.Listener, error) {
	nl, err := net.Listen(o.Network, o.Address)
	if err != nil {
		return nil, err
	}

	return listenWith(nl, o)
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

// this function turns net.Listener.Accept() into a channel, so that we can use select{} while it is blocked
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
					"queue listener: accept error: %v, retrying in %v",
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
			if c != nil {
				c.Close()
			}

			return
		}
	}
}

func (l *listener) listenInternal() {
	var (
		concurrency    int
		queue          *ring
		err            error
		acceptInternal chan<- net.Conn
		internalError  chan<- error
		nextTimeout    <-chan time.Time
	)

	queue = newRing(l.maxQueueSize)
	for {
		// TODO: timeout in the queue. What is the right and expected value?

		var nextConn net.Conn
		if queue.size > 0 && concurrency < l.maxConcurrency {
			acceptInternal = l.acceptInternal
			nextConn = queue.peek()
		} else {
			acceptInternal = nil
		}

		if err != nil && queue.size == 0 {
			internalError = l.internalError
		} else {
			internalError = nil
		}

		// setting the timeout period to a fixed min value, that is a percentage of the queue timeout.
		// This way we can avoid for one too many rapid timeout events of stalled connections, and
		// second, we can also ensure a certain precision of the timeouts and the minimum queue
		// timeout.
		if l.options.QueueTimeout > 0 && nextTimeout == nil {
			nextTimeout = time.After(
				l.options.QueueTimeout * queueTimeoutPrecisionPercentage / 100,
			)
		}

		if l.options.Metrics != nil {
			l.options.Metrics.UpdateGauge(acceptedConnectionsKey, float64(concurrency))
			l.options.Metrics.UpdateGauge(queuedConnectionsKey, float64(queue.size))
		}

		select {
		case conn := <-l.acceptExternal:
			cc := connection{
				net:     conn,
				release: l.releaseConnection,
				quit:    l.quit,
			}

			if l.options.QueueTimeout > 0 {
				cc.queueDeadline = time.Now().Add(l.options.QueueTimeout)
			}

			drop := queue.enqueue(cc)
			if drop != nil {
				drop.(connection).net.Close()
			}

			l.testNotifyQueueChange()
		case err = <-l.externalError:
		case acceptInternal <- nextConn:
			queue.dequeue()
			concurrency++
			l.testNotifyQueueChange()
		case internalError <- err:
			// we cannot accept anymore, but we returned the permanent error
			err = nil
			l.Close()
		case <-l.releaseConnection:
			concurrency--
		case now := <-nextTimeout:
			var dropped int
			for queue.size > 0 && queue.peekOldest().(connection).queueDeadline.Before(now) {
				drop := queue.dequeueOldest()
				drop.(connection).net.Close()
			}

			nextTimeout = nil
			l.testNotifyQueueChange()
			if dropped > 0 {
				l.testNotifyQueueChange()
			}
		case <-l.quit:
			queue.rangeOver(func(c net.Conn) { c.(connection).net.Close() })

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

func (l *listener) testNotifyQueueChange() {
	if l.options.testQueueChangeHook == nil {
		return
	}

	select {
	case l.options.testQueueChangeHook <- token:
	default:
	}
}

func (l *listener) clearQueueChangeHook() {
	if l.options.testQueueChangeHook == nil {
		return
	}

	for {
		select {
		case <-l.options.testQueueChangeHook:
		default:
			return
		}
	}
}
