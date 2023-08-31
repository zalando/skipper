package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlowService(t *testing.T) {
	wait := make(chan struct{})

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-wait
	}))
	defer func() {
		close(wait)
		service.Close()
	}()

	doc := fmt.Sprintf(`* -> backendTimeout("100ms") -> "%s"`, service.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got: %v", rsp)
	}
}

func TestFastService(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Millisecond)
	}))
	defer service.Close()

	doc := fmt.Sprintf(`* -> backendTimeout("100ms") -> "%s"`, service.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got: %v", rsp)
	}
}

func TestBackendTimeoutInTheMiddleOfServiceResponse(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("Wish You"))

		f := w.(http.Flusher)
		f.Flush()

		time.Sleep(200 * time.Millisecond)

		w.Write([]byte(" Were Here"))
	}))
	defer service.Close()

	doc := fmt.Sprintf(`* -> backendTimeout("100ms") -> "%s"`, service.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got: %v", rsp)
	}

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Error(err)
	}

	content := string(body)
	if content != "Wish You" {
		t.Errorf("expected partial content, got %s", content)
	}

	const msg = "error while copying the response stream: context deadline exceeded"
	if err = testLog.WaitFor(msg, 100*time.Millisecond); err != nil {
		t.Errorf("expected '%s' in logs", msg)
	}
}

type unstableRoundTripper struct {
	inner   http.RoundTripper
	timeout time.Duration
	attempt int
}

// Simulates dial timeout on every odd request
func (r *unstableRoundTripper) RoundTrip(req *http.Request) (rsp *http.Response, err error) {
	if r.attempt%2 == 0 {
		time.Sleep(r.timeout)
		rsp, err = nil, &proxyError{
			code:          -1,   // omit 0 handling in proxy.Error()
			dialingFailed: true, // indicate error happened before http
		}
	} else {
		rsp, err = r.inner.RoundTrip(req)
	}
	r.attempt = r.attempt + 1
	return
}

func newUnstable(timeout time.Duration) func(r http.RoundTripper) http.RoundTripper {
	return func(r http.RoundTripper) http.RoundTripper {
		return &unstableRoundTripper{inner: r, timeout: timeout}
	}
}

// Retryable request, dial timeout on first attempt, load balanced backend
// dial timeout (100ms) + service latency (100ms) > backendTimeout("150ms") => Gateway Timeout
func TestRetryAndSlowService(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer service.Close()

	doc := fmt.Sprintf(`* -> backendTimeout("150ms") -> <"%s", "%s">`, service.URL, service.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		CustomHttpRoundTripperWrap: newUnstable(100 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got: %v", rsp)
	}
}

// Retryable request, dial timeout on first attempt, load balanced backend
// dial timeout (100ms) + service latency (100ms) < backendTimeout("250ms") => OK
func TestRetryAndFastService(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer service.Close()

	doc := fmt.Sprintf(`* -> backendTimeout("250ms") -> <"%s", "%s">`, service.URL, service.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		CustomHttpRoundTripperWrap: newUnstable(100 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got: %v", rsp)
	}
}
