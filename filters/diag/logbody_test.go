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
	"runtime"
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
		args []interface{}
		want error
	}{
		{
			name: "no args should fail",
			args: []interface{}{},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "less than expected args should fail",
			args: []interface{}{"request"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg0 string should fail",
			args: []interface{}{"REQUEST", 10},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg0 type should fail",
			args: []interface{}{5, 10},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg1 type should fail",
			args: []interface{}{"request", "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "wrong arg2 type should fail",
			args: []interface{}{"request", 1024.0, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "arg2 below the status range should fail",
			args: []interface{}{"request", 1024.0, 99.0},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "arg2 above the status range should fail",
			args: []interface{}{"request", 1024.0, 600.0},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "more than expected args should fail",
			args: []interface{}{"request", 1024.0, 500.0, 1.0},
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

func TestLogBodyCreateFilterValid(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []interface{}
	}{
		{
			name: "request without status condition",
			args: []interface{}{"request", 1024.0},
		},
		{
			name: "response without status condition",
			args: []interface{}{"response", 1024.0},
		},
		{
			name: "request with status condition",
			args: []interface{}{"request", 1024.0, 500.0},
		},
		{
			name: "response with status condition",
			args: []interface{}{"response", 1024.0, 500.0},
		},
		{
			name: "lowest valid status",
			args: []interface{}{"request", 1024.0, 100.0},
		},
		{
			name: "highest valid status",
			args: []interface{}{"request", 1024.0, 599.0},
		}} {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewLogBody()
			if _, err := spec.CreateFilter(tt.args); err != nil {
				t.Fatalf("Failed to create filter for args %v: %v", tt.args, err)
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

	t.Run("Request with status condition logs on matching status", func(t *testing.T) {
		be := statusEchoBackend(http.StatusInternalServerError)
		defer be.Close()

		content := "testrequest"
		got, rspBody := runLogBody(t, be.URL, `logBody("request", 1024, 500)`, content, http.StatusInternalServerError)

		if !strings.Contains(got, content) {
			t.Fatalf("Failed to find %q log, got: %q", content, got)
		}

		// buffering the request body must not truncate what the backend receives
		if rspBody != content {
			t.Fatalf("Failed to pass the request body through, got: %q", rspBody)
		}
	})

	t.Run("Request with status condition is silent below the status", func(t *testing.T) {
		be := statusEchoBackend(http.StatusOK)
		defer be.Close()

		content := "testrequest"
		got, rspBody := runLogBody(t, be.URL, `logBody("request", 1024, 500)`, content, http.StatusOK)

		if strings.Contains(got, content) {
			t.Fatalf("Found request body %q in %q", content, got)
		}

		if rspBody != content {
			t.Fatalf("Failed to pass the request body through, got: %q", rspBody)
		}
	})

	t.Run("Response with status condition logs on matching status", func(t *testing.T) {
		be := statusContentBackend(http.StatusBadGateway, strings.Repeat("b", 10))
		defer be.Close()

		got, _ := runLogBody(t, be.URL, `logBody("response", 1024, 500)`, "testrequest", http.StatusBadGateway)

		if !strings.Contains(got, strings.Repeat("b", 10)) {
			t.Fatalf("Failed to find rsp content %q log, got: %q", strings.Repeat("b", 10), got)
		}
	})

	t.Run("Response with status condition is silent below the status", func(t *testing.T) {
		be := statusContentBackend(http.StatusOK, strings.Repeat("b", 10))
		defer be.Close()

		got, rspBody := runLogBody(t, be.URL, `logBody("response", 1024, 500)`, "testrequest", http.StatusOK)

		if strings.Contains(got, strings.Repeat("b", 10)) {
			t.Fatalf("Found response body %q in %q", strings.Repeat("b", 10), got)
		}

		// the response body is still delivered to the client
		if rspBody != strings.Repeat("b", 10) {
			t.Fatalf("Failed to pass the response body through, got: %q", rspBody)
		}
	})
}

// TestLogBodyRequestStatusConditionClientCancel covers the memory question the
// status condition raises: with logBody("request", limit, status) the request
// body cannot be logged while it streams, because the status that decides
// whether to log it is not known until the response phase. It is therefore
// buffered, and a client that disconnects while the backend is still working
// means that buffer is built up for a response phase that never runs.
//
// Two properties make that safe, and each is asserted below:
//
//  1. the buffer is capped by the filter's own limit argument, not by the size
//     of the request body, so a large upload cannot inflate it; and
//  2. the buffer is reachable only from the request's state bag, so it is
//     released with the request rather than accumulating across cancellations.
func TestLogBodyRequestStatusConditionClientCancel(t *testing.T) {
	defer log.SetOutput(os.Stderr)

	const (
		limit         = 1024
		bodySize      = 256 * 1024
		backendSleep  = 500 * time.Millisecond
		clientTimeout = 20 * time.Millisecond
	)

	// The backend drains the request body and then sleeps well past the client
	// timeout, so the client always disconnects before a response status exists.
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(backendSleep)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer be.Close()

	fr := make(filters.Registry)
	fr.Register(NewLogBody())
	routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request", %d, 500) -> "%s"`, limit, be.URL))
	p := proxytest.New(fr, routes...)
	defer p.Close()

	body := strings.Repeat("a", bodySize)

	// postAndCancel issues a POST that gives up while the backend is sleeping.
	// It returns the error the client saw, which must not be nil for the
	// cancellation assertions below to mean anything.
	postAndCancel := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", p.URL, strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "text/plain")

		rsp, err := p.Client().Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, rsp.Body)
		rsp.Body.Close()
		return nil
	}

	t.Run("the buffered body is capped by the limit, not the body size", func(t *testing.T) {
		// Drive the stream exactly as Request() does when a status condition
		// is set: the callback appends to a buffer instead of logging.
		buf := &bytes.Buffer{}
		stream := newLogBodyStream(
			limit,
			func(chunk []byte) { buf.Write(chunk) },
			io.NopCloser(strings.NewReader(body)),
		)

		n, err := io.Copy(io.Discard, stream)
		if err != nil {
			t.Fatalf("Failed to read the body through the stream: %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("Failed to close the stream: %v", err)
		}

		// The body still passes through in full ...
		if n != bodySize {
			t.Fatalf("Failed to pass the whole body through, want %d bytes, got: %d", bodySize, n)
		}
		// ... while only limit bytes of it are ever retained.
		if buf.Len() != limit {
			t.Fatalf("Failed to cap the buffered body at %d bytes, got: %d", limit, buf.Len())
		}
	})

	t.Run("nothing is logged when the client cancels before the status is known", func(t *testing.T) {
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		err := postAndCancel()
		log.SetOutput(os.Stderr)

		if err == nil {
			t.Fatalf("Failed to cancel before the backend responded, expected a client error")
		}

		// Logging is deferred to the response phase, which never sees a status
		// here, so the buffered body must not reach the log.
		if got := logbuf.String(); strings.Contains(got, strings.Repeat("a", limit)) {
			t.Fatalf("Found the buffered request body in the log after cancellation, got: %q", got)
		}
	})

	t.Run("repeated cancellations do not accumulate buffers", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping the allocation check in short mode")
		}

		const iterations = 50

		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)

		// Warm up so that one-off proxy and transport allocations are not
		// counted as growth, then measure across the cancelled requests.
		if err := postAndCancel(); err == nil {
			t.Fatalf("Failed to cancel before the backend responded, expected a client error")
		}

		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		for i := 0; i < iterations; i++ {
			if err := postAndCancel(); err == nil {
				t.Fatalf("Failed to cancel before the backend responded on iteration %d", i)
			}
		}

		runtime.GC()
		runtime.ReadMemStats(&after)

		// Retaining one buffer per cancelled request would grow the heap by
		// iterations*limit; retaining the whole body would grow it by
		// iterations*bodySize. The threshold sits far below the latter and well
		// above ordinary test noise, so it fails on a real leak without being
		// sensitive to allocation churn.
		const threshold = iterations * bodySize / 8

		var growth uint64
		if after.HeapAlloc > before.HeapAlloc {
			growth = after.HeapAlloc - before.HeapAlloc
		}
		if growth > threshold {
			t.Fatalf("Failed to release the buffered bodies, heap grew by %d bytes over %d cancelled requests, want at most %d",
				growth, iterations, uint64(threshold))
		}
	})
}

// statusEchoBackend responds with the given status and echoes the request
// body back, so that a test can assert the backend received it unchanged.
func statusEchoBackend(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
}

// statusContentBackend drains the request body and responds with the given
// status and a fixed response body.
func statusContentBackend(status int, content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(status)
		w.Write([]byte(content))
	}))
}

// runLogBody proxies a POST with the given filter to beURL and returns what
// was logged and the response body received by the client.
func runLogBody(t *testing.T, beURL, filter, content string, wantStatus int) (string, string) {
	t.Helper()

	fr := make(filters.Registry)
	fr.Register(NewLogBody())

	routes := eskip.MustParse(fmt.Sprintf(`r: * -> %s -> "%s"`, filter, beURL))
	p := proxytest.New(fr, routes...)
	defer p.Close()

	logbuf := bytes.NewBuffer(nil)
	log.SetOutput(logbuf)
	rsp, err := p.Client().Post(p.URL, "text/plain", bytes.NewBufferString(content))
	if err != nil {
		log.SetOutput(os.Stderr)
		t.Fatalf("Failed to POST: %v", err)
	}

	rspBuf := bytes.NewBuffer(nil)
	io.Copy(rspBuf, rsp.Body)
	rsp.Body.Close()
	log.SetOutput(os.Stderr)

	if rsp.StatusCode != wantStatus {
		t.Fatalf("Failed to get status %d, got: %d", wantStatus, rsp.StatusCode)
	}

	return logbuf.String(), rspBuf.String()
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

		lg := func(format string, args ...interface{}) {
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

		lg := func(format string, args ...interface{}) {
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

		lg := func(format string, args ...interface{}) {
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

		lg := func(format string, args ...interface{}) {
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

		lg := func(format string, args ...interface{}) {
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
