package queuelistener

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
	skpnet "github.com/zalando/skipper/net"
)

func TestStackListener(t *testing.T) {
	addr := ":9090"
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
		t.Fatalf("Failed to create StackListener: %v", err)
	}
	defer l.Close()

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Errorf("Serve failed: %v", err)
		}
	}()

	dst := "http://" + l.Addr().String()
	rsp, err := http.DefaultClient.Get(dst)
	if err != nil {
		t.Fatalf("Failed to do a GET request to %s", dst)
	}
	if rsp.StatusCode != http.StatusAccepted {
		t.Fatalf("Failed to get response status code we expect, got: %d", rsp.StatusCode)
	}
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)
	http.DefaultClient.CloseIdleConnections()
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
			log.Fatalf("Serve failed: %v", err)
		}
	}()

	err = fmt.Errorf("an error")
	for err != nil {
		_, err = http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		time.Sleep(time.Millisecond)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		rsp, err := http.DefaultClient.Get("http://" + l.Addr().String() + "/")
		if err != nil {
			b.Fatalf("Failed to send request: %v", err)
		}
		if rsp.StatusCode != http.StatusAccepted {
			b.Fatalf("Failed to get status code: %d != %d", rsp.StatusCode, http.StatusAccepted)
		}
	}

}
