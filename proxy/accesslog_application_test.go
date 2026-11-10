package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/logging"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestAccessLogApplicationFromAnnotateFilter verifies that a route with
// annotate("Application", ...) produces an access log entry with the application name.
func TestAccessLogApplicationFromAnnotateFilter(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer service.Close()

	var buf bytes.Buffer
	accessLogger := logging.NewAccessLogger(logging.Options{
		AccessLogOutput:      &buf,
		AccessLogJSONEnabled: true,
	})

	doc := fmt.Sprintf(`r: * -> annotate("Application", "my-service") -> "%s"`, service.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		AccessLogger: accessLogger,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	r, _ := http.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, r)

	got := buf.String()
	if !strings.Contains(got, `"application":"my-service"`) {
		t.Errorf("access log missing application field, got: %s", got)
	}
}

// TestAccessLogApplicationFromAnnotateFilterText verifies the text format
// access log also contains the application name.
func TestAccessLogApplicationFromAnnotateFilterText(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer service.Close()

	var buf bytes.Buffer
	accessLogger := logging.NewAccessLogger(logging.Options{
		AccessLogOutput: &buf,
	})

	doc := fmt.Sprintf(`r: * -> annotate("Application", "my-service") -> "%s"`, service.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		AccessLogger: accessLogger,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	r, _ := http.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, r)

	got := buf.String()
	if !strings.Contains(got, "my-service") {
		t.Errorf("access log missing application name, got: %s", got)
	}
}

// TestAccessLogApplicationEndToEnd starts a real HTTP server backed by the proxy,
// sends a request through it, and verifies the access log entry contains the
// application name from the annotate filter.
func TestAccessLogApplicationEndToEnd(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	var buf syncBuffer
	accessLogger := logging.NewAccessLogger(logging.Options{
		AccessLogOutput:      &buf,
		AccessLogJSONEnabled: true,
	})

	doc := fmt.Sprintf(`r: * -> annotate("Application", "my-service") -> "%s"`, backend.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		AccessLogger: accessLogger,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	resp, err := http.Get(ps.URL + "/test")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), `"application":"my-service"`) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("access log missing application field, got: %s", buf.String())
}

// TestClientCanceledLogHasApplicationField verifies that when a client cancels a request
// on a route with annotate("Application", ...), the "client canceled" log message
// includes the application field.
func TestClientCanceledLogHasApplicationField(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer service.Close()

	doc := fmt.Sprintf(`r: * -> annotate("Application", "my-service") -> "%s"`, service.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	if err := postTruncated(ps.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}

	if err := testLog.WaitFor("client canceled", 3*time.Second); err != nil {
		t.Fatalf("expected 'client canceled' in log: %v", err)
	}

	got := testLog.String()
	if !strings.Contains(got, "application=my-service") {
		t.Errorf("log missing application field, got: %s", got)
	}
}
