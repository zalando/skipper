package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientTimeout(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	d := 200 * time.Millisecond

	payload := []byte("backend reply")
	backend := startTestServer(payload, 0, func(r *http.Request) {
		time.Sleep(2 * d)
	})

	defer backend.Close()

	doc := fmt.Sprintf(`hello: * -> "%s"`, backend.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error()
		return
	}

	ps := httptest.NewServer(tp.proxy)
	defer func() {
		ps.Close()
		tp.close()
	}()

	req, err := http.NewRequest("GET", ps.URL, nil)
	if err != nil {
		t.Errorf("Failed to create request: %v", err)
	}

	rsp, err := (&http.Client{
		Timeout: d,
	}).Do(req)

	if err == nil {
		t.Error("err should not be nil")
	}
	if rsp != nil {
		t.Error("response should be nil")
	}

	const msgErrClientTimeout = "context canceled"
	if err = testLog.WaitFor(msgErrClientTimeout, 3*d); err != nil {
		t.Errorf("log should contain '%s'", msgErrClientTimeout)
	}
	const msgErrClientCanceledAfter = "client canceled after"
	if err = testLog.WaitFor(msgErrClientCanceledAfter, 3*d); err != nil {
		t.Errorf("log should contain '%s'", msgErrClientCanceledAfter)
	}
}

func TestClientCancellation(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer service.Close()

	doc := fmt.Sprintf(`* -> "%s"`, service.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	addr := ps.Listener.Addr().String()

	const N = 100
	for range N {
		if err := postTruncated(addr); err != nil {
			t.Fatal(err)
		}
	}

	const msg = "POST / HTTP/1.1"
	if err = testLog.WaitForN(msg, N, 500*time.Millisecond); err != nil {
		t.Fatalf("expected %d requests, got %d", N, tp.log.Count(msg))
	}

	// Look for N messages like
	// 2020/12/05 17:24:00 client canceled after 1.090638ms, route  with backend network http://127.0.0.1:39687, status code 499: dialing failed false: context canceled, remote host: 127.0.0.1, request: "POST / HTTP/1.1", user agent: ""
	for _, m := range []string{"client canceled after", "status code 499", "context canceled"} {
		count := testLog.Count(m)
		if count != N {
			t.Errorf("expected '%s' %d times, got %d", m, N, tp.log.Count(m))
		}
	}
}

func postTruncated(addr string) (err error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return
	}
	_, err = conn.Write([]byte("POST / HTTP/1.1\nHost: " + addr + "\nContent-Length: 100\n\ntruncated"))
	if err != nil {
		return
	}
	return conn.Close()
}

func TestClientTimeoutBeforeStreaming(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	backend := startTestServer([]byte("backend reply"), 0, func(*http.Request) {})
	defer backend.Close()

	doc := fmt.Sprintf(`hello: * -> latency("100ms") -> "%s"`, backend.URL)
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := (&http.Client{
		Timeout: 50 * time.Millisecond,
	}).Do(req)
	if err == nil || rsp != nil {
		t.Errorf("unexpected err or response: %v %v", err, rsp)
	}

	const msg = "Client request: context canceled"
	if err = testLog.WaitFor(msg, 200*time.Millisecond); err != nil {
		t.Errorf("log should contain '%s'", msg)
	}
}
