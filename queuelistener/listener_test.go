package queuelistener

import (
	"net"
	"reflect"
	"testing"
	"time"
)

func TestQueueListenerListen(t *testing.T) {
	t.Skip()

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
				Network:                  network,
				Address:                  addr,
				ActiveMemoryLimitBytes:   tt.memoryLimit,
				ActiveConnectionBytes:    tt.bytesPerRequest,
				InactiveMemoryLimitBytes: tt.memoryLimit,
				InactiveConnectionBytes:  tt.bytesPerRequest / 10,
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

func TestQueueListener(t *testing.T) {
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
				Network:                  network,
				Address:                  addr,
				ActiveMemoryLimitBytes:   tt.memoryLimit,
				ActiveConnectionBytes:    tt.bytesPerRequest,
				InactiveMemoryLimitBytes: tt.memoryLimit,
				InactiveConnectionBytes:  tt.bytesPerRequest / 10,
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

						println("accept returned", cnt)
						cnt++
						_, err = conn.Read(buf)
						if err != nil {
							println("read filaed:", err.Error())
						} else {
							println("got:", string(buf), cnt)
						}
					}
				}()

				for i := 0; i < tt.allow; i++ {
					// println("connecting")
					clconn, err := net.DialTimeout("tcp4", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						// println("connection error")
						t.Fatalf("Failed to dial: %v", err)
					}
					println("client connected", i)
					defer clconn.Close()
					//go func() {
					if _, err := clconn.Write(ping); err != nil {
						println("write err:", err.Error())
					}
					//}()
				}
				t.Logf("did %d connections", tt.allow)
				time.Sleep(time.Second)
				println("dial should be enqueued")
				for i := 0; i < 10*tt.allow; i++ {
					// println("connecting to queue")
					clconn, err := net.DialTimeout("tcp4", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						// println("connection error at", i)
						t.Fatalf("2Failed to dial: %v", err)
					}
					println("client connected", i)
					defer clconn.Close()
					//go func() {
					if _, err := clconn.Write(ping); err != nil {
						println("write err2:", err.Error())
					}
					//}()
				}
			}()

			quit <- struct{}{}
		})
	}

}
