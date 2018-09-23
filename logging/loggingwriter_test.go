package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWritesAndCounts(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &loggingWriter{writer: rr}

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
	w := &loggingWriter{writer: rr}
	w.WriteHeader(http.StatusTeapot)

	if rr.Code != http.StatusTeapot {
		t.Error("failed to write status code")
	}

	if w.code != http.StatusTeapot {
		t.Error("failed to store status code")
	}
}

func TestReturnsUnderlyingHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &loggingWriter{writer: rr}
	w.Header().Set("X-Test-Header", "test-value")
	if rr.Header().Get("X-Test-Header") != "test-value" {
		t.Error("failed to set the header")
	}
}

func TestFlushesPartialPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &loggingWriter{writer: rr}
	w.Write([]byte("Hello, world!"))
	w.Flush()
	if !rr.Flushed {
		t.Error("failed to flush underlying writer")
	}
}

func TestSets200OnMissingStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &loggingWriter{writer: rr}
	w.WriteHeader(0)

	if w.code != http.StatusOK {
		t.Errorf("failed to overwrite status code. Expected 200 but got %d", w.code)
	}
}
