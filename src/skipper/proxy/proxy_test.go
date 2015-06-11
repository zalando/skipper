package proxy

import "testing"
import "net/http"
import "net/http/httptest"
import "github.com/mailgun/route"
import "strconv"
import "net/url"
import "bytes"
import "skipper/settings"
import "io"
import "time"

const streamingDelay time.Duration = 3 * time.Millisecond

type requestCheck func(*http.Request)

func voidCheck(*http.Request){}

type settingsSource struct {
	get      chan settings.Settings
	settings *testSettings
}

type testSettings struct {
	routerImpl route.Router
}

type testBackend struct {
	url string
}

func makeTestSettingsSource(url string) *settingsSource {
	rt := route.New()
	tb := &testBackend{url}
	rt.AddRoute("Path(\"/hello/<v>\")", tb)

	ss := &settingsSource{
		make(chan settings.Settings),
		&testSettings{rt}}

	go func() {
		for {
			ss.get <- ss.settings
		}
	}()

	return ss
}

func (s *settingsSource) Get() <-chan settings.Settings {
	return s.get
}

func (ts *testSettings) Route(r *http.Request) (settings.Backend, error) {
	b, err := ts.routerImpl.Route(r)
	if b == nil || err != nil {
		return nil, err
	}

	return b.(settings.Backend), nil
}

func (tb *testBackend) Url() string {
	return tb.url
}

func writeParts(w io.Writer, parts int, data []byte) {
    partSize := len(data) / parts
    for i := 0; i < len(data); i += partSize {
        w.Write(data[i:i + partSize])
        time.Sleep(streamingDelay)
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    }
    w.Write(data[:len(data) - len(data) % parts])
}

func startTestServer(payload []byte, parts int, check requestCheck) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)

		if len(payload) <= 0 {
            return
		}

        w.Header().Set("Content-Type", "text/plain")
        w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(200)

        if parts > 0 {
            writeParts(w, parts, payload)
            return
        }

        w.Write(payload)
	}))
}

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

	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()
	p := Make(makeTestSettingsSource(s.URL))
	p.ServeHTTP(w, r)

	if w.Code != 200 {
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

	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
	r := &http.Request{
		URL:    u,
		Method: "POST",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()
	p := Make(makeTestSettingsSource(s.URL))
	p.ServeHTTP(w, r)

	if w.Code != 200 {
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

    ss := makeTestSettingsSource("")
    ss.settings.routerImpl.AddRoute("Path(\"/host-one<any>\")", &testBackend{s1.URL})
    ss.settings.routerImpl.AddRoute("Path(\"/host-two<any>\")", &testBackend{s2.URL})

	p := Make(ss)

    var (
        r *http.Request
        w *httptest.ResponseRecorder
        u *url.URL
    )

    u, _ = url.ParseRequestURI("http://localhost:9090/host-one")
    r = &http.Request{
        URL: u,
        Method: "GET"}
    w = httptest.NewRecorder()
    p.ServeHTTP(w, r)
    if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload1) {
        t.Error("wrong routing 1")
    }

    u, _ = url.ParseRequestURI("http://localhost:9090/host-two")
    r = &http.Request{
        URL: u,
        Method: "GET"}
    w = httptest.NewRecorder()
    p.ServeHTTP(w, r)
    if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload2) {
        t.Error("wrong routing 2")
    }
}

func TestStreaming(t *testing.T) {
    const expectedParts = 3

    payload := []byte("some data to stream")
    s := startTestServer(payload, expectedParts, voidCheck)
    defer s.Close()

    ss := makeTestSettingsSource(s.URL)
    p := Make(ss)

    u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
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
