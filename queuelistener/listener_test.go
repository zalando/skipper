package queuelistener

import (
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics/metricstest"
)

type testConnection struct {
	closed bool
}

type testListener struct {
	closed            bool
	failNextTemporary bool
	fail              bool
	connsBeforeFail   int
	addr net.Addr
}

type testError struct{}

var errTemporary testError

func (err testError) Error() string   { return "test error" }
func (err testError) Timeout() bool   { return false }
func (err testError) Temporary() bool { return true }

func (c testConnection) Read([]byte) (int, error)         { return 0, nil }
func (c testConnection) Write([]byte) (int, error)        { return 0, nil }
func (c testConnection) LocalAddr() net.Addr              { return nil }
func (c testConnection) RemoteAddr() net.Addr             { return nil }
func (c testConnection) SetDeadline(time.Time) error      { return nil }
func (c testConnection) SetReadDeadline(time.Time) error  { return nil }
func (c testConnection) SetWriteDeadline(time.Time) error { return nil }

func (c *testConnection) Close() error {
	c.closed = true
	return nil
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

	return &testConnection{}, nil
}

func (l testListener) Addr() net.Addr {
	if l.addr == nil {
		return &net.IPAddr{}
	}

	return l.addr
}

func (l *testListener) Close() error {
	l.closed = true
	return nil
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

func waitForGaugeTO(m *metricstest.MockMetrics, key string, value float64, timeout time.Duration) error {
	to := time.After(timeout)
	for {
		v, ok := m.Gauge(key)
		if ok && v == value {
			return nil
		}

		select {
		case <-to:
			return errors.New("timeout")
		case <-time.After(timeout / 20):
		}
	}
}

func waitForGauge(m *metricstest.MockMetrics, key string, value float64) error {
	return waitForGaugeTO(m, key, value, 120*time.Millisecond)
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
				t.Fatal(err)
			}

			defer conn.Close()
			if err := ping(conn, message); err != nil {
				close(done)
				t.Fatal(err)
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

		if !conn.(connection).net.(*testConnection).closed {
			t.Error("failed to close underlying connection")
		}
	})

	t.Run("wrapped listener returns temporary error, logs and retries", func(t *testing.T) {
		log := loggingtest.New()
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
			Metrics: m,
		})

		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		if err := waitForGauge(m, queuedConnectionsKey, 3); err != nil {
			t.Fatalf("failed to reach expected queue size: %v", err)
		}

		for i := 0; i < 3; i++ {
			conn, err := l.Accept()
			if err != nil {
				t.Fatal(err)
			}

			defer conn.Close()
		}

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

		if l.Addr() != addr {
			t.Error("failed to return the right address")
		}
	})
}

// queue:
// - when max concurrency reached, Accept blocks
// - closing an accepted connection allows accepting the newest one from the queue
// - when max queue size reached, new incoming connections purge the oldest ones from the queue
// - when kicking or timeouting a connection from the queue, the external connection is closed
func TestQueue(t *testing.T) {
}

// options:
// - network and address work the same way as for net.Listen
// - max concurrency and max queue size has priority over memory limit and connection bytes
// - when max concurrency is not set, it is calculated from memory limit and connection bytes
// - when max queue size is not set, it is calculated from max concurrency
// - the calculated max queue size is limited to a constant
// - by default, connections in the queue don't timeout
// - connections in the queue use the configured timeout
func TestOptions(t *testing.T) {
}

// teardown:
// - queued connections are closed
// - connections accepted by the calling code are not closed by the listener
// - connections accepted from the wrapped listener after tear down are closed
// - calling accept after closed, returns an error
// - the wrapped listener is closed
func TestTeardown(t *testing.T) {
}

// monitoring:
// - logs the temporary errors
// - updates the gauges for the concurrency and the queue size
func TestMonitoring(t *testing.T) {
}

// concurrency:
// - multiple calls to close have no effect
// - multiple calls to close on the connections have no effect
func TestConcurrency(t *testing.T) {
}

func TestListen(t *testing.T) {
	for _, tt := range []struct {
		name             string
		memoryLimit      int
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
		memoryLimit     int
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
				name:        "test fallback to defaults if memoryLimit is not set",
				allow:       defaultActiveMemoryLimitBytes / defaultActiveConnectionBytes +
					defaultInactiveMemoryLimitBytes / defaultInactiveConnectionBytes,
			},
			{
				name:            "test concurreny is ok",
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
