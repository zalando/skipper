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

			waitShutdownCH := make(chan struct{})
			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
				close(waitShutdownCH)
			}()

			waitServeCH := make(chan struct{})
			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
				close(waitServeCH)
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

			<-waitShutdownCH
			<-waitServeCH
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
			name:     "test working example v6",
			host:     "good.example",
			version:  0x02,
			protocol: proxyproto.TCPv6,
			destAddr: "1.2.3.4",
			want:     http.StatusOK,
		},
		{
			name:     "test working example v4",
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
			protocol: proxyproto.TCPv4,
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

			waitShutdownCH := make(chan struct{})
			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
				close(waitShutdownCH)
			}()

			waitServeCH := make(chan struct{})
			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
				close(waitServeCH)
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

			<-waitShutdownCH
			<-waitServeCH
			t.Log("done")
		})
	}

}

func TestProxyListenerConfigErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		allowList []string
		denyList  []string
		skipList  []string
	}{
		{
			name:      "test failing allow list",
			allowList: []string{"ab", "0.0.0.0/0"},
		},
		{
			name:      "test failing allow list",
			allowList: []string{"::g/0", "0.0.0.0/0"},
		},
		{
			name:      "test failing allow list",
			allowList: []string{"::/0", "256.0.0.0/0"},
		},
		{
			name:      "test failing allow list",
			allowList: []string{"::/0", "0.0.0.0/33"},
		},
		{
			name:      "test failing allow list",
			allowList: []string{"::/0", "a"},
		},
		{
			name:     "test failing deny list",
			denyList: []string{"::/0", "256.0.0.0/0"},
		},
		{
			name:     "test failing skip list",
			skipList: []string{"::/0", "0.0.0.0/33"},
		}} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:       createTestListener(),
				AllowListCIDRs: tt.allowList,
				DenyListCIDRs:  tt.denyList,
				SkipListCIDRs:  tt.skipList,
			})
			if l != nil || err == nil {
				t.Fatal("Failed to get err")
			}
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

			waitShutdownCH := make(chan struct{})
			go func() {
				time.Sleep(time.Second)
				t.Log("Start shutdown")
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Logf("Failed to graceful shutdown: %v", err)
				}
				close(waitShutdownCH)
			}()

			waitServeCH := make(chan struct{})
			go func() {
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Serve failed: %v", err)
				}
				close(waitServeCH)
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

			<-waitShutdownCH
			<-waitServeCH
			t.Log("done")
		})
	}

}

func TestHasTLVSSL(t *testing.T) {
	tests := []struct {
		name     string
		tlvs     []proxyproto.TLV
		expected bool
	}{
		{
			name: "with SSL TLV and SSL flag set",
			tlvs: []proxyproto.TLV{
				{Type: 0x20, Value: []byte{0x01}}, // SSL TLV with SSL flag set
			},
			expected: true,
		},
		{
			name: "with SSL TLV but SSL flag not set",
			tlvs: []proxyproto.TLV{
				{Type: 0x20, Value: []byte{0x00}}, // PP2_TYPE_SSL without SSL flag (plain HTTP via PROXY)
			},
			expected: false,
		},
		{
			name: "without SSL TLV",
			tlvs: []proxyproto.TLV{
				{Type: 0x01, Value: []byte("test")}, // PP2_TYPE_ALPN
			},
			expected: false,
		},
		{
			name:     "empty TLVs",
			tlvs:     []proxyproto.TLV{},
			expected: false,
		},
		{
			name: "SSL TLV with empty value",
			tlvs: []proxyproto.TLV{
				{Type: 0x20, Value: []byte{}}, // PP2_TYPE_SSL with empty value
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasTLVSSL(tt.tlvs)
			if result != tt.expected {
				t.Fatalf("hasTLVSSL(%v) = %v, want %v", tt.tlvs, result, tt.expected)
			}
		})
	}
}

func TestProxyProtocolHTTPvsHTTPS(t *testing.T) {
	tests := []struct {
		name            string
		sslFlag         byte
		expectedSSLBool bool
		description     string
	}{
		{
			name:            "HTTP via PROXY (SSL flag 0x00)",
			sslFlag:         0x00,
			expectedSSLBool: false,
			description:     "Client sends HTTP via PROXY protocol - should detect as non-SSL",
		},
		{
			name:            "HTTPS via PROXY (SSL flag 0x01)",
			sslFlag:         0x01,
			expectedSSLBool: true,
			description:     "Client sends HTTPS via PROXY protocol - should detect as SSL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlvs := []proxyproto.TLV{
				{Type: 0x20, Value: []byte{tt.sslFlag}},
			}

			result := hasTLVSSL(tlvs)
			if result != tt.expectedSSLBool {
				t.Fatalf("%s: hasTLVSSL returned %v, expected %v", tt.description, result, tt.expectedSSLBool)
			}

			clientAddr := "192.0.2.1:54321"
			localAddr := "10.0.0.1:80"
			key := clientAddr + "|" + localAddr
			proxySSLCache.Store(key, tt.expectedSSLBool)
			defer proxySSLCache.Delete(key)

			ssl, ok := GetProxyProtoSSL(clientAddr, localAddr)
			if !ok {
				t.Fatalf("%s: GetProxyProtoSSL lookup failed", tt.description)
			}
			if ssl != tt.expectedSSLBool {
				t.Fatalf("%s: GetProxyProtoSSL returned %v, expected %v", tt.description, ssl, tt.expectedSSLBool)
			}
		})
	}
}

func TestProxyProtocolFullHTTPvsHTTPS(t *testing.T) {
	tests := []struct {
		name        string
		sslFlag     byte
		expectHTTP  bool
		description string
	}{
		{
			name:        "HTTP request via PROXY",
			sslFlag:     0x00,
			expectHTTP:  true,
			description: "External HTTP -> NLB -> PROXY:9998 with SSL flag 0x00 should be HTTP",
		},
		{
			name:        "HTTPS request via PROXY",
			sslFlag:     0x01,
			expectHTTP:  false,
			description: "External HTTPS -> NLB -> PROXY:9999 with SSL flag 0x01 should be HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:       createTestListener(),
				AllowListCIDRs: []string{"::/0", "0.0.0.0/0"},
			})
			if err != nil {
				t.Fatalf("%s: Failed to create proxy listener: %v", tt.description, err)
			}

			addr := l.Addr().String()
			serverString := "response"
			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var localAddr string
					if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
						localAddr = addr.String()
					}

					ssl, ok := GetProxyProtoSSL(r.RemoteAddr, localAddr)

					if ok && ssl {
						w.WriteHeader(http.StatusOK)
					} else {
						w.WriteHeader(http.StatusPermanentRedirect)
					}
					w.Write([]byte(serverString))
				}),
			}

			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						dialer := &net.Dialer{Timeout: 5 * time.Second}
						conn, err := dialer.Dial(network, addr)
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
								IP:   net.ParseIP("10.0.0.5"),
								Port: 8080,
							},
						}

						err = header.SetTLVs([]proxyproto.TLV{
							{Type: 0x20, Value: []byte{tt.sslFlag}},
						})
						if err != nil {
							conn.Close()
							return nil, err
						}

						if _, err := header.WriteTo(conn); err != nil {
							conn.Close()
							return nil, err
						}
						return conn, nil
					},
				},
			}

			waitShutdownCH := make(chan struct{})
			go func() {
				time.Sleep(time.Second)
				srv.Shutdown(context.Background())
				close(waitShutdownCH)
			}()

			waitServeCH := make(chan struct{})
			go func() {
				srv.Serve(l)
				close(waitServeCH)
			}()

			req, err := http.NewRequest("GET", "http://"+addr+"/test", nil)
			if err != nil {
				t.Fatalf("%s: Failed to create request: %v", tt.description, err)
			}

			rsp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s: Failed to get response: %v", tt.description, err)
			}
			defer rsp.Body.Close()

			expectedCode := http.StatusOK
			if tt.expectHTTP {
				expectedCode = http.StatusPermanentRedirect
			}

			if rsp.StatusCode != expectedCode {
				t.Fatalf("%s: Got status %d, expected %d", tt.description, rsp.StatusCode, expectedCode)
			}

			<-waitShutdownCH
			<-waitServeCH
		})
	}
}

func TestProxyProtocolWithDeadlines(t *testing.T) {
	tests := []struct {
		name          string
		readTimeout   time.Duration
		writeTimeout  time.Duration
		shouldSucceed bool
		description   string
	}{
		{
			name:          "read deadline not exceeded",
			readTimeout:   2 * time.Second,
			writeTimeout:  2 * time.Second,
			shouldSucceed: true,
			description:   "Normal read/write operations within deadline should succeed",
		},
		{
			name:          "read deadline exceeded",
			readTimeout:   10 * time.Millisecond,
			writeTimeout:  2 * time.Second,
			shouldSucceed: false,
			description:   "Read operation with very short timeout should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewListener(Options{
				Listener:       createTestListener(),
				AllowListCIDRs: []string{"::/0", "0.0.0.0/0"},
			})
			if err != nil {
				t.Fatalf("%s: Failed to create proxy listener: %v", tt.description, err)
			}

			addr := l.Addr().String()
			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("ok"))
				}),
			}

			client := &http.Client{
				Timeout: 3 * time.Second,
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						dialer := &net.Dialer{Timeout: 5 * time.Second}
						conn, err := dialer.Dial(network, addr)
						if err != nil {
							return nil, err
						}

						conn.SetReadDeadline(time.Now().Add(tt.readTimeout))
						conn.SetWriteDeadline(time.Now().Add(tt.writeTimeout))

						header := &proxyproto.Header{
							Version:           2,
							Command:           proxyproto.PROXY,
							TransportProtocol: proxyproto.TCPv4,
							SourceAddr: &net.TCPAddr{
								IP:   net.ParseIP(clientIP),
								Port: clientPort,
							},
							DestinationAddr: &net.TCPAddr{
								IP:   net.ParseIP("10.0.0.5"),
								Port: 8080,
							},
						}

						if _, err := header.WriteTo(conn); err != nil && !tt.shouldSucceed {
							return nil, nil
						}
						if err != nil {
							return nil, err
						}
						return conn, nil
					},
				},
			}

			waitShutdownCH := make(chan struct{})
			go func() {
				time.Sleep(time.Second)
				srv.Shutdown(context.Background())
				close(waitShutdownCH)
			}()

			waitServeCH := make(chan struct{})
			go func() {
				srv.Serve(l)
				close(waitServeCH)
			}()

			req, err := http.NewRequest("GET", "http://"+addr+"/test", nil)
			if err != nil {
				t.Fatalf("%s: Failed to create request: %v", tt.description, err)
			}

			rsp, err := client.Do(req)
			if !tt.shouldSucceed && (err != nil || rsp == nil) {
				<-waitShutdownCH
				<-waitServeCH
				return
			}

			if tt.shouldSucceed {
				if err != nil {
					t.Fatalf("%s: Request should succeed but got error: %v", tt.description, err)
				}
				defer rsp.Body.Close()
				if rsp.StatusCode != http.StatusOK {
					t.Fatalf("%s: Expected 200, got %d", tt.description, rsp.StatusCode)
				}
			}

			<-waitShutdownCH
			<-waitServeCH
		})
	}
}

func TestTLVCacheCleanupConnDeadlines(t *testing.T) {
	conn := &tlvCacheCleanupConn{
		conn: &mockConn{},
	}

	deadline := time.Now().Add(time.Second)
	err := conn.SetDeadline(deadline)
	if err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	err = conn.SetReadDeadline(deadline)
	if err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	err = conn.SetWriteDeadline(deadline)
	if err != nil {
		t.Fatalf("SetWriteDeadline failed: %v", err)
	}
}

type mockConn struct {
	readDeadline  time.Time
	writeDeadline time.Time
	deadline      time.Time
}

func (m *mockConn) Read(b []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(b []byte) (int, error)        { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error      { m.deadline = t; return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { m.readDeadline = t; return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { m.writeDeadline = t; return nil }
