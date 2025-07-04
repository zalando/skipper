package queuelistener

import (
	"fmt"
	"io"
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

func BenchmarkStackListener(b *testing.B) {
	maxprocs := runtime.GOMAXPROCS(-5)
	addr := fmt.Sprintf(":90%02d", maxprocs)

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
		Keepalive:         0,
		KeepaliveRequests: 0,
		Metrics:           metrics.Default,
	}
	cm.Configure(srv)

	l, err := StackListener(Options{
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
		b.Fatalf("Failed to create StackListener: %v", err)
	}
	defer l.Close()

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

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		rsp, err := http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		if err != nil {
			b.Fatalf("Failed to send request: %v", err)
		}
		if rsp.StatusCode != http.StatusAccepted {
			b.Fatalf("Failed to get status code: %d != %d", rsp.StatusCode, http.StatusAccepted)
		}
		io.Copy(io.Discard, rsp.Body)
		rsp.Body.Close()
		http.DefaultClient.CloseIdleConnections()
	}
}
