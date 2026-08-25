package diag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestLogBodyCreateFilter(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []any
		want error
	}{
		{
			name: "no args should fail",
			args: []any{},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "less than expected args should fail",
			args: []any{"request"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg0 string should fail",
			args: []any{"REQUEST", 10},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg0 type should fail",
			args: []any{5, 10},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg1 type should fail",
			args: []any{"request", "foo"},
			want: filters.ErrInvalidFilterParameters,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewLogBody()
			_, err := spec.CreateFilter(tt.args)
			if !errors.Is(err, filters.ErrInvalidFilterParameters) {
				t.Fatalf("Failed to get filter error: %v, for args: %v", err, tt.args)
			}
		})
	}

}

func TestLogBody(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	t.Run("Request", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> absorbSilent() -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewAbsorbSilent())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request", 1024) -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequest"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := p.Client().Post(p.URL, "text/plain", buf)
		log.SetOutput(os.Stderr)
		if err != nil {
			t.Fatalf("Failed to POST: %v", err)
		}
		defer rsp.Body.Close()

		if got := logbuf.String(); !strings.Contains(got, content) {
			t.Fatalf("Failed to find %q log, got: %q", content, got)
		}
	})

	t.Run("Response", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("response", 1024) -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequest"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := p.Client().Post(p.URL, "text/plain", buf)
		if err != nil {
			t.Fatalf("Failed to do post request: %v", err)
		}

		defer rsp.Body.Close()
		io.Copy(io.Discard, rsp.Body)
		log.SetOutput(os.Stderr)

		got := logbuf.String()
		if strings.Contains(got, content) {
			t.Fatalf("Found request body %q in %q", content, got)
		}
		// repeatContent("a", 10)
		if !strings.Contains(got, "aaaaaaaaaa") {
			t.Fatalf("Failed to find rsp content %q log, got: %q", "aaaaaaaaaa", got)
		}
	})

	t.Run("Request-response chaining", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request", 1024) -> logBody("response", 1024) -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		requestContent := "testrequestresponsechain"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(requestContent)
		rsp, err := p.Client().Post(p.URL, "text/plain", buf)
		if err != nil {
			t.Fatalf("Failed to get respone: %v", err)
		}
		defer rsp.Body.Close()
		io.Copy(io.Discard, rsp.Body)
		log.SetOutput(os.Stderr)

		got := logbuf.String()
		if !strings.Contains(got, requestContent) {
			t.Fatalf("Failed to find req %q log, got: %q", requestContent, got)
		}
		// repeatContent("a", 10)
		if !strings.Contains(got, "aaaaaaaaaa") {
			t.Fatalf("Failed to find %q log, got: %q", "aaaaaaaaaa", got)
		}
	})

	t.Run("Request with limit", func(t *testing.T) {
		count := 1024
		content := strings.Repeat("b", count)
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b := make([]byte, 0, count)
			buf := bytes.NewBuffer(b)
			_, err := io.Copy(buf, r.Body)
			if err != nil {
				t.Fatalf("Failed to read body on backend receiver: %v", err)
			}

			if got := buf.String(); len(got) != count {
				t.Fatalf("Failed to get request body in backend. want: %q, got: %q", content, got)
			}
			w.WriteHeader(200)
			w.Write([]byte(strings.Repeat("a", count)))
		}))
		defer backend.Close()

		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewRepeat())

		limit := 10
		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request", %d) -> "%s"`, limit, backend.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := p.Client().Post(p.URL, "text/plain", buf)
		log.SetOutput(os.Stderr)
		if err != nil {
			t.Fatalf("Failed to POST: %v", err)
		}
		defer rsp.Body.Close()

		want := ` \"` + content[:limit] + "\\\"\"" + "\n"
		got := logbuf.String()
		from := len(got) - limit - 7
		if want != got[from:] {
			t.Fatalf("Failed want suffix: %q, got: %q\nwant hex: %x\ngot hex : %x", want, got, want, got[from:])
		}
	})

	t.Run("Response with limit", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> repeatContent("a", 1024) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewAbsorbSilent())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("response", 10) -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequest"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := p.Client().Post(p.URL, "text/plain", buf)
		if err != nil {
			t.Fatalf("Failed to do post request: %v", err)
		}

		rspBuf := bytes.NewBuffer(nil)
		io.Copy(rspBuf, rsp.Body)
		rsp.Body.Close()
		log.SetOutput(os.Stderr)

		got := logbuf.String()
		if strings.Contains(got, content) {
			t.Fatalf("Found request body %q in %q", content, got)
		}

		// repeatContent("a", 1024) but only 10 bytes
		want := " \\\"" + strings.Repeat("a", 10) + "\\\"\"" + "\n"
		if !strings.HasSuffix(got, want) {
			t.Fatalf("Failed to find rsp content %q log, got: %q", want, got)
		}

		// rsp body is not truncated
		data := rspBuf.String()
		if data != strings.Repeat("a", 1024) {
			t.Fatalf("Failed to not change response body(%d): %v", len(data), data)
		}
	})
}

type mybuf struct {
	buf *bytes.Buffer
}

func (mybuf) Close() error {
	return nil
}

func (b mybuf) Read(p []byte) (int, error) {
	return b.buf.Read(p)
}

func TestHttpBodyLogBodyStream(t *testing.T) {
	t.Run("logbodystream request", func(t *testing.T) {
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

		lg := func(format string, args ...any) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}

		body := newLogBodyStream(
			len(sent),
			func(chunk []byte) {
				lg(
					`logBody("request") %s: %s`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
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

	t.Run("logbodystream request with limit", func(t *testing.T) {
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

		lg := func(format string, args ...any) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}

		limit := 10
		body := newLogBodyStream(
			limit,
			func(chunk []byte) {
				lg(
					`logBody("request") %s: %s`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
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
		wantLogData := fmt.Sprintf(`logBody("request") %s: %s`, req.Header.Get(flowid.HeaderName), sent[:limit])
		if wantLogData != lgData {
			t.Fatalf("Failed to get log %q, got %q", wantLogData, lgData)
		}
	})

	t.Run("logbodystream response", func(t *testing.T) {
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

		lg := func(format string, args ...any) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}
		body := newLogBodyStream(
			len(sent),
			func(chunk []byte) {
				lg(
					`logBody("response") %s: %s`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
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

	t.Run("logbodystream response with limit", func(t *testing.T) {
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

		lg := func(format string, args ...any) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}
		limit := 10
		body := newLogBodyStream(
			limit,
			func(chunk []byte) {
				lg(
					`logBody("response") %s: %s`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
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
		wantLogData := fmt.Sprintf(`logBody("response") %s: %s`, req.Header.Get(flowid.HeaderName), sent[:limit])
		if wantLogData != lgData {
			t.Fatalf("Failed to get log %q, got %q", wantLogData, lgData)
		}
	})

	t.Run("logbodystream response with canceled request", func(t *testing.T) {
		sent := strings.Repeat("b", 1024)
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
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte("OK"))
		}))
		defer backend.Close()

		lgbuf := &bytes.Buffer{}

		var b mybuf
		b.buf = bytes.NewBufferString(sent)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", backend.URL, b.buf)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Add(flowid.HeaderName, "qux")

		rsp, err := backend.Client().Do(req)
		if err != nil {
			t.Fatalf("Failed to do request, expect no error, but go: %v", err)
		}
		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get the expected status code 200, got: %d", rsp.StatusCode)
		}

		lg := func(format string, args ...any) {
			s := fmt.Sprintf(format, args...)
			lgbuf.WriteString(s)
		}
		body := newLogBodyStream(
			len(sent),
			func(chunk []byte) {
				lg(
					`logBody("response") %s: %s`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
			rsp.Body,
		)
		defer body.Close()
		rsp.Body = body

		var buf bytes.Buffer
		_, err = io.Copy(&buf, rsp.Body)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Failed to get expected error: %v", err)
		}

		rsp.Body.Close()
		rspBody := buf.String()
		if rspBody != "" {
			t.Fatalf("Failed to get empty response body, got: %q", rspBody)
		}

		lgData := lgbuf.String()
		if lgData != "" {
			t.Fatalf("Failed to get empty log, got: %q", lgData)
		}
	})
}
