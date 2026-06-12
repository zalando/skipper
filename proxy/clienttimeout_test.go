package proxy

import (
	"bufio"
	"fmt"
	"io"
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
	for i := 0; i < N; i++ {
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

func TestReadTimeoutSocketClosed(t *testing.T) {
	backend := startTestServer([]byte("backend reply"), 0, func(*http.Request) {})
	defer backend.Close()

	doc := fmt.Sprintf(`
		r1: Path("/timeout") -> readTimeout("10ms") -> status(201) -> "%s";
		r2: Path("/ok") -> status(200) -> inlineContent("OK") -> <shunt>
	`, backend.URL)

	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	addr := ps.Listener.Addr().String()

	readResp := func(conn net.Conn) *http.Response {
		t.Helper()
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		_, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		resp.Body.Close()

		return resp
	}

	// Verify both routes return their configured status codes (normal client).
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/timeout", http.StatusCreated},
		{"/ok", http.StatusOK},
	} {
		resp, err := ps.Client().Get("http://" + addr + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("GET %s: got %d, want %d", tc.path, resp.StatusCode, tc.want)
		}
	}

	postConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial post conn: %v", err)
	}
	defer postConn.Close()

	// Write only the headers + partial body, then stall. Do not close the
	// connection — the proxy must wait for more bytes until the deadline fires.
	if _, err := postConn.Write([]byte("POST /timeout HTTP/1.1\r\nHost: " + addr + "\r\nContent-Length: 2000\r\n\r\ntruncated")); err != nil {
		t.Fatalf("Failed to write POST stall: %v", err)
	}
	postResp := readResp(postConn)
	if postResp.StatusCode != 499 {
		t.Errorf("POST /timeout with stalled body: got %d, want 499", postResp.StatusCode)
	}
	// Because of https://github.com/zalando/skipper/issues/4060 and https://github.com/golang/go/issues/70834 we want to have a closed connection
	if !postResp.Close {
		t.Fatalf("Failed to get close connection header")
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
