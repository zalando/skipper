package queuelistener

import (
	"net"
	"reflect"
	"testing"
	"time"
)

func TestQueuelistenerListen(t *testing.T) {
	for _, tt := range []struct {
		name             string
		memoryLimit      int
		bytesPerRequest  int
		network, address string
		wantErr          bool
	}{
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
			name:            "test default limit",
			memoryLimit:     -1,
			bytesPerRequest: 1,
			wantErr:         false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			addr := ":1835"
			if tt.address != "" {
				addr = tt.address
			}
			network := "tcp"
			if tt.network != "" {
				network = tt.network
			}

			got, err := Listen(tt.memoryLimit, tt.bytesPerRequest, network, addr)
			if (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Fatalf("Failed to Listen: WantErr %v, err %v", tt.wantErr, err)
			}
			if tt.wantErr {
				return
			}

			l, ok := got.(*listener)
			if !ok && !tt.wantErr {
				t.Fatalf("Failed to Listen: !WantErr %v, !ok %v", !tt.wantErr, !ok)
			}
			if ok && l.q == nil {
				t.Fatalf("Failed to Listen: l.q %v, ok %v", l.q, ok)
			}

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

func TestQueuelistener(t *testing.T) {
	for _, tt := range []struct {
		name            string
		memoryLimit     int
		bytesPerRequest int
		allow           int
	}{
		{
			name:        "test fallback to defaults if memoryLimit is not set",
			memoryLimit: -1,
			allow:       defaultMaxConcurrency,
		},
		{
			name:            "test fallback to defaults if memoryLimit is lower than bytesPerRequest",
			memoryLimit:     1,
			bytesPerRequest: 5,
			allow:           defaultMaxConcurrency},
		{
			name:            "test concurreny is ok",
			memoryLimit:     10,
			bytesPerRequest: 5,
			allow:           10/5 + 10*(10/5), // concurrency + queue size
		}} {
		t.Run(tt.name, func(t *testing.T) {
			addr := ":1838"
			network := "tcp"

			got, err := Listen(tt.memoryLimit, tt.bytesPerRequest, network, addr)
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
				go func() {
					for {
						select {
						case <-quit:
							l.Close()
							return
						default:
						}
						l.Accept()
					}
				}()

				for i := 0; i < tt.allow; i++ {
					clconn, err := net.DialTimeout("tcp", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						t.Fatalf("Failed to dial: %v", err)
					}
					defer clconn.Close()
				}
				t.Logf("did %d connections", tt.allow)
				time.Sleep(time.Second)
				println("dial should be enqueued")
				for i := 0; i < 10*tt.allow; i++ {
					println("connecting", i)
					clconn, err := net.DialTimeout("tcp", "127.0.0.1"+addr, 100*time.Second)
					if err != nil {
						println("connection error at", i)
						t.Fatalf("2Failed to dial: %v", err)
					}
					defer clconn.Close()
				}
			}()

			quit <- struct{}{}
		})
	}

}
