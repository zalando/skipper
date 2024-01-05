package io

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters/flowid"
)

func TestHttpBodyLogBody(t *testing.T) {
	t.Run("logbody request", func(t *testing.T) {
		sent := strings.Repeat("a", 1024)
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b := make([]byte, 0, 1024)
			buf := bytes.NewBuffer(b)
			_, err := io.Copy(buf, r.Body)
			if err != nil {
				t.Fatalf("Failed to read body on backend receiver: %v", err)
			}

			if got := buf.String(); got != sent {
				t.Fatalf("Failed to get request body in backend. want: %q, got: %q", sent, got)
			}
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		}))
		defer backend.Close()

		lgbuf := &bytes.Buffer{}

		var b mybuf
		b.buf = bytes.NewBufferString(sent)

		req, err := http.NewRequest("POST", backend.URL, b.buf)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Add(flowid.HeaderName, "foo")

		lg := func(format string, args ...interface{}) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}

		body := LogBody(
			context.Background(),
			fmt.Sprintf(`logBody("request") %s: `, req.Header.Get(flowid.HeaderName)),
			lg,
			req.Body,
		)
		defer body.Close()
		req.Body = body

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatalf("Failed to do POST request, got err: %v", err)
		}
		defer rsp.Body.Close()

		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get the expected status code 200, got: %d", rsp.StatusCode)
		}

		lgData := lgbuf.String()
		wantLogData := fmt.Sprintf(`logBody("request") %s: %s`, req.Header.Get(flowid.HeaderName), sent)
		if wantLogData != lgData {
			t.Fatalf("Failed to get log %q, got %q", wantLogData, lgData)
		}
	})

	t.Run("logbody response", func(t *testing.T) {
		sent := strings.Repeat("a", 512)
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(sent))
		}))
		defer backend.Close()

		lgbuf := &bytes.Buffer{}

		req, err := http.NewRequest("GET", backend.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Add(flowid.HeaderName, "bar")

		rsp, err := backend.Client().Do(req)
		if err != nil {
			t.Fatalf("Failed to do POST request, got err: %v", err)
		}

		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get the expected status code 200, got: %d", rsp.StatusCode)
		}

		lg := func(format string, args ...interface{}) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}
		body := LogBody(
			context.Background(),
			fmt.Sprintf(`logBody("response") %s: `, req.Header.Get(flowid.HeaderName)),
			lg,
			rsp.Body,
		)
		defer body.Close()
		rsp.Body = body

		var buf bytes.Buffer
		io.Copy(&buf, rsp.Body)
		rsp.Body.Close()
		rspBody := buf.String()
		if rspBody != sent {
			t.Fatalf("Failed to get sent %q, got rspbody %q", sent, rspBody)
		}

		lgData := lgbuf.String()
		wantLogData := fmt.Sprintf(`logBody("response") %s: %s`, req.Header.Get(flowid.HeaderName), sent)
		if wantLogData != lgData {
			t.Fatalf("Failed to get log %q, got %q", wantLogData, lgData)
		}
	})

}
