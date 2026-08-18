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

	proxyproto "github.com/pires/go-proxyproto"
)

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
			DisableKeepAlives: true, // each request uses its own connection; prevents idle-connection races
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
			DisableKeepAlives: true, // each request uses its own connection; prevents idle-connection races
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

			// handlerErr collects assertion failures from the HTTP handler goroutine.
			// t.Fatalf inside a non-test goroutine is a no-op (only exits that goroutine),
			// so we send errors through a channel and check them in the test goroutine.
			handlerErr := make(chan error, 8)

			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Logf("server RemoteAddr: %q", r.RemoteAddr)
					expectedClientAddr := fmt.Sprintf("%s:%d", clientIP, clientPort)
					if r.RemoteAddr != expectedClientAddr {
						handlerErr <- fmt.Errorf("unexpected RemoteAddr: want %q got %q", expectedClientAddr, r.RemoteAddr)
					}
					if r.Host != tt.host {
						handlerErr <- fmt.Errorf("unexpected Host: want %q got %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							handlerErr <- fmt.Errorf("failed to read body in server: %v", err)
						} else if s := string(buf); s != clientString {
							handlerErr <- fmt.Errorf("unexpected body: want %q got %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			client := createProxyClient(addr, "10.0.0.5", 8080)

			// shutdownCH is closed after client.Do returns so that srv.Shutdown
			// is only called once the request has completed, not on a fixed timer.
			shutdownCH := make(chan struct{})
			waitShutdownCH := make(chan struct{})
			go func() {
				<-shutdownCH
				t.Log("Start shutdown")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
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
			if rsp != nil {
				rsp.Body.Close() // drain body before triggering shutdown
			}
			close(shutdownCH) // trigger shutdown now that response body is consumed
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			<-waitShutdownCH
			<-waitServeCH

			close(handlerErr)
			for e := range handlerErr {
				t.Error(e)
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

			handlerErr := make(chan error, 8)

			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					expectedClientAddr := fmt.Sprintf("%s:%d", clientIP, clientPort)
					if r.RemoteAddr != expectedClientAddr {
						handlerErr <- fmt.Errorf("unexpected RemoteAddr: want %q got %q", expectedClientAddr, r.RemoteAddr)
					}
					if r.Host != tt.host {
						handlerErr <- fmt.Errorf("unexpected Host: want %q got %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							handlerErr <- fmt.Errorf("failed to read body in server: %v", err)
						} else if s := string(buf); s != clientString {
							handlerErr <- fmt.Errorf("unexpected body: want %q got %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			client := createBogusProxyClient(addr, tt.destAddr, 8080, tt.version, tt.protocol)

			shutdownCH := make(chan struct{})
			waitShutdownCH := make(chan struct{})
			go func() {
				<-shutdownCH
				t.Log("Start shutdown")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
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
			if rsp != nil {
				rsp.Body.Close()
			}
			close(shutdownCH)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			<-waitShutdownCH
			<-waitServeCH

			close(handlerErr)
			for e := range handlerErr {
				t.Error(e)
			}
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
			base := createTestListener()
			l, err := NewListener(Options{
				Listener:       base,
				AllowListCIDRs: tt.allowList,
				DenyListCIDRs:  tt.denyList,
				SkipListCIDRs:  tt.skipList,
			})
			base.Close()
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

			handlerErr := make(chan error, 8)

			srv := &http.Server{
				Addr: addr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Host != tt.host {
						handlerErr <- fmt.Errorf("unexpected Host: want %q got %q", tt.host, r.Host)
					}
					if r.Method == "POST" {
						buf, err := io.ReadAll(r.Body)
						if err != nil {
							handlerErr <- fmt.Errorf("failed to read body in server: %v", err)
						} else if s := string(buf); s != clientString {
							handlerErr <- fmt.Errorf("unexpected body: want %q got %q", clientString, s)
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(serverString))
				}),
			}

			// Use a per-subtest client with keep-alives disabled to prevent
			// idle connections from one subtest from contaminating the next.
			client := &http.Client{
				Transport: &http.Transport{DisableKeepAlives: true},
			}

			shutdownCH := make(chan struct{})
			waitShutdownCH := make(chan struct{})
			go func() {
				<-shutdownCH
				t.Log("Start shutdown")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
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
			if rsp != nil {
				rsp.Body.Close()
			}
			close(shutdownCH)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to get response: %v", err)
			}
			if !tt.wantErr && rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}

			<-waitShutdownCH
			<-waitServeCH

			close(handlerErr)
			for e := range handlerErr {
				t.Error(e)
			}
			t.Log("done")
		})
	}

}
