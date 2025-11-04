package nettest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlowAcceptListener(t *testing.T) {
	slowBackend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("slow accept listener backend"))
	}))
	l, err := NewSlowAcceptListener(&SlowAcceptListenerOptions{
		Network: "tcp",
		Address: ":0",
		Delay:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	slowBackend.Listener = l
	slowBackend.Start()
	defer slowBackend.Close()

	req, err := http.NewRequest("GET", slowBackend.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	timeBeforeRequest := time.Now()
	rsp, err := slowBackend.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get response, want %d, got %d", 200, rsp.StatusCode)
	}

	io.Copy(io.Discard, rsp.Body)
	if d := time.Since(timeBeforeRequest); d < 100*time.Millisecond {
		t.Fatalf("Failed to have slow response %v < %v", d, 100*time.Millisecond)
	}

	// restore delay will have a fast response
	l.Delay(0)
	slowBackend.Client().CloseIdleConnections()
	timeBeforeRequest = time.Now()
	rsp, err = slowBackend.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get response, want %d, got %d", 200, rsp.StatusCode)
	}

	io.Copy(io.Discard, rsp.Body)
	if d := time.Since(timeBeforeRequest); d > 100*time.Millisecond {
		t.Fatalf("Failed to have slow response %v > %v", d, 100*time.Millisecond)
	}
}

func TestSlowAcceptListenerFailedCreation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		network string
		address string
	}{
		{
			name:    "test wrong network",
			network: "foo",
			address: ":0",
		},
		{
			name:    "test wrong address",
			network: "tcp",
			address: "foo",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSlowAcceptListener(&SlowAcceptListenerOptions{
				Network: tt.network,
				Address: tt.address,
				Delay:   time.Second,
			})
			if err == nil {
				t.Fatal("Failed to get an error")
			}
		})
	}
}

func TestSlowAcceptListenerClosedListener(t *testing.T) {
	l, err := NewSlowAcceptListener(&SlowAcceptListenerOptions{
		Network: "tcp",
		Address: ":0",
		Delay:   time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	l.Close()

	_, err = l.Accept()
	if err == nil {
		t.Fatal("Failed to get an error from Accept(), but listener was closed")
	}
}
