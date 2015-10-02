package proxy

import (
	"bytes"
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const streamingDelay time.Duration = 3 * time.Millisecond

type requestCheck func(*http.Request)

func voidCheck(*http.Request) {}

func writeParts(w io.Writer, parts int, data []byte) {
	partSize := len(data) / parts
	for i := 0; i < len(data); i += partSize {
		w.Write(data[i : i+partSize])
		time.Sleep(streamingDelay)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	w.Write(data[:len(data)-len(data)%parts])
}

func startTestServer(payload []byte, parts int, check requestCheck) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)

		if len(payload) <= 0 {
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)

		if parts > 0 {
			writeParts(w, parts, payload)
			return
		}

		w.Write(payload)
	}))
}

// used to let the data client updates be propagated
func delay() { time.Sleep(12 * time.Millisecond) }

func TestGetRoundtrip(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, 0, func(r *http.Request) {
		if r.Method != "GET" {
			t.Error("wrong request method")
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}
	})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()

	data := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	p := Make(routing.New(testdataclient.New(data), nil, false), false)
	delay()

	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	if ct, ok := w.Header()["Content-Type"]; !ok || ct[0] != "text/plain" {
		t.Error("wrong content type")
	}

	if cl, ok := w.Header()["Content-Length"]; !ok || cl[0] != strconv.Itoa(len(payload)) {
		t.Error("wrong content length")
	}

	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Error("wrong content", string(w.Body.Bytes()))
	}
}

func TestPostRoundtrip(t *testing.T) {
	s := startTestServer(nil, 0, func(r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong request method", r.Method)
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}
	})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "POST",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()

	data := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	p := Make(routing.New(testdataclient.New(data), nil, false), false)
	delay()

	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	if w.Body.Len() != 0 {
		t.Error("wrong content", string(w.Body.Bytes()))
	}
}

func TestRoute(t *testing.T) {
	payload1 := []byte("host one")
	s1 := startTestServer(payload1, 0, voidCheck)
	defer s1.Close()

	payload2 := []byte("host two")
	s2 := startTestServer(payload2, 0, voidCheck)
	defer s2.Close()

	data := fmt.Sprintf(`
        route1: Path("/host-one/*any") -> "%s";
        route1: Path("/host-two/*any") -> "%s"
    `, s1.URL, s2.URL)
	routing := routing.New(testdataclient.New(data), nil, false)
	p := Make(routing, false)
	delay()

	var (
		r *http.Request
		w *httptest.ResponseRecorder
		u *url.URL
	)

	u, _ = url.ParseRequestURI("https://www.example.org/host-one/some/path")
	r = &http.Request{
		URL:    u,
		Method: "GET"}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), payload1) {
		t.Error("wrong routing 1")
	}

	u, _ = url.ParseRequestURI("https://www.example.org/host-two/some/path")
	r = &http.Request{
		URL:    u,
		Method: "GET"}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), payload2) {
		t.Error("wrong routing 2")
	}
}

func TestStreaming(t *testing.T) {
	const expectedParts = 3

	payload := []byte("some data to stream")
	s := startTestServer(payload, expectedParts, voidCheck)
	defer s.Close()

	data := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	p := Make(routing.New(testdataclient.New(data), nil, false), false)
	delay()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET"}
	w := httptest.NewRecorder()

	parts := 0
	total := 0
	done := make(chan int)
	go p.ServeHTTP(w, r)
	go func() {
		for {
			buf := w.Body.Bytes()

			if len(buf) == 0 {
				time.Sleep(streamingDelay)
				continue
			}

			parts++
			total += len(buf)

			if total >= len(payload) {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		if parts <= expectedParts {
			t.Error("streaming failed", parts)
		}
	case <-time.After(150 * time.Millisecond):
		t.Error("streaming timeout")
	}
}

func TestAppliesFilters(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, 0, func(r *http.Request) {
		if h, ok := r.Header["X-Test-Request-Header"]; !ok ||
			h[0] != "request header value" {
			t.Error("request header is missing")
		}
	})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()

	fr := make(filters.Registry)
	reqh := filters.CreateRequestHeader()
	rsph := filters.CreateResponseHeader()
	fr[reqh.Name()] = reqh
	fr[rsph.Name()] = rsph

	data := fmt.Sprintf(`hello:
        Path("/hello") ->
        requestHeader("X-Test-Request-Header", "request header value") ->
        responseHeader("X-Test-Response-Header", "response header value") ->
        "%s"`, s.URL)
	p := Make(routing.New(testdataclient.New(data), fr, false), false)
	delay()

	p.ServeHTTP(w, r)

	if h, ok := w.Header()["X-Test-Response-Header"]; !ok || h[0] != "response header value" {
		t.Error("missing response header")
	}
}

func TestProcessesRequestWithShuntBackend(t *testing.T) {
	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()

	fr := make(filters.Registry)
	rsph := filters.CreateResponseHeader()
	fr[rsph.Name()] = rsph

	data := `hello: Path("/hello") -> responseHeader("X-Test-Response-Header", "response header value") -> <shunt>`
	p := Make(routing.New(testdataclient.New(data), fr, false), false)
	delay()

	p.ServeHTTP(w, r)

	if h, ok := w.Header()["X-Test-Response-Header"]; !ok || h[0] != "response header value" {
		t.Error("wrong response header")
	}
}
