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
	"encoding/hex"
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
	return postBodyWithFilter(t, fr, backendURL, body, `blockContent("MALICIOUS")`)
}

func postBodyWithFilter(t *testing.T, fr filters.Registry, backendURL, body, filterExpr string) (int, error) {
	t.Helper()
	r := eskip.MustParse(fmt.Sprintf(`* -> %s -> "%s"`, filterExpr, backendURL))
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

	// multiple needles configured, one straddles the boundary
	t.Run("multiple needles one straddles boundary is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		// "SAFE" appears fully inside the first slice; "MALICIOUS" straddles the boundary.
		// Neither alone would bypass when matched correctly; this ensures the multi-needle
		// case does not regress.
		body := "SAFE" + strings.Repeat("A", boundary-12) + "MALICIOUS" + strings.Repeat("B", 100)
		r := eskip.MustParse(fmt.Sprintf(`* -> blockContent("SAFE", "MALICIOUS") -> "%s"`, backend.URL))
		proxy := proxytest.New(fr, r...)
		defer proxy.Close()

		req, err := http.NewRequest("POST", proxy.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		rsp, err := proxy.Client().Do(req)
		if err == nil {
			defer rsp.Body.Close()
			if rsp.StatusCode != 400 {
				t.Errorf("multi-needle: want 400, got %d", rsp.StatusCode)
			}
		}
		if rec.pattern {
			t.Errorf("multi-needle: blocked pattern reached backend")
		}
	})

	// needle starts exactly at the seam (no straddling; regression guard)
	t.Run("needle at exact start of second chunk is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		body := strings.Repeat("A", boundary) + "MALICIOUS" + strings.Repeat("B", 100)
		status, err := postBody(t, fr, backend.URL, body)
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("seam: want 400, got %d", status)
		}
		if rec.pattern {
			t.Errorf("seam: blocked pattern reached backend")
		}
	})

	// needle fully inside the second chunk (regression guard)
	t.Run("needle in second chunk is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		body := strings.Repeat("A", boundary+100) + "MALICIOUS" + strings.Repeat("B", 100)
		status, err := postBody(t, fr, backend.URL, body)
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("second-chunk: want 400, got %d", status)
		}
		if rec.pattern {
			t.Errorf("second-chunk: blocked pattern reached backend")
		}
	})
}

func TestBlockContentHexStraddlingReadBoundary(t *testing.T) {
	hexSpec := NewBlockHex(testMaxMatcherBuffer)
	fr := make(filters.Registry)
	fr.Register(hexSpec)

	// hex encoding of "MALICIOUS"
	hexNeedle := hex.EncodeToString([]byte("MALICIOUS"))
	filterExpr := fmt.Sprintf(`blockContentHex("%s")`, hexNeedle)

	t.Run("blockContentHex needle straddling 32KiB boundary is blocked", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		body := strings.Repeat("A", boundary-8) + "MALICIOUS" + strings.Repeat("B", 100)
		status, err := postBodyWithFilter(t, fr, backend.URL, body, filterExpr)
		if err != nil {
			t.Fatal(err)
		}
		if status != 400 {
			t.Errorf("hex boundary: want 400, got %d", status)
		}
		if rec.pattern {
			t.Errorf("hex boundary: blocked pattern reached backend")
		}
	})

	t.Run("blockContentHex clean body passes", func(t *testing.T) {
		var rec receipt
		backend := recordingBackend(&rec)
		defer backend.Close()

		status, err := postBodyWithFilter(t, fr, backend.URL, strings.Repeat("A", boundary+100), filterExpr)
		if err != nil {
			t.Fatal(err)
		}
		if status != 200 {
			t.Errorf("hex clean: want 200, got %d", status)
		}
		if !rec.received {
			t.Errorf("hex clean: backend did not receive the body")
		}
	})
}
