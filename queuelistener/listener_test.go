package queuelistener

import (
	"net"
	"reflect"
	"testing"
	"time"
)

// interface:
// - can accept connections from the wrapped listener
// - connection read/write works
// - closing connections closes the underlying connection
// - when wrapped listener returns temporary error, logs them and retries with a delay
// - when wrapped listener permanently fails returns the queued connections and fails afterwards, and it doesn't
// call the external listener anymore
// - returns the external listener address
func TestInterface(t *testing.T) {
	t.Run("accepts connections from the wrapped listener", func(t *testing.T) {
		l, err := Listen(Options{Network: "tcp", Address: ":0"})
		if err != nil {
			t.Fatal(err)
		}

		defer l.Close()
		go func() {
		}()
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
