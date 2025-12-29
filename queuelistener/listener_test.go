package queuelistener

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics/metricstest"
)

type testConnection struct {
	sync.Mutex
	closed bool
}

type testListener struct {
	sync.Mutex
	closed            bool
	failNextTemporary bool
	fail              bool
	connsBeforeFail   int
	addr              net.Addr
	conns             chan *testConnection
}

type testError struct{}

var errTemporary testError

func (err testError) Error() string   { return "test error" }
func (err testError) Timeout() bool   { return false }
func (err testError) Temporary() bool { return true }

func (c *testConnection) Read([]byte) (int, error)         { return 0, nil }
func (c *testConnection) Write([]byte) (int, error)        { return 0, nil }
func (c *testConnection) LocalAddr() net.Addr              { return nil }
func (c *testConnection) RemoteAddr() net.Addr             { return nil }
func (c *testConnection) SetDeadline(time.Time) error      { return nil }
func (c *testConnection) SetReadDeadline(time.Time) error  { return nil }
func (c *testConnection) SetWriteDeadline(time.Time) error { return nil }

func (c *testConnection) Close() error {
	c.Lock()
	defer c.Unlock()
	c.closed = true
	return nil
}

func (c *testConnection) isClosed() bool {
	c.Lock()
	defer c.Unlock()
	return c.closed
}

func (l *testListener) Accept() (net.Conn, error) {
	if l.failNextTemporary {
		l.failNextTemporary = false
		return nil, errTemporary
	}

	if l.fail {
		return nil, errors.New("listener error")
	}

	if l.connsBeforeFail > 0 {
		l.connsBeforeFail--
		if l.connsBeforeFail == 0 {
			l.fail = true
		}
	}

	c := &testConnection{}
	if cap(l.conns) > 0 {
		select {
		case l.conns <- c:
		default:
			// Drop one if cannot store the latest.
			// The test might have received a connection in the meantime so do not block.
			// Sending is safe as Accept is called from a single goroutine.
			select {
			case <-l.conns:
			default:
			}
			l.conns <- c
		}
	}

	return c, nil
}

func (l *testListener) Addr() net.Addr {
	if l.addr == nil {
		return &net.IPAddr{}
	}

	return l.addr
}

func (l *testListener) Close() error {
	l.Lock()
	defer l.Unlock()
	l.closed = true
	return nil
}

func (l *testListener) isClosed() bool {
	l.Lock()
	defer l.Unlock()
	return l.closed
}

func receive(rw io.ReadWriter, message string) error {
	m := make([]byte, len(message))
	b := m
	for len(b) > 0 {
		n, err := rw.Read(b)
		if err != nil {
			return err
		}

		b = b[n:]
	}

	if string(m) != message {
		return errors.New("corrupted message")
	}

	return nil
}

func ping(rw io.ReadWriter, message string) error {
	if _, err := rw.Write([]byte(message)); err != nil {
		return err
	}

	return receive(rw, message)
}

func pong(rw io.ReadWriter, message string) error {
	if err := receive(rw, message); err != nil {
		return err
	}

	_, err := rw.Write([]byte(message))
	return err
}

func waitForTO(f func() bool, timeout time.Duration) error {
	to := time.After(timeout)
	for {
		if f() {
			return nil
		}

		select {
		case <-to:
			return errors.New("timeout")
		case <-time.After(timeout / 20):
		}
	}
}

func waitFor(f func() bool) error {
	return waitForTO(f, 120*time.Millisecond)
}

func waitForGaugeFuncTO(m *metricstest.MockMetrics, key string, f func(float64) bool, timeout time.Duration) error {
	return waitForTO(func() bool {
		v, ok := m.Gauge(key)
		return ok && f(v)
	}, timeout)
}

func waitForGaugeFunc(m *metricstest.MockMetrics, key string, f func(float64) bool) error {
	return waitForGaugeFuncTO(m, key, f, 120*time.Millisecond)
}

func waitForGaugeTO(m *metricstest.MockMetrics, key string, value float64, timeout time.Duration) error {
	return waitForGaugeFuncTO(m, key, func(v float64) bool { return v == value }, timeout)
}

func waitForGauge(m *metricstest.MockMetrics, key string, value float64) error {
	return waitForGaugeTO(m, key, value, 120*time.Millisecond)
}

func closeAll(conns []net.Conn) {
	for _, c := range conns {
		c.Close()
	}
}

func acceptN(t *testing.T, l net.Listener, n int) []net.Conn {
	var (
		conns []net.Conn
		c     net.Conn
		err   error
	)

	for len(conns) < n {
		c, err = l.Accept()
		if err != nil {
			break
		}

		conns = append(conns, c)
	}

	if err != nil {
		closeAll(conns)
		t.Error(err)
		return nil
	}

	return conns
}

func acceptOne(t *testing.T, l net.Listener) net.Conn {
	conns := acceptN(t, l, 1)
	if len(conns) == 0 {
		return nil
	}

	return conns[0]
}

func dialN(t *testing.T, addr net.Addr, n int) []net.Conn {
	var (
		conns []net.Conn
		c     net.Conn
		err   error
	)

	for len(conns) < n {
		c, err = net.Dial("tcp", addr.String())
		if err != nil {
			break
		}

		conns = append(conns, c)
	}

	if err != nil {
		closeAll(conns)
		t.Error(err)
		return nil
	}

	return conns
}

func dialOne(t *testing.T, addr net.Addr) net.Conn {
	conns := dialN(t, addr, 1)
	if len(conns) == 0 {
		return nil
	}

	return conns[0]
}

func goAcceptN(t *testing.T, l net.Listener, n int) <-chan []net.Conn {
	accepted := make(chan []net.Conn)
	go func() { accepted <- acceptN(t, l, n) }()
	return accepted
}

func goDialN(t *testing.T, addr net.Addr, n int) <-chan []net.Conn {
	dialed := make(chan []net.Conn)
	go func() { dialed <- dialN(t, addr, n) }()
	return dialed
}

func acceptTimeout(t *testing.T, l net.Listener, timeout time.Duration) net.Conn {
	conn := make(chan net.Conn)
	go func() {
		c, err := l.Accept()
		if err != nil {
			t.Error(err)
		}

		conn <- c
	}()

	select {
	case c := <-conn:
		return c
	case <-time.After(timeout):
		t.Error("timeout while accepting connection")
		return nil
	}
}

func shouldAccept(t *testing.T, l net.Listener) net.Conn {
	return acceptTimeout(t, l, 120*time.Millisecond)
}

func TestInterface(t *testing.T) {
	t.Run("accepts functioning connections from the wrapped listener", func(t *testing.T) {
		const message = "ping"

		l, err := Listen(Options{Network: "tcp", Address: ":0"})
		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		addr := l.Addr()
		done := make(chan struct{})

		go func() {
			conn, err := net.Dial(addr.Network(), addr.String())
			if err != nil {
				close(done)
				t.Error(err)
			}

			defer conn.Close()
			if err := ping(conn, message); err != nil {
				close(done)
				t.Error(err)
			}

			close(done)
		}()

		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		pong(conn, message)
		<-done
	})

	t.Run("closing a connection closes the underlying connection", func(t *testing.T) {
		l, err := listenWith(&testListener{}, Options{})
		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}

		if !conn.(*connection).external.Conn.(*testConnection).isClosed() {
			t.Error("failed to close underlying connection")
		}
	})

	t.Run("wrapped listener returns temporary error, logs and retries", func(t *testing.T) {
		log := loggingtest.New()
		defer log.Close()

		l, err := listenWith(&testListener{failNextTemporary: true}, Options{
			Log: log,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		defer conn.Close()
		if err := log.WaitFor(errTemporary.Error(), 120*time.Millisecond); err != nil {
			t.Error("failed to log temporary error")
		}
	})

	t.Run("wrapped permanently fails, returns queued connections and the error", func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		l, err := listenWith(&testListener{connsBeforeFail: 3}, Options{
			MaxQueueSize: 3,
			Metrics:      m,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		if err := waitForGauge(m, queuedConnectionsKey, 3); err != nil {
			t.Fatalf("failed to reach expected queue size: %v", err)
		}

		conns := acceptN(t, l, 3)
		defer closeAll(conns)
		if _, err := l.Accept(); err == nil {
			t.Error("failed to receive wrapped listener error")
		}
	})

	t.Run("returns the external listener address", func(t *testing.T) {
		addr := &net.IPAddr{}
		l, err := listenWith(&testListener{addr: addr}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		if l.Addr() != addr {
			t.Error("failed to return the right address")
		}
	})
}

func TestQueue(t *testing.T) {
	t.Run("when max concurrency reached, queue is used", func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		l, err := listenWith(&testListener{}, Options{
			Metrics:        m,
			MaxConcurrency: 3,
			MaxQueueSize:   3,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()

		accepted := goAcceptN(t, l, 3)
		defer closeAll(<-accepted)

		if err := waitFor(func() bool {
			v, ok := m.Gauge(acceptedConnectionsKey)
			return ok && v >= 3
		}); err != nil {
			t.Fatal(err)
		}

		if err := waitFor(func() bool {
			v, ok := m.Gauge(queuedConnectionsKey)
			return ok && v >= 1
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("when max concurrency reached, accept blocks", func(t *testing.T) {
		l, err := Listen(Options{
			Network:        "tcp",
			Address:        ":0",
			MaxConcurrency: 3,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()

		accepted := goAcceptN(t, l, 3)
		dialed := goDialN(t, l.Addr(), 5)
		defer closeAll(<-accepted)
		defer closeAll(<-dialed)

		unblocked := make(chan struct{})
		go func() {
			if c, _ := l.Accept(); c != nil {
				c.Close()
			}

			close(unblocked)
		}()

		select {
		case <-unblocked:
			t.Error("failed to block listener after max concurrency reached")
		case <-time.After(120 * time.Millisecond):
		}
	})

	t.Run("closing an accepted connection allows unblocks accept", func(t *testing.T) {
		l, err := Listen(Options{
			Network:        "tcp",
			Address:        ":0",
			MaxConcurrency: 3,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()

		accepted := goAcceptN(t, l, 3)
		dialed := goDialN(t, l.Addr(), 5)
		defer closeAll(<-dialed)

		acceptedConns := <-accepted
		acceptedConns[0].Close()
		defer closeAll(acceptedConns[1:])

		if c := shouldAccept(t, l); c != nil {
			c.Close()
		}
	})

	t.Run("at max queue size, new connections purge the oldest item", func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		hook := make(chan struct{}, 1)
		l, err := Listen(Options{
			Network:             "tcp",
			Address:             ":0",
			MaxQueueSize:        3,
			Metrics:             m,
			testQueueChangeHook: hook,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()

		// dial to have a connection in the queue and save the dialer reference for later testing:
		conn1 := dialOne(t, l.Addr())
		if conn1 != nil {
			defer conn1.Close()
		}

		// fill up the queue:
		dialed := goDialN(t, l.Addr(), 2)
		defer closeAll(<-dialed)
		if err := waitForGauge(m, queuedConnectionsKey, 3); err != nil {
			t.Fatal(err)
		}

		// dial again to have the oldest queued connection purged:
		l.(*listener).clearQueueChangeHook()
		conn2 := dialOne(t, l.Addr())
		if conn2 != nil {
			defer conn2.Close()
		}

		// accept a new connection, should be paired with conn2
		<-hook
		aconn := acceptOne(t, l)
		if aconn != nil {
			defer aconn.Close()
		}

		// test the latest client connection that it works:
		done := make(chan struct{})
		go func() {
			if err := ping(conn2, "hello2"); err != nil {
				t.Error("connection doesn't work", err)
			}

			close(done)
		}()
		if err := pong(aconn, "hello2"); err != nil {
			t.Error("connection doesn't work", err)
		}

		// test that the purged connection doesn't work:
		if err := ping(conn1, "hello1"); err == nil {
			t.Error("connection should not work")
		}

		<-done
	})

	t.Run("when dropping or timeouting a connection, it is closed", func(t *testing.T) {
		t.Run("drop", func(t *testing.T) {
			tl := &testListener{conns: make(chan *testConnection, 1)}
			l, err := listenWith(tl, Options{
				Network:      "tcp",
				Address:      ":0",
				MaxQueueSize: 3,
			})
			if err != nil {
				t.Fatal(err)
			}

			defer l.Close()

			conn := <-tl.conns
			if err := waitFor(func() bool { return conn.isClosed() }); err != nil {
				t.Error("failed to close timeouted connection", err)
			}
		})

		t.Run("timeout", func(t *testing.T) {
			tl := &testListener{conns: make(chan *testConnection, 1)}
			l, err := listenWith(tl, Options{
				Network:      "tcp",
				Address:      ":0",
				QueueTimeout: 3 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}

			defer l.Close()

			conn := <-tl.conns
			to := time.After(120 * time.Millisecond)
			for !conn.isClosed() {
				select {
				case <-time.After(3 * time.Millisecond):
				case <-to:
					t.Error("failed to close timeouted connection")
					return
				}
			}
		})
	})
}

func TestOptions(t *testing.T) {
	t.Run("network and address work the same way as for net.Listen", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			addr    string
			network string
			want    net.Listener
			wantErr bool
		}{
			{
				name:    "no options return error",
				want:    nil,
				wantErr: true,
			},
			{
				name:    "no addr in options return dynamic port assigned by the kernel",
				network: "tcp",
				wantErr: false,
			},
			{
				name:    "no network in options return error",
				addr:    ":4001",
				wantErr: true,
			},
			{
				name:    "invalid addr in options return error",
				addr:    ":abc",
				network: "tcp",
				wantErr: true,
			},
			{
				name:    "invalid network in options return error",
				addr:    ":4001",
				network: "abc",
				wantErr: true,
			},
			{
				name:    "valid network and addr in options return listener",
				addr:    ":4001",
				network: "tcp",
				wantErr: false,
			}} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := Listen(Options{
					Network: tt.network,
					Address: tt.addr,
				})
				if got != nil {
					defer got.Close()
				}
				if (!tt.wantErr && got == nil) || (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
					t.Errorf("Failed to get listener: Want error %v, got %v, err %v", tt.wantErr, got, err)
				}
			})
		}
	})
	t.Run("max concurrency and max queue size has priority over memory limit and connection bytes", func(t *testing.T) {
		o := Options{
			Network:        "tcp",
			MaxConcurrency: 20,
			MaxQueueSize:   10,
		}
		got, err := Listen(o)
		if err != nil {
			t.Fatalf("Failed to get listener: %v", err)
		}
		defer got.Close()
		l := got.(*listener)

		if l.maxConcurrency != int64(o.MaxConcurrency) || l.maxQueueSize != int64(o.MaxQueueSize) {
			t.Errorf("Failed to overwrite calculated settings: %d != %d || %d != %d", l.maxConcurrency, o.MaxConcurrency, l.maxQueueSize, o.MaxQueueSize)
		}
	})
	t.Run("when max concurrency is not set, it is calculated from memory limit and connection bytes", func(t *testing.T) {
		o := Options{
			Network:          "tcp",
			MaxQueueSize:     10,
			MemoryLimitBytes: 10_000,
			ConnectionBytes:  4,
		}
		got, err := Listen(o)
		if err != nil {
			t.Fatalf("Failed to get listener: %v", err)
		}
		defer got.Close()
		l := got.(*listener)

		if o.MaxConcurrency != 0 && l.maxConcurrency == int64(o.MaxConcurrency) {
			t.Errorf("Failed to use calculate maxConcurrency settings: %d == %d", l.maxConcurrency, o.MaxConcurrency)
		}
		if l.maxConcurrency != o.MemoryLimitBytes/int64(o.ConnectionBytes) {
			t.Errorf("Calculated is not: %d != %d", l.maxConcurrency, o.MemoryLimitBytes/int64(o.ConnectionBytes))
		}
	})
	t.Run("when max queue size is not set, it is calculated from max concurrency", func(t *testing.T) {
		o := Options{
			Network:        "tcp",
			MaxConcurrency: 10,
		}
		got, err := Listen(o)
		if err != nil {
			t.Fatalf("Failed to get listener: %v", err)
		}
		defer got.Close()
		l := got.(*listener)

		if o.MaxQueueSize != 0 && l.maxQueueSize == int64(o.MaxQueueSize) {
			t.Errorf("Failed to use calculated maxQueueSize setting: %d == %d", l.maxConcurrency, o.MaxConcurrency)
		}
		if l.maxQueueSize == 0 || l.maxQueueSize > maxCalculatedQueueSize {
			t.Errorf("Calculated maxQueueSize not in bounds: %d", l.maxQueueSize)
		}
		if l.maxQueueSize != 10*int64(o.MaxConcurrency) {
			t.Errorf("Calculated maxQueueSize is wrong: %d != %d", l.maxQueueSize, 10*o.MaxConcurrency)
		}
	})
	t.Run("the calculated max queue size is limited to a constant", func(t *testing.T) {
		o := Options{
			Network:        "tcp",
			MaxConcurrency: maxCalculatedQueueSize + 1,
		}
		got, err := Listen(o)
		if err != nil {
			t.Fatalf("Failed to get listener: %v", err)
		}
		defer got.Close()
		l := got.(*listener)

		if l.maxQueueSize != maxCalculatedQueueSize {
			t.Errorf("Calculated maxQueueSize not in bounds: %d != %d", l.maxQueueSize, maxCalculatedQueueSize)
		}
	})
	t.Run("connections in the queue use the configured timeout", func(t *testing.T) {
		o := Options{
			Network:        "tcp",
			MaxConcurrency: 5,
			MaxQueueSize:   10,
			QueueTimeout:   500 * time.Millisecond,
		}
		got, err := Listen(o)
		if err != nil {
			t.Fatalf("Failed to get listener: %v", err)
		}
		defer got.Close()
		l := got.(*listener)

		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(o.QueueTimeout*2))
		defer cancel()
		dialer := net.Dialer{}
		conn, err := dialer.DialContext(ctx, "tcp", l.Addr().String())
		if err != nil {
			t.Fatalf("Failed to do DialContext err: %v", err)
		}
		defer conn.Close()
		if _, err := conn.Read([]byte("foo")); err != nil {
			return
		}
		t.Fatal("Failed to timeout while dialing")
	})
}

func TestTeardown(t *testing.T) {
	t.Run("queued connections are closed", func(t *testing.T) {
		tl := &testListener{
			conns:           make(chan *testConnection, 3),
			connsBeforeFail: 3,
		}

		m := &metricstest.MockMetrics{}
		l, err := listenWith(tl, Options{MaxQueueSize: 3, Metrics: m})
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForGauge(m, queuedConnectionsKey, 3); err != nil {
			l.Close()
			t.Fatal(err)
		}

		if err := l.Close(); err != nil {
			t.Fatal(err)
		}

		var conns []*testConnection
		for i := 0; i < 3; i++ {
			conns = append(conns, <-tl.conns)
		}

		to := time.After(120 * time.Millisecond)
		for {
			select {
			case <-to:
				t.Error("failed to close all connections")
				return
			default:
			}

			allClosed := true
			for _, c := range conns {
				if !c.isClosed() {
					allClosed = false
					break
				}
			}

			if allClosed {
				break
			}
		}
	})

	t.Run("accepted connections are not closed", func(t *testing.T) {
		tl := &testListener{
			conns:           make(chan *testConnection, 3),
			connsBeforeFail: 3,
		}

		m := &metricstest.MockMetrics{}
		l, err := listenWith(tl, Options{MaxQueueSize: 3, Metrics: m})
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForGauge(m, queuedConnectionsKey, 3); err != nil {
			l.Close()
			t.Fatal(err)
		}

		c0, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		if err := l.Close(); err != nil {
			t.Fatal(err)
		}

		var conns []*testConnection
		for i := 0; i < 3; i++ {
			conns = append(conns, <-tl.conns)
		}

		to := time.After(120 * time.Millisecond)
		for {
			select {
			case <-to:
				t.Error("failed to close all connections")
				return
			default:
			}

			allClosed := true
			for _, c := range conns[:2] {
				if !c.isClosed() {
					allClosed = false
					break
				}
			}

			if allClosed {
				break
			}
		}

		if c0.(*connection).external.Conn.(*testConnection).isClosed() {
			t.Error("the accepted connection was closed by the queue")
		}

		c0.Close()
	})

	t.Run("connections accepted from the wrapped listener closed after tear down", func(t *testing.T) {
		tl := &testListener{conns: make(chan *testConnection, 1)}
		m := &metricstest.MockMetrics{}
		l, err := listenWith(tl, Options{Metrics: m})
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForGaugeFunc(m, queuedConnectionsKey, func(v float64) bool { return v > 0 }); err != nil {
			l.Close()
			t.Fatal(err)
		}

		if err := l.Close(); err != nil {
			t.Fatal(err)
		}

		var conns []*testConnection
		for {
			var noConns bool
			select {
			case c := <-tl.conns:
				conns = append(conns, c)
			default:
				noConns = true
			}

			if noConns {
				break
			}
		}

		to := time.After(120 * time.Millisecond)
		for {
			allClosed := true
			for _, c := range conns {
				if !c.isClosed() {
					allClosed = false
					break
				}
			}

			if allClosed {
				break
			}

			select {
			case <-to:
				t.Fatal("failed to close all connections")
			default:
			}
		}
	})

	t.Run("calling accept after closed, returns an error", func(t *testing.T) {
		l, err := Listen(Options{Network: "tcp", Address: ":0"})
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			if _, err := l.Accept(); err == nil {
				t.Error("failed to return an error")
			}
		}()

		// no better way to make sure that the first Accept() blocks
		time.Sleep(30 * time.Millisecond)
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}

		if _, err := l.Accept(); err == nil {
			t.Error("failed to return an error")
		}
	})

	t.Run("the wrapped listener is closed", func(t *testing.T) {
		tl := &testListener{}
		l, err := listenWith(tl, Options{})
		if err != nil {
			t.Fatal(err)
		}

		if err := l.Close(); err != nil {
			t.Fatal(err)
		}

		if err := waitFor(func() bool { return tl.isClosed() }); err != nil {
			t.Error("failed to close connection", err)
		}
	})
}

func TestMonitoring(t *testing.T) {
	t.Run("logs the temporary errors", func(t *testing.T) {
		log := loggingtest.New()
		defer log.Close()

		l, err := listenWith(&testListener{failNextTemporary: true}, Options{
			Log: log,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		defer conn.Close()
		if err := log.WaitFor(errTemporary.Error(), 120*time.Millisecond); err != nil {
			t.Error("failed to log temporary error")
		}
	})

	t.Run("updates the gauges for the concurrency and the queue size, measures accept latency", func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		l, err := listenWith(&testListener{}, Options{
			Metrics:        m,
			MaxConcurrency: 3,
			MaxQueueSize:   3,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()

		accepted := goAcceptN(t, l, 3)
		defer closeAll(<-accepted)

		if err := waitFor(func() bool {
			v, ok := m.Gauge(acceptedConnectionsKey)
			return ok && v == 3
		}); err != nil {
			t.Fatal(err)
		}

		if err := waitFor(func() bool {
			v, ok := m.Gauge(queuedConnectionsKey)
			return ok && v == 3
		}); err != nil {
			t.Fatal(err)
		}

		m.WithMeasures(func(measures map[string][]time.Duration) {
			if len(measures[acceptLatencyKey]) != 3 {
				t.Error("latency measures mismatch")
			}
		})
	})

	t.Run("multiple calls to close are tolerated", func(t *testing.T) {
		l, err := Listen(Options{Network: "tcp", Address: ":0"})
		if err != nil {
			t.Fatal(err)
		}

		if err := l.Close(); err != nil {
			t.Error(err)
		}

		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})

	t.Run("multiple calls to close on the connections are tolerated", func(t *testing.T) {
		l, err := Listen(Options{Network: "tcp", Address: ":0"})
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		done := make(chan struct{})
		defer func() { close(done) }()
		go func() {
			c, err := net.Dial("tcp", l.Addr().String())
			if err != nil {
				t.Error(err)
				return
			}

			defer c.Close()
			<-done
		}()

		c, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}

		if err := c.Close(); err != nil {
			t.Error(err)
		}

		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
}

func TestListen(t *testing.T) {
	for _, tt := range []struct {
		name             string
		memoryLimit      int64
		bytesPerRequest  int
		network, address string
		wantErr          bool
	}{
		{
			name: "all defaults, network and address from the test",
		},
		{
			name:    "test wrong listener config network",
			network: "foo",
			wantErr: true,
		},
		{
			name:    "test wrong listener config address",
			address: ":foo",
			wantErr: true,
		},
		{
			name:    "test default limit",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			addr := ":1835"
			if tt.address != "" {
				addr = tt.address
			}
			network := "tcp"
			if tt.network != "" {
				network = tt.network
			}

			// got, err := Listen(tt.memoryLimit, tt.bytesPerRequest, network, addr)
			got, err := Listen(Options{
				Network:          network,
				Address:          addr,
				MemoryLimitBytes: tt.memoryLimit,
				ConnectionBytes:  tt.bytesPerRequest,
			})

			if (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Fatalf("Failed to Listen: WantErr %v, err %v", tt.wantErr, err)
			}
			if tt.wantErr {
				return
			}

			l, ok := got.(*listener)
			if !ok {
				t.Fatalf("Failed to Listen: !ok %v", !ok)
			}

			l.closedHook = make(chan struct{})
			defer func() {
				l.Close()
				<-l.closedHook
			}()

			msg := []byte("ping")
			go func() {
				conn, err2 := l.Accept()
				if err2 != nil {
					t.Errorf("Failed to accept: %v", err2)
				}
				conn.Write(msg)
				conn.Close()
			}()

			raddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1"+addr)
			if err != nil {
				t.Fatalf("Failed to resolve %s: %v", addr, err)
			}
			clconn, err := net.DialTCP("tcp", nil, raddr)
			if err != nil {
				t.Fatalf("Failed to dial: %v", err)
			}
			buf := make([]byte, len(msg))
			_, err = clconn.Read(buf)
			if err != nil || !reflect.DeepEqual(msg, buf) {
				t.Errorf("Failed to get msg %s, got %s, err: %v", string(msg), string(buf), err)
			}
		})
	}

}

func TestQueue1(t *testing.T) {
	for _, tt := range []struct {
		name            string
		memoryLimit     int64
		bytesPerRequest int
		allow           int
	}{
		{
			name:            "small limits",
			memoryLimit:     100,
			bytesPerRequest: 10,
			allow:           21, // 100/10 + (100/10)*10 = 110
		},
		/*
			{
				name:        "test falling back to defaults if memoryLimit is not set",
				allow:       defaultActiveMemoryLimitBytes / defaultActiveConnectionBytes +
					defaultInactiveMemoryLimitBytes / defaultInactiveConnectionBytes,
			},
			{
				name:            "test concurrency is ok",
				memoryLimit:     10,
				bytesPerRequest: 5,
				allow:           10/5 + 10*(10/5), // concurrency + queue size
			},
		*/
	} {
		t.Run(tt.name, func(t *testing.T) {
			addr := ":1838"
			network := "tcp"

			got, err := Listen(Options{
				Network:          network,
				Address:          addr,
				MemoryLimitBytes: tt.memoryLimit,
				ConnectionBytes:  tt.bytesPerRequest,
			})

			if err != nil {
				t.Fatalf("Failed to Listen: err %v", err)
			}

			l, ok := got.(*listener)
			if !ok {
				t.Fatalf("Failed to Listen: !ok %v", !ok)
			}

			quit := make(chan struct{})

			func() {
				defer l.Close()

				ping := []byte("ping")
				go func() {
					var cnt int
					buf := make([]byte, 4)
					for {
						select {
						case <-quit:
							l.Close()
							return
						default:
						}
						conn, err := l.Accept()
						if err != nil {
							continue
						}

						cnt++
						conn.Read(buf)
					}
				}()

				for i := 0; i < tt.allow; i++ {
					clconn, err := net.DialTimeout("tcp4", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						t.Fatalf("Failed to dial: %v", err)
					}

					defer clconn.Close()
					clconn.Write(ping)
				}
				t.Logf("did %d connections", tt.allow)
				time.Sleep(time.Second)
				for i := 0; i < 10*tt.allow; i++ {
					clconn, err := net.DialTimeout("tcp4", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						t.Fatalf("2Failed to dial: %v", err)
					}

					defer clconn.Close()
					clconn.Write(ping)
				}
			}()

			quit <- struct{}{}
		})
	}

}

func TestConnectionClose(t *testing.T) {
	t.Run("server", func(t *testing.T) {
		t.Run("measure close only once [bugfix]", func(t *testing.T) {
			m := &metricstest.MockMetrics{}
			l, err := listenWith(&testListener{}, Options{Metrics: m})
			if err != nil {
				t.Fatal(err)
			}

			defer l.Close()
			conn, err := l.Accept()
			if err != nil {
				t.Fatal(err)
			}

			if err := waitForGauge(m, acceptedConnectionsKey, 1); err != nil {
				t.Fatalf("failed to detect active connection: %v", err)
			}

			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}

			if err := waitForGauge(m, acceptedConnectionsKey, 0); err != nil {
				t.Fatalf("failed to detect closed connection: %v", err)
			}

			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}

			if err := waitForGauge(m, acceptedConnectionsKey, -1); err == nil {
				t.Fatal("the accepted connections count is below zero")
			}
		})
	})
}
