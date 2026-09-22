package block

// Reproducer for the blockContent cross-boundary bypass (GHSA-373c-6ffw-j63p).
//
// The matcher in io/read_stream.go scans each consumer-read slice separately,
// so a blocked string that straddles the transport copy-buffer boundary
// (32 KiB) is never present in any single scanned slice and the request is
// forwarded to the backend. These tests fail on the unfixed code and pass
// once matcher.Read scans the whole pending buffer before slicing.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	// production default of -max-matcher-buffer-size
	testMaxMatcherBuffer = 2 * 1024 * 1024
	// transport copy buffer / consumer read-chunk size
	boundary = 32768
)

type receipt struct {
	received bool
	length   int
	pattern  bool
}

func recordingBackend(rec *receipt) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err == nil && len(b) > 0 {
			rec.received = true
			rec.length += len(b)
			if bytes.Contains(b, []byte("MALICIOUS")) {
				rec.pattern = true
			}
		}
		r.Body.Close()
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
}

func postBody(t *testing.T, fr filters.Registry, backendURL, body string) (int, error) {
	r := eskip.MustParse(fmt.Sprintf(`* -> blockContent("MALICIOUS") -> "%s"`, backendURL))
	proxy := proxytest.New(fr, r...)
	defer proxy.Close()

	req, err := http.NewRequest("POST", proxy.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := proxy.Client().Do(req)
	if err != nil {
		return 0, err
	}
	defer rsp.Body.Close()
	return rsp.StatusCode, nil
}

func TestBlockContentStraddlingReadBoundary(t *testing.T) {
	spec := NewBlock(testMaxMatcherBuffer)
	fr := make(filters.Registry)
	fr.Register(spec)

	// control: needle fully inside a single read slice must be blocked
	t.Run("needle inside one slice is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		status, err := postBody(t, fr, backend.URL, "hello MALICIOUS world")
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("control: want 400, got %d", status)
		}
	})

	// attack: needle straddles the 32 KiB consumer-read boundary
	t.Run("needle straddling 32KiB boundary is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		body := strings.Repeat("A", boundary-8) + "MALICIOUS" + strings.Repeat("B", 100)
		status, err := postBody(t, fr, backend.URL, body)
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("boundary: want 400, got %d (blocked content reached backend: %v, %d bytes)",
				status, rec.pattern, rec.length)
		}
		if rec.pattern {
			t.Errorf("boundary: backend received the blocked pattern")
		}
	})

	// attack: same at the second 32 KiB boundary
	t.Run("needle straddling 64KiB boundary is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		body := strings.Repeat("A", 2*boundary-6) + "MALICIOUS" + strings.Repeat("B", 100)
		status, err := postBody(t, fr, backend.URL, body)
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("2nd boundary: want 400, got %d (blocked content reached backend: %v, %d bytes)",
				status, rec.pattern, rec.length)
		}
		if rec.pattern {
			t.Errorf("2nd boundary: backend received the blocked pattern")
		}
	})

	// control: clean body of the same size must pass
	t.Run("clean body of attack size passes", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		status, err := postBody(t, fr, backend.URL, strings.Repeat("A", boundary+100))
		if err != nil {
			t.Fatal(err)
		}
		if status != 200 {
			t.Errorf("clean: want 200, got %d", status)
		}
		if !rec.received {
			t.Errorf("clean: backend did not receive the body")
		}
	})
}
