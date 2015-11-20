package logging

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestServesRequest(t *testing.T) {
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})

	h := NewHandler(innerHandler)
	body := "Hello, world!"
	r, _ := http.NewRequest("POST",
		"http://www.example.org",
		bytes.NewBufferString(body))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	back := w.Body.String()

	if back != body {
		t.Error("failed to serve request")
		t.Log("expected body:", body)
		t.Log("got body:     ", back)
	}
}

func TestLogsAccess(t *testing.T) {
	var accessLog bytes.Buffer
	Init(Options{AccessLogOutput: &accessLog})

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := NewHandler(innerHandler)

	h.ServeHTTP(httptest.NewRecorder(), &http.Request{})

	output := accessLog.String()
	if !strings.Contains(output, strconv.Itoa(http.StatusTeapot)) {
		t.Error("failed to log access")
	}
}
