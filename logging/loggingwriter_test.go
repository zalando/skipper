package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrites(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}

	body := "Hello, world!"
	w.Write([]byte(body))
	back := rr.Body.String()

	if back != body {
		t.Error("failed to write body")
	}

	if w.bytes != int64(len(body)) {
		t.Error("failed to count bytes")
	}
}

func TestWritesAndStoresStatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.WriteHeader(http.StatusTeapot)

	if rr.Code != http.StatusTeapot {
		t.Error("failed to write status code")
	}

	if w.GetCode() != http.StatusTeapot {
		t.Error("failed to get status code")
	}

	w.WriteHeader(0)
	if w.GetCode() != http.StatusOK {
		t.Errorf("Failed to get default status %d", http.StatusOK)
	}

}

func TestReturnsUnderlyingHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.Header().Set("X-Test-Header", "test-value")
	if rr.Header().Get("X-Test-Header") != "test-value" {
		t.Error("failed to set the header")
	}
}

func TestFlushesPartialPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.Write([]byte("Hello, world!"))
	w.Flush()
	if !rr.Flushed {
		t.Error("failed to flush underlying writer")
	}
}

func TestHijack(t *testing.T) {
	rr := httptest.NewRecorder()
	w := NewLoggingWriter(rr)
	w.Write([]byte("Hello, world!"))
	_, _, err := w.Hijack()
	if err == nil {
		t.Fatal("Failed to get hijack error")
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("Failed to cast responsewriter to Hijacker")
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("Failed to get hijacker: %v", err)
		}
		defer conn.Close()

		rw.Write([]byte("foo"))
		buf := make([]byte, 0, 3)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("Failed to read from conn: %v", err)
		}
		if n != 3 {
			t.Fatalf("Failed to read 3 bytes, read %d bytes", n)
		}
	}))
	defer backend.Close()

}
