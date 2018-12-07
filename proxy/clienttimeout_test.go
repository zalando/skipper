package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientTimeout(t *testing.T) {
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
	tp.log.Unmute()

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
		t.Error("err should be nil")
	}
	if rsp != nil {
		t.Error("response should be nil")
	}

	const msgErrClientTimeout = "context canceled"
	if err = tp.log.WaitFor(msgErrClientTimeout, 3*d); err != nil {
		t.Errorf("log should contain '%s'", msgErrClientTimeout)
	}
}
