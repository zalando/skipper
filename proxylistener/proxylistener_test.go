package proxylistener

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
)

// type testListener struct {
// 	addr net.Addr
// }

// func (l *testListener) Accept() (net.Conn, error) {

// }

// func (l *testListener) Addr() net.Addr {
// 	if l.addr == nil {
// 		return &net.IPAddr{}
// 	}

// 	return l.addr
// }
// func (l *testListener) Close() error { return nil }

func createTestListener() net.Listener {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	return l
}

var (
	clientIP   = "1.2.3.4"
	clientPort = 12345
)

func createProxyClient(proxyAddr, destAddr string, destPort int) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := dialer.Dial(network, proxyAddr)
				if err != nil {
					return nil, err
				}

				header := &proxyproto.Header{
					Version:           2,
					Command:           proxyproto.PROXY,
					TransportProtocol: proxyproto.TCPv4,
					SourceAddr: &net.TCPAddr{
						IP:   net.ParseIP(clientIP),
						Port: clientPort,
					},
					DestinationAddr: &net.TCPAddr{
						IP:   net.ParseIP(destAddr),
						Port: destPort,
					},
				}

				if _, err := header.WriteTo(conn); err != nil {
					conn.Close()
					return nil, err
				}

				return conn, nil
			},
		},
	}
}

func createBogusProxyClient(proxyAddr, destAddr string, destPort int, version byte, protocol proxyproto.AddressFamilyAndProtocol) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := dialer.Dial(network, proxyAddr)
				if err != nil {
					return nil, err
				}

				header := &proxyproto.Header{
					Version:           version,
					Command:           proxyproto.PROXY,
					TransportProtocol: protocol,
					SourceAddr: &net.TCPAddr{
						IP:   net.ParseIP(clientIP),
						Port: clientPort,
					},
					DestinationAddr: &net.TCPAddr{
						IP:   net.ParseIP(destAddr),
						Port: destPort,
					},
				}
				if _, err := header.WriteTo(conn); err != nil {
					conn.Close()
					return nil, err
				}

				return conn, nil
			},
		},
	}

	return cli
}

func TestProxyListenerWithProxyClient(t *testing.T) {
	for _, tt := range []struct {
		name           string
		host           string
		timeout        time.Duration
		sleep          time.Duration
		readBufferSize int
		allowList      []string
		denyList       []string
		skipList       []string
		want           int
		wantErr        bool
	}{
		{
			name:      "test allow list",
			host:      "allow.example",
			allowList: []string{"::/0", "0.0.0.0/0"},
			want:      http.StatusOK,
		},
		{
			name:     "test deny list",
			host:     "deny.example",
			denyList: []string{"::/0", "0.0.0.0/0"},
			wantErr:  true,
		},
		{
			name:     "test skip list",
			host:     "skip.example",
			skipList: []string{"::/0", "0.0.0.0/0"},
			want:     http.StatusBadRequest,
		},
		{
			name:    "test default no list",
			host:    "default.example",
			wantErr: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:          createTestListener(),
				ReadHeaderTimeout: tt.timeout,
				ReadBufferSize:    tt.readBufferSize,
				AllowListCIDRs:    tt.allowList,
				DenyListCIDRs:     tt.denyList,
				SkipListCIDRs:     tt.skipList,
			})
			if err != nil {
				t.Fatalf("Failed to create proxy listener: %v", err)
			}

			clientString := `hello from client`
			serverString := `hello from server`
			addr := l.Addr().String()
			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Logf("server RemoteAddr: %q", r.RemoteAddr)
					expectedClientAddr := fmt.Sprintf("%s:%d", clientIP, clientPort)
					if r.RemoteAddr != expectedClientAddr {
						t.Fatalf("Failed to get the expected client %q, got %q", expectedClientAddr, r.RemoteAddr)
					}
					if r.Host != tt.host {
						t.Fatalf("Failed to get the expected host %q, got: %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							t.Fatalf("Failed to read body in server: %v", err)
						}
						if s := string(buf); s != clientString {
							t.Fatalf("Failed to get %q, got: %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			client := createProxyClient(addr, "10.0.0.5", 8080)

			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
			}()

			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
			}()

			buf := bytes.NewBufferString(clientString)
			req, err := http.NewRequest("POST", "http://"+addr+"/foo", buf)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Host = tt.host
			rsp, err := client.Do(req)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			t.Log("done")
		})
	}

}

func TestProxyListenerWithBogusProxyClient(t *testing.T) {
	for _, tt := range []struct {
		name     string
		host     string
		version  byte
		protocol proxyproto.AddressFamilyAndProtocol
		destAddr string
		want     int
		wantErr  bool
	}{
		{
			name:     "test working example",
			host:     "good.example",
			version:  0x02,
			protocol: proxyproto.TCPv4,
			destAddr: "1.2.3.4",
			want:     http.StatusOK,
		},
		{
			name:     "test bogus version",
			host:     "bogus.example",
			version:  0x05,
			protocol: proxyproto.TCPv4,
			destAddr: "1.2.3.4",
			wantErr:  true,
		},
		{
			name:     "test bogus protocol",
			host:     "bogus.example",
			version:  0x02,
			protocol: proxyproto.UnixDatagram,
			destAddr: "1.2.3.4",
			wantErr:  true,
		},
		{
			name:     "test bogus header",
			host:     "bogus.example",
			version:  0x02,
			protocol: proxyproto.TCPv6,
			destAddr: "4",
			wantErr:  true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:       createTestListener(),
				AllowListCIDRs: []string{"::/0", "0.0.0.0/0"},
			})
			if err != nil {
				t.Fatalf("Failed to create proxy listener: %v", err)
			}

			clientString := `hello from client`
			serverString := `hello from server`
			addr := l.Addr().String()
			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					expectedClientAddr := fmt.Sprintf("%s:%d", clientIP, clientPort)
					if r.RemoteAddr != expectedClientAddr {
						t.Fatalf("Failed to get the expected client %q, got %q", expectedClientAddr, r.RemoteAddr)
					}
					if r.Host != tt.host {
						t.Fatalf("Failed to get the expected host %q, got: %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							t.Fatalf("Failed to read body in server: %v", err)
						}
						if s := string(buf); s != clientString {
							t.Fatalf("Failed to get %q, got: %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			client := createBogusProxyClient(addr, tt.destAddr, 8080, tt.version, tt.protocol)

			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
			}()

			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
			}()

			buf := bytes.NewBufferString(clientString)
			req, err := http.NewRequest("POST", "http://"+addr+"/foo", buf)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Host = tt.host
			rsp, err := client.Do(req)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			t.Log("done")
		})
	}

}

func TestProxyListenerWithHttpClient(t *testing.T) {
	for _, tt := range []struct {
		name           string
		host           string
		timeout        time.Duration
		sleep          time.Duration
		readBufferSize int
		allowList      []string
		denyList       []string
		skipList       []string
		want           int
		wantErr        bool
	}{
		{
			name:      "test allow list",
			host:      "allow.example",
			allowList: []string{"::/0", "0.0.0.0/0"},
			want:      http.StatusOK,
		},
		{
			name:     "test deny list",
			host:     "deny.example",
			denyList: []string{"::/0", "0.0.0.0/0"},
			want:     http.StatusOK,
		},
		{
			name:     "test skip list",
			host:     "skip.example",
			skipList: []string{"::/0", "0.0.0.0/0"},
			want:     http.StatusOK,
		},
		{
			name: "test default no list",
			host: "default.example",
			want: http.StatusOK,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:          createTestListener(),
				ReadHeaderTimeout: tt.timeout,
				ReadBufferSize:    tt.readBufferSize,
				AllowListCIDRs:    tt.allowList,
				DenyListCIDRs:     tt.denyList,
				SkipListCIDRs:     tt.skipList,
			})
			if err != nil {
				t.Fatalf("Failed to create proxy listener: %v", err)
			}

			clientString := `hello from client`
			serverString := `hello from server`
			addr := l.Addr().String()
			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Host != tt.host {
						t.Fatalf("Failed to get the expected host %q, got: %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							t.Fatalf("Failed to read body in server: %v", err)
						}
						if s := string(buf); s != clientString {
							t.Fatalf("Failed to get %q, got: %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			client := http.DefaultClient

			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
			}()

			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
			}()

			buf := bytes.NewBufferString(clientString)
			req, err := http.NewRequest("POST", "http://"+addr+"/foo", buf)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Host = tt.host
			rsp, err := client.Do(req)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			t.Log("done")
		})
	}

}
