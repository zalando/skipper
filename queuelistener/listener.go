package queuelistener

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
)

const (
	initialBounceDelay              = 500 * time.Microsecond
	maxBounceDelay                  = 100 * time.Millisecond
	defaultMemoryLimitBytes         = 150 * 1000 * 1000
	defaultConnectionBytes          = 50 * 1000
	queueTimeoutPrecisionPercentage = 5
	maxCalculatedQueueSize          = 50_000
	acceptedConnectionsKey          = "listener.accepted.connections"
	queuedConnectionsKey            = "listener.queued.connections"
	queueTimeoutKey                 = "listener.queued.timeouts"
	acceptLatencyKey                = "listener.accept.latency"
)

type external struct {
	net.Conn
	accepted time.Time
}

type connection struct {
	*external
	queueDeadline time.Time
	release       chan<- struct{}
	quit          <-chan struct{}
	once          sync.Once
	closeErr      error
}

// Options are used to initialize the queue listener.
type Options struct {

	// Network sets the name of the network. Compatible with the `network` argument
	// to net.Listen().
	Network string

	// Address sets the listener address, e.g. :9090. Compatible with the `address`
	// argument to net.Listen().
	Address string

	// MaxConcurrency sets the maximum accepted connections.
	MaxConcurrency int

	// MaxQueue size sets the maximum allowed queue size of pending connections. When
	// not set, it is derived from the MaxConcurrency value.
	MaxQueueSize int

	// MemoryLimitBytes sets the approximated maximum memory used by the accepted
	// connections, calculated together with the ConnectionBytes value. Defaults to
	// 150MB.
	//
	// When MaxConcurrency is set, this field is ignored.
	MemoryLimitBytes int64

	// ConnectionBytes is used to calculate the MaxConcurrency when MaxConcurrency is
	// not set explicitly but calculated from MemoryLimitBytes.
	ConnectionBytes int

	// QueueTimeout set the time limit for pending connections spent in the queue. It
	// should be set to a similar value as the ReadHeaderTimeout of net/http.Server.
	QueueTimeout time.Duration

	// Metrics is used to collect monitoring data about the queue, including the current
	// concurrent connections and the number of connections in the queue.
	Metrics metrics.Metrics

	// Log is used to log unexpected, non-fatal errors. It defaults to logging.DefaultLog.
	Log logging.Logger

	testQueueChangeHook chan struct{}
}

type listener struct {
	metrics           metrics.Metrics
	log               logging.Logger
	options           Options
	maxConcurrency    int64
	maxQueueSize      int64
	externalListener  net.Listener
	acceptExternal    chan *external
	externalError     chan error
	acceptInternal    chan *connection
	internalError     chan error
	releaseConnection chan struct{}
	quit              chan struct{}
	closeMx           sync.Mutex
	closedHook        chan struct{} // for testing
}

var (
	token             struct{}
	errListenerClosed = errors.New("listener closed")
	errAcceptTimeout  = errors.New("accept timeout")
)

func (c *connection) Close() error {
	c.once.Do(func() {
		select {
		case c.release <- token:
		case <-c.quit:
		}

		c.closeErr = c.external.Close()
	})

	return c.closeErr
}

func (o Options) maxConcurrency() int64 {
	if o.MaxConcurrency > 0 {
		return int64(o.MaxConcurrency)
	}

	maxConcurrency := o.MemoryLimitBytes / int64(o.ConnectionBytes)

	// theoretical minimum, but rather only for testing. When the max concurrency is not set, then the
	// TCP-LIFO should not be used, at all.
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	return maxConcurrency
}

func (o Options) maxQueueSize() int64 {
	if o.MaxQueueSize > 0 {
		return int64(o.MaxQueueSize)
	}

	maxQueueSize := min(10*o.maxConcurrency(), maxCalculatedQueueSize)

	return maxQueueSize
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
		log:               o.Log,
		metrics:           o.Metrics,
		maxConcurrency:    o.maxConcurrency(),
		maxQueueSize:      o.maxQueueSize(),
		externalListener:  nl,
		acceptExternal:    make(chan *external),
		externalError:     make(chan error),
		acceptInternal:    make(chan *connection),
		internalError:     make(chan error),
		releaseConnection: make(chan struct{}),
		quit:              make(chan struct{}),
	}
	if l.metrics == nil {
		l.metrics = metrics.NoMetric{}
	}
	l.log.Infof("TCP lifo listener config: %s", l)

	go l.listenExternal()
	go l.listenInternal()
	return l, nil
}

// Listen creates and initializes a listener that can be used to limit the
// concurrently accepted incoming client connections.
//
// The queue listener will return only a limited number of concurrent connections
// by its Accept() method, defined by the max concurrency configuration. When the
// max concurrency is reached, the Accept() method will block until one or more
// accepted connections are closed. When the max concurrency limit is reached, the
// new incoming client connections are stored in a queue. When an active (accepted)
// connection is closed, the listener will return the most recent one from the
// queue (LIFO). When the queue is full, the oldest pending connection is closed
// and dropped, and the new one is inserted into the queue.
//
// The listener needs to be closed in order to release local resources. After it is
// closed, Accept() returns an error without blocking.
//
// See type Options for info about the configuration of the listener.
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

func (l *listener) String() string {
	return fmt.Sprintf("concurrency: %d, queue size: %d, memory limit: %d, bytes per connection: %d, queue timeout: %s", l.maxConcurrency, l.maxQueueSize, l.options.MemoryLimitBytes, l.options.ConnectionBytes, l.options.QueueTimeout)
}

// this function turns net.Listener.Accept() into a channel, so that we can use select{} while it is blocked
func (l *listener) listenExternal() {
	var (
		c              net.Conn
		ex             *external
		err            error
		delay          time.Duration
		acceptExternal chan<- *external
		externalError  chan<- error
		retry          <-chan time.Time
	)

	for {
		c, err = l.externalListener.Accept()
		if err != nil {
			// based on net/http.Server.Serve():
			//lint:ignore SA1019 Temporary is deprecated in Go 1.18, but keep it for now (https://github.com/zalando/skipper/issues/1992)
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				delay = bounce(delay)
				l.log.Errorf(
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
			ex = &external{c, time.Now()}
			externalError = nil
			retry = nil
			delay = 0
		}

		select {
		case acceptExternal <- ex:
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
		concurrency    int64
		queue          *ring
		err            error
		acceptInternal chan<- *connection
		internalError  chan<- error
		nextTimeout    <-chan time.Time
	)

	queue = newRing(l.maxQueueSize)
	for {
		var nextConn *connection
		if queue.size > 0 && concurrency < l.maxConcurrency {
			acceptInternal = l.acceptInternal
			nextConn = queue.peek().(*connection)
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

		l.metrics.UpdateGauge(acceptedConnectionsKey, float64(concurrency))
		l.metrics.UpdateGauge(queuedConnectionsKey, float64(queue.size))

		select {
		case conn := <-l.acceptExternal:
			cc := &connection{
				external: conn,
				release:  l.releaseConnection,
				quit:     l.quit,
				once:     sync.Once{},
			}

			if l.options.QueueTimeout > 0 {
				cc.queueDeadline = time.Now().Add(l.options.QueueTimeout)
			}

			drop := queue.enqueue(cc)
			if drop != nil {
				drop.(*connection).external.Close()
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
			for queue.size > 0 && queue.peekOldest().(*connection).queueDeadline.Before(now) {
				drop := queue.dequeueOldest()
				drop.(*connection).external.Close()
			}

			nextTimeout = nil
			l.testNotifyQueueChange()
			if dropped > 0 {
				l.testNotifyQueueChange()
			}
		case <-l.quit:
			queue.rangeOver(func(c net.Conn) { c.(*connection).external.Close() })

			// Closing the real listener in a separate goroutine is based on inspecting the
			// stdlib. It's fair to just log the errors.
			if err := l.externalListener.Close(); err != nil {
				l.log.Errorf("Failed to close network listener: %v.", err)
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
		l.metrics.MeasureSince(acceptLatencyKey, c.external.accepted)
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
