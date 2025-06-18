package queuelistener

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
)

type stackListener struct {
	log              logging.Logger
	metrics          metrics.Metrics
	maxConcurrency   int64
	maxQueueSize     int64
	memoryLimitBytes int64
	connectionBytes  int
	queueTimeout     time.Duration
	stack            *naiveStack[external]
	externalListener net.Listener
	acceptInternal   chan external
	quit             chan struct{}
	once             sync.Once
}

func StackListener(o Options) (net.Listener, error) {
	nl, err := net.Listen(o.Network, o.Address)
	if err != nil {
		return nil, fmt.Errorf("StackListener failed net.Listen: %w", err)
	}

	acceptCH := make(chan external)

	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	if o.MemoryLimitBytes <= 0 {
		o.MemoryLimitBytes = defaultMemoryLimitBytes
	}

	if o.ConnectionBytes <= 0 {
		o.ConnectionBytes = defaultConnectionBytes
	}

	m := o.Metrics
	if m == nil {
		m = metrics.NoMetric{}
	}
	l := &stackListener{
		log:              o.Log,
		metrics:          m,
		externalListener: nl,
		maxConcurrency:   o.maxConcurrency(),
		maxQueueSize:     o.maxQueueSize(),
		memoryLimitBytes: o.MemoryLimitBytes,
		connectionBytes:  o.ConnectionBytes,
		queueTimeout:     o.QueueTimeout,
		stack:            NewStack(),
		acceptInternal:   acceptCH,
		quit:             make(chan struct{}),
		once:             sync.Once{},
	}
	l.log.Infof("TCP lifo listener config: %s", l)

	go l.listenExternal()
	go l.listenInternal()
	return l, nil
}

func (l *stackListener) String() string {
	return fmt.Sprintf("stackListener concurrency: %d, queue size: %d, memory limit: %d, bytes per connection: %d, queue timeout: %s", l.maxConcurrency, l.maxQueueSize, l.memoryLimitBytes, l.connectionBytes, l.queueTimeout)
}

func (l *stackListener) Accept() (net.Conn, error) {
	select {
	case <-l.quit:
		return nil, errListenerClosed
	case c := <-l.acceptInternal:
		l.metrics.MeasureSince(acceptLatencyKey, c.accepted)
		d := time.Since(c.accepted)
		if d > l.queueTimeout {
			l.metrics.IncCounter(queueTimeoutKey)
			if c.Conn != nil {
				c.Conn.Close()
			}
			return nil, errAcceptTimeout
		}
		return c, nil
	}
}

func (l *stackListener) Addr() net.Addr {
	return l.externalListener.Addr()
}

func (l *stackListener) Close() error {
	l.once.Do(func() {
		close(l.quit)
		l.externalListener.Close()
		close(l.acceptInternal)
	})

	return nil
}

func (l *stackListener) listenExternal() {
	var (
		err error
		c   net.Conn
	)
	for {
		select {
		case <-l.quit:
			return
		default:
		}

		c, err = l.externalListener.Accept()
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				l.log.Infof("Server closed: %v", err)
				return
			}

			// client closed for example
			//l.log.Infof("Failed to accept connection (%T): %v", err, err)
			if c != nil {
				l.log.Info("close connection")
				c.Close()
			}
			continue
		}
		cc := external{c, time.Now()}
		l.stack.Push(&cc)
	}
}

func (l *stackListener) listenInternal() {
	for {
		select {
		case <-l.quit:
			return
		default:
		}
		cc := l.stack.Pop()
		if cc == nil {
			// reduce cpu usage caused by busywait
			time.Sleep(10 * time.Microsecond)
			continue
		}
		l.metrics.IncCounter(acceptedConnectionsKey)
		l.metrics.UpdateGauge(queuedConnectionsKey, float64(l.stack.top+1))

		l.acceptInternal <- *cc
	}
}
