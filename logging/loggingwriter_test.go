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
