package queuelistener

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
)

func TestStackListener(t *testing.T) {
	for _, tt := range []struct {
		name string
		opt  Options
		want error
	}{
		{
			name: "wrong listener options",
			want: fmt.Errorf("StackListener failed net.Listen:"),
		},
		{
			name: "listener with metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     time.Second,
				Metrics:          metrics.Default,
			},
			want: nil,
		},
		{
			name: "listener without metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     time.Second,
				Metrics:          nil,
			},
			want: nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &http.Server{
				Addr: tt.opt.Address,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
					w.Write([]byte("OK"))
				}),
				ReadTimeout:       time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      time.Second,
				IdleTimeout:       time.Second,
				MaxHeaderBytes:    1000,
			}
			defer srv.Close()

			cm := &skpnet.ConnManager{
				Keepalive:         0,
				KeepaliveRequests: 0,
				Metrics:           tt.opt.Metrics,
			}
			cm.Configure(srv)

			l, err := StackListener(tt.opt)
			if err != nil {
				if tt.want == nil {
					t.Fatalf("Failed to create StackListener: %v", err)
				} else {
					// have err and want error
					return
				}
			}
			defer l.Close()
			if tt.want != nil && err == nil {
				t.Fatalf("Failed to get error from StackListener, want: %v", tt.want)
			}

			go func() {
				t.Logf("start server")
				if err := srv.Serve(l); err != http.ErrServerClosed {
					log.Errorf("Serve failed: %v", err)
				}
				return
			}()

			dst := "http://" + l.Addr().String()
			var rsp *http.Response
			for range 3 {
				rsp, err = http.DefaultClient.Get(dst)
				if err != nil {
					time.Sleep(time.Second)
					continue
				}
				break
			}
			if err != nil {
				t.Fatalf("Failed to do a GET request to %s", dst)
			}
			if rsp.StatusCode != http.StatusAccepted {
				t.Fatalf("Failed to get response status code we expect, got: %d", rsp.StatusCode)
			}
			defer rsp.Body.Close()
			io.Copy(io.Discard, rsp.Body)
			http.DefaultClient.CloseIdleConnections()
		})
	}
}

func TestStackListenerRequestFlow(t *testing.T) {
	opt := Options{
		Network:          "tcp",
		Address:          ":9090",
		MaxConcurrency:   10000,
		MaxQueueSize:     10000,
		MemoryLimitBytes: 1000,
		ConnectionBytes:  100,
		QueueTimeout:     time.Second,
		Metrics:          &metricstest.MockMetrics{},
	}

	srv := &http.Server{
		Addr: opt.Address,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("OK"))
		}),
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    1000,
	}
	defer srv.Close()

	cm := &skpnet.ConnManager{
		Keepalive:         0,
		KeepaliveRequests: 0,
		Metrics:           opt.Metrics,
	}
	cm.Configure(srv)

	l, err := StackListener(opt)
	if err != nil {
		t.Fatalf("Failed to create StackListener: %v", err)
	}
	defer l.Close()

	go func() {
		t.Logf("start server")
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Errorf("Serve failed: %v", err)
		}
		return
	}()

	dst := "http://" + l.Addr().String()
	var rsp *http.Response
	for range 3 {
		rsp, err = http.DefaultClient.Get(dst)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		t.Fatalf("Failed to do a GET request to %s", dst)
	}
	if rsp.StatusCode != http.StatusAccepted {
		t.Fatalf("Failed to get response status code we expect, got: %d", rsp.StatusCode)
	}
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)

	for range 10 {
		rsp, err = http.DefaultClient.Get(dst)
		if err != nil {
			t.Fatalf("Failed to do a GET request to %s", dst)
		}
		if rsp.StatusCode != http.StatusAccepted {
			t.Fatalf("Failed to get response status code we expect, got: %d", rsp.StatusCode)
		}
		t.Logf("Response: %d", rsp.StatusCode)
		io.Copy(io.Discard, rsp.Body)
		rsp.Body.Close()
	}

	t.Logf("opt.Metrics: %+v", opt.Metrics)
}

func TestStackListenerTimeout(t *testing.T) {
	for _, tt := range []struct {
		name string
		opt  Options
		want error
	}{
		{
			name: "listener timeout with metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     time.Microsecond,
				Metrics:          &metricstest.MockMetrics{},
			},
			want: nil,
		},
		{
			name: "listener timeout without metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     time.Microsecond,
				Metrics:          nil,
			},
			want: nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &http.Server{
				Addr: tt.opt.Address,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
					w.Write([]byte("OK"))
				}),
				ReadTimeout:       time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      time.Second,
				IdleTimeout:       time.Second,
				MaxHeaderBytes:    1000,
			}
			defer srv.Close()

			cm := &skpnet.ConnManager{
				Keepalive:         0,
				KeepaliveRequests: 0,
				Metrics:           tt.opt.Metrics,
			}
			cm.Configure(srv)

			l, err := StackListener(tt.opt)
			if err != nil {
				if tt.want == nil {
					t.Fatalf("Failed to create StackListener: %v", err)
				} else {
					// have err and want error
					return
				}
			}
			defer l.Close()

			go func() {
				t.Logf("start server")
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Server closed: %v", err)
				}
				return
			}()

			dst := "http://" + l.Addr().String()

			_, err = http.DefaultClient.Get(dst)
			if err == nil {
				t.Fatal("Failed to create a fail")
			}

			if tt.opt.Metrics != nil {
				if m, ok := tt.opt.Metrics.(*metricstest.MockMetrics); ok {
					m.WithCounters(func(c map[string]int64) {
						t.Logf("counters: %v", c)

						assert.Equal(t, int64(1), c["listener.queued.timeouts"])
						assert.Equal(t, int64(1), c["listener.accepted.connections"])
					})
				}
			}

			http.DefaultClient.CloseIdleConnections()

		})
	}
}

func TestStackListenerShutdown(t *testing.T) {
	for _, tt := range []struct {
		name        string
		opt         Options
		backendTime time.Duration
		want        error
	}{
		{
			name: "listener timeout with metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     100 * time.Millisecond,
				Metrics:          &metricstest.MockMetrics{},
			},
			backendTime: time.Second,
			want:        nil,
		},
		{
			name: "listener timeout without metrics",
			opt: Options{
				Network:          "tcp",
				Address:          ":9090",
				MaxConcurrency:   10000,
				MaxQueueSize:     10000,
				MemoryLimitBytes: 1000,
				ConnectionBytes:  100,
				QueueTimeout:     100 * time.Millisecond,
				Metrics:          nil,
			},
			backendTime: time.Second,
			want:        nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &http.Server{
				Addr: tt.opt.Address,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

					// backend hang
					time.Sleep(tt.backendTime)

					w.WriteHeader(http.StatusAccepted)
					w.Write([]byte("OK"))
				}),
				ReadTimeout:       time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      time.Second,
				IdleTimeout:       time.Second,
				MaxHeaderBytes:    1000,
			}
			defer srv.Close()

			cm := &skpnet.ConnManager{
				Keepalive:         0,
				KeepaliveRequests: 0,
				Metrics:           tt.opt.Metrics,
			}
			cm.Configure(srv)

			l, err := StackListener(tt.opt)
			if err != nil {
				if tt.want == nil {
					t.Fatalf("Failed to create StackListener: %v", err)
				} else {
					// have err and want error
					return
				}
			}
			defer l.Close()

			go func() {
				t.Logf("start server")
				if err := srv.Serve(l); err != http.ErrServerClosed {
					t.Logf("Server closed: %v", err)
				}
				return
			}()

			dst := "http://" + l.Addr().String()
			errCH := make(chan error)
			go func(ch chan error) {
				_, err = http.DefaultClient.Get(dst)
				ch <- err
			}(errCH)

			time.Sleep(500 * time.Millisecond)
			// server init close
			err = l.Close()
			t.Logf("Close: %v", err)

			err = <-errCH
			t.Logf("client err: %v", err)
			// if !strings.Contains(err.Error(), "connection refused") {
			// 	t.Errorf("Failed to get client err: %v", err)
			// }

			if tt.opt.Metrics != nil {
				if m, ok := tt.opt.Metrics.(*metricstest.MockMetrics); ok {
					m.WithCounters(func(c map[string]int64) {
						t.Logf("counters: %v", c)

						// assert.Equal(t, int64(1), c["listener.queued.timeouts"])
						// assert.Equal(t, int64(1), c["listener.accepted.connections"])
					})
				}
			}

			http.DefaultClient.CloseIdleConnections()

		})
	}
}

func TestTCPListenerStackListener(t *testing.T) {
	addr := ":9090"
	l, err := StackListener(Options{
		Network:          "tcp",
		Address:          addr,
		MaxConcurrency:   10000,
		MaxQueueSize:     10000,
		MemoryLimitBytes: 1000,
		ConnectionBytes:  100,
		QueueTimeout:     time.Second,
		Metrics:          metrics.Default,
		//Metrics:          metrics.NoMetric{},
	})
	if err != nil {
		t.Fatalf("Failed to create QueueListener: %v", err)
	}
	defer l.Close()

	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("OK"))
		}),
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    1000,
	}
	defer srv.Close()

	cm := &skpnet.ConnManager{
		Keepalive:         10 * time.Second,
		KeepaliveRequests: 10,
		Metrics:           metrics.Default,
	}
	cm.Configure(srv)

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Errorf("Serve failed: %v", err)
		}
	}()

	// check ready
	err = fmt.Errorf("an error")
	var rsp *http.Response
	for err != nil {
		rsp, err = http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		time.Sleep(time.Millisecond)
	}
	io.Copy(io.Discard, rsp.Body)
	rsp.Body.Close()
	http.DefaultClient.CloseIdleConnections()

	buf := []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	result := make([]byte, 1024)

	for n := 0; n < 2; n++ {
		c, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
			return
		}
		_, err = c.Write(buf)
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
			c.Close()
			return
		}

		n, err := c.Read(result)
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
			c.Close()
			return
		}

		if n != 124 {
			t.Fatalf("Read %d bytes: %q", n, string(result[0:n]))
		}
		t.Logf("Read %d bytes: %q", n, string(result[0:n]))
		c.Close()

	}
}

func TestTCPListenerQueueListener(t *testing.T) {
	addr := ":9090"
	l, err := Listen(Options{
		Network:          "tcp",
		Address:          addr,
		MaxConcurrency:   10000,
		MaxQueueSize:     10000,
		MemoryLimitBytes: 1000,
		ConnectionBytes:  100,
		QueueTimeout:     time.Second,
		Metrics:          metrics.Default,
	})
	if err != nil {
		t.Fatalf("Failed to create QueueListener: %v", err)
	}
	defer l.Close()

	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("OK"))
		}),
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    1000,
	}
	defer srv.Close()

	cm := &skpnet.ConnManager{
		Keepalive:         10 * time.Second,
		KeepaliveRequests: 10,
		Metrics:           metrics.Default,
	}
	cm.Configure(srv)

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Errorf("Serve failed: %v", err)
		}
	}()

	// check ready
	err = fmt.Errorf("an error")
	var rsp *http.Response
	for err != nil {
		rsp, err = http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		time.Sleep(time.Millisecond)
	}
	io.Copy(io.Discard, rsp.Body)
	rsp.Body.Close()
	http.DefaultClient.CloseIdleConnections()

	buf := []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	result := make([]byte, 1024)

	for n := 0; n < 2; n++ {
		c, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
			return
		}
		_, err = c.Write(buf)
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
			c.Close()
			return
		}

		n, err := c.Read(result)
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
			c.Close()
			return
		}

		if n != 124 {
			t.Fatalf("Read %d bytes: %q", n, string(result[0:n]))
		}
		c.Close()

	}
}

func BenchmarkStackListener(b *testing.B) {
	maxprocs := runtime.GOMAXPROCS(-5)
	stackAddr := fmt.Sprintf(":90%02d", maxprocs)
	stackListener, err := StackListener(Options{
		Log:              &noLog{},
		Network:          "tcp",
		Address:          stackAddr,
		MaxConcurrency:   10000,
		MaxQueueSize:     10000,
		MemoryLimitBytes: 1000,
		ConnectionBytes:  100,
		QueueTimeout:     time.Second,
		//Metrics:          metrics.Default,
		Metrics: metrics.NoMetric{},
	})
	if err != nil {
		b.Fatalf("Failed to create StackListener: %v", err)
	}

	benchmarkListener(b, stackAddr, stackListener)
	stackListener.Close()

}

type noLog struct{}

func (*noLog) Error(...interface{})          {}
func (*noLog) Errorf(string, ...interface{}) {}
func (*noLog) Warn(...interface{})           {}
func (*noLog) Warnf(string, ...interface{})  {}
func (*noLog) Info(...interface{})           {}
func (*noLog) Infof(string, ...interface{})  {}
func (*noLog) Debug(...interface{})          {}
func (*noLog) Debugf(string, ...interface{}) {}

func BenchmarkQueueListener(b *testing.B) {
	queueAddr := ":9090"
	queueListener, err := Listen(Options{
		Log:              &noLog{},
		Network:          "tcp",
		Address:          queueAddr,
		MaxConcurrency:   10000,
		MaxQueueSize:     10000,
		MemoryLimitBytes: 1000,
		ConnectionBytes:  100,
		QueueTimeout:     time.Second,
		//Metrics:          metrics.Default,
		Metrics: metrics.NoMetric{},
	})
	if err != nil {
		b.Fatalf("Failed to create QueueListener: %v", err)
	}
	benchmarkListener(b, queueAddr, queueListener)
	queueListener.Close()
}

func benchmarkListener(b *testing.B, addr string, l net.Listener) {
	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("OK"))
		}),
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    1000,
	}
	defer srv.Close()

	cm := &skpnet.ConnManager{
		Keepalive:         10 * time.Second,
		KeepaliveRequests: 10,
		Metrics:           metrics.Default,
	}
	cm.Configure(srv)

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Errorf("Serve failed: %v", err)
		}
	}()

	// check ready
	err := fmt.Errorf("an error")
	var rsp *http.Response
	for err != nil {
		rsp, err = http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		time.Sleep(time.Millisecond)
	}
	io.Copy(io.Discard, rsp.Body)
	rsp.Body.Close()
	http.DefaultClient.CloseIdleConnections()

	// tr := http.Transport{
	// 	DisableKeepAlives: true,
	// }
	// client := http.Client{
	// 	Transport: &tr,
	// }

	buf := []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	result := make([]byte, 1024)
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		c, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			b.Logf("Failed to dial: %v", err)
			return
		}
		_, err = c.Write(buf)
		if err != nil {
			b.Logf("Failed to write: %v", err)
			c.Close()
			return
		}

		n, err := c.Read(result)
		if err != nil {
			b.Logf("Failed to write: %v", err)
			c.Close()
			return
		}

		if n != 124 {
			b.Logf("Read %d bytes: %q", n, string(result[0:n]))
		}
		c.Close()
	}

	/*
		for n := 0; n < b.N; n++ {
			// TODO(sszuecs): we should create connections TCP/IP
			// and not use a pooled client

			rsp, err := client.Get("http://" + l.Addr().String() + "/")
			if err != nil {
				b.Fatalf("Failed to send request: %v", err)
			}
			if rsp.StatusCode != http.StatusAccepted {
				b.Fatalf("Failed to get status code: %d != %d", rsp.StatusCode, http.StatusAccepted)
			}
			res, err := io.ReadAll(rsp.Body)
			if result := string(res); result != "OK" {
				b.Logf("Failed to get result: %q", result)
			}
			rsp.Body.Close()
		}
		client.CloseIdleConnections()
	*/
}
