package proxy

import (
	"bytes"
    "github.com/zalando/skipper/settings"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
    "fmt"
)

const streamingDelay time.Duration = 3 * time.Millisecond

type dataClient struct {
	FReceive chan string
}

func makeDataClient(data string) *dataClient {
	dc := &dataClient{make(chan string)}
	dc.feed(data)
	return dc
}

func (dc *dataClient) Receive() <-chan string {
	return dc.FReceive
}

func (dc *dataClient) feed(data string) {
	go func() {
		dc.FReceive <- data
	}()
}

type requestCheck func(*http.Request)

// type settingsSource struct {
// 	settings *settings.Settings
// }
// 
// func (ss *settingsSource) Subscribe(c chan<- *settings.Settings) {
// 	go func() {
// 		for {
// 			c <- ss.settings
// 		}
// 	}()
// }
// 
// func voidCheck(*http.Request) {}

// func makeTestSettingsDispatcher(url string, filters []filters.Filter, shunt bool) *settingsSource {
// 	return &settingsSource{mock.MakeSettings(url, filters, shunt)}
// }

// func makeEmptySettingsDispatcher() *settingsSource {
// 	return &settingsSource{}
// }

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
		w.WriteHeader(200)

		if parts > 0 {
			writeParts(w, parts, payload)
			return
		}

		w.Write(payload)
	}))
}

// func urlToBackend(u string) *mock.Backend {
// 	up, _ := url.ParseRequestURI(u)
// 	return &mock.Backend{up.Scheme, up.Host, false}
// }

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
	p := Make(settings.MakeSource(makeDataClient(data), nil, false), false)

    // wait for settings dispatched
    time.Sleep(15 * time.Millisecond)
    time.Sleep(15 * time.Millisecond)

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

// func TestPostRoundtrip(t *testing.T) {
// 	s := startTestServer(nil, 0, func(r *http.Request) {
// 		if r.Method != "POST" {
// 			t.Error("wrong request method", r.Method)
// 		}
// 
// 		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
// 			t.Error("wrong request header")
// 		}
// 	})
// 	defer s.Close()
// 
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "POST",
// 		Header: http.Header{"X-Test-Header": []string{"test value"}}}
// 	w := httptest.NewRecorder()
// 	p := Make(makeTestSettingsDispatcher(s.URL, nil, false), false)
// 	p.ServeHTTP(w, r)
// 
// 	if w.Code != 200 {
// 		t.Error("wrong status", w.Code)
// 	}
// 
// 	if w.Body.Len() != 0 {
// 		t.Error("wrong content", string(w.Body.Bytes()))
// 	}
// }
// 
// func TestRoute(t *testing.T) {
// 	payload1 := []byte("host one")
// 	s1 := startTestServer(payload1, 0, voidCheck)
// 	defer s1.Close()
// 
// 	payload2 := []byte("host two")
// 	s2 := startTestServer(payload2, 0, voidCheck)
// 	defer s2.Close()
// 
// 	sd := makeTestSettingsDispatcher("", nil, false)
// 	ts := mock.MakeSettings("", nil, false)
// 	ts.RouterImpl.AddRoute("Path(\"/host-one<any>\")", &mock.Route{urlToBackend(s1.URL), nil})
// 	ts.RouterImpl.AddRoute("Path(\"/host-two<any>\")", &mock.Route{urlToBackend(s2.URL), nil})
// 
// 	// unsynchronized access for the test, fine here
// 	sd.settings = ts
// 
// 	p := Make(sd, false)
// 
// 	var (
// 		r *http.Request
// 		w *httptest.ResponseRecorder
// 		u *url.URL
// 	)
// 
// 	u, _ = url.ParseRequestURI("http://localhost:9090/host-one")
// 	r = &http.Request{
// 		URL:    u,
// 		Method: "GET"}
// 	w = httptest.NewRecorder()
// 	p.ServeHTTP(w, r)
// 	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload1) {
// 		t.Error("wrong routing 1")
// 	}
// 
// 	u, _ = url.ParseRequestURI("http://localhost:9090/host-two")
// 	r = &http.Request{
// 		URL:    u,
// 		Method: "GET"}
// 	w = httptest.NewRecorder()
// 	p.ServeHTTP(w, r)
// 	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload2) {
// 		t.Error("wrong routing 2")
// 	}
// }
// 
// func TestStreaming(t *testing.T) {
// 	const expectedParts = 3
// 
// 	payload := []byte("some data to stream")
// 	s := startTestServer(payload, expectedParts, voidCheck)
// 	defer s.Close()
// 
// 	p := Make(makeTestSettingsDispatcher(s.URL, nil, false), false)
// 
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "GET"}
// 	w := httptest.NewRecorder()
// 
// 	parts := 0
// 	total := 0
// 	done := make(chan int)
// 	go p.ServeHTTP(w, r)
// 	go func() {
// 		for {
// 			buf := w.Body.Bytes()
// 
// 			if len(buf) == 0 {
// 				time.Sleep(streamingDelay)
// 				continue
// 			}
// 
// 			parts++
// 			total += len(buf)
// 
// 			if total >= len(payload) {
// 				close(done)
// 				return
// 			}
// 		}
// 	}()
// 
// 	select {
// 	case <-done:
// 		if parts <= expectedParts {
// 			t.Error("streaming failed", parts)
// 		}
// 	case <-time.After(150 * time.Millisecond):
// 		t.Error("streaming timeout")
// 	}
// }
// 
// func TestNotFoundUntilSettingsReceived(t *testing.T) {
// 	payload := []byte("Hello World!")
// 
// 	s := startTestServer(payload, 0, func(r *http.Request) {
// 		t.Error("shouldn't be able to route to here")
// 	})
// 	defer s.Close()
// 
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "GET",
// 		Header: http.Header{"X-Test-Header": []string{"test value"}}}
// 	w := httptest.NewRecorder()
// 	p := Make(makeEmptySettingsDispatcher(), false)
// 	p.ServeHTTP(w, r)
// 
// 	if w.Code != 404 {
// 		t.Error("wrong status", w.Code)
// 	}
// }
// 
// func TestAppliesFilters(t *testing.T) {
// 	payload := []byte("Hello World!")
// 
// 	s := startTestServer(payload, 0, func(r *http.Request) {
// 		if h, ok := r.Header["X-Test-Request-Header-0"]; !ok ||
// 			h[0] != "request header value 0" {
// 			t.Error("request header 0 is missing")
// 		}
// 
// 		if h, ok := r.Header["X-Test-Request-Header-1"]; !ok ||
// 			h[0] != "request header value 1" {
// 			t.Error("request header 1 is missing")
// 		}
// 	})
// 	defer s.Close()
// 
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "GET",
// 		Header: http.Header{"X-Test-Header": []string{"test value"}}}
// 	w := httptest.NewRecorder()
// 	p := Make(makeTestSettingsDispatcher(s.URL, []filters.Filter{
// 		&mock.Filter{
// 			RequestHeaders:  map[string]string{"X-Test-Request-Header-0": "request header value 0"},
// 			ResponseHeaders: map[string]string{"X-Test-Response-Header-0": "response header value 0"}},
// 		&mock.Filter{
// 			RequestHeaders:  map[string]string{"X-Test-Request-Header-1": "request header value 1"},
// 			ResponseHeaders: map[string]string{"X-Test-Response-Header-1": "response header value 1"}}}, false), false)
// 
// 	p.ServeHTTP(w, r)
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-0"]; !ok || h[0] != "response header value 0" {
// 		t.Error("missing response header 0")
// 	}
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-1"]; !ok || h[0] != "response header value 1" {
// 		t.Error("missing response header 1")
// 	}
// }
// 
// func TestAppliesFiltersInOrder(t *testing.T) {
// 	payload := []byte("Hello World!")
// 
// 	s := startTestServer(payload, 0, func(r *http.Request) {
// 		if h, ok := r.Header["X-Test-Request-Header-0"]; !ok ||
// 			h[0] != "request header value 1" {
// 			t.Error("request header 0 is wrong")
// 		}
// 
// 		if h, ok := r.Header["X-Test-Request-Header-1"]; !ok ||
// 			h[0] != "request header value 1" {
// 			t.Error("request header 1 is missing")
// 		}
// 	})
// 	defer s.Close()
// 
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "GET",
// 		Header: http.Header{"X-Test-Header": []string{"test value"}}}
// 	w := httptest.NewRecorder()
// 	p := Make(makeTestSettingsDispatcher(s.URL, []filters.Filter{
// 		&mock.Filter{
// 			RequestHeaders: map[string]string{
// 				"X-Test-Request-Header-0": "request header value 0"},
// 			ResponseHeaders: map[string]string{
// 				"X-Test-Response-Header-0": "response header value 0",
// 				"X-Test-Response-Header-1": "response header value 0"}},
// 		&mock.Filter{
// 			RequestHeaders: map[string]string{
// 				"X-Test-Request-Header-0": "request header value 1",
// 				"X-Test-Request-Header-1": "request header value 1"},
// 			ResponseHeaders: map[string]string{
// 				"X-Test-Response-Header-1": "response header value 1"}}}, false), false)
// 
// 	p.ServeHTTP(w, r)
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-0"]; !ok || h[0] != "response header value 0" {
// 		t.Error("wrong response header 0")
// 	}
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-1"]; !ok || h[0] != "response header value 0" {
// 		t.Error("wrong response header 1")
// 	}
// }
// 
// func TestProcessesRequestWithShuntBackend(t *testing.T) {
// 	u, _ := url.ParseRequestURI("http://localhost:9090/hello/")
// 	r := &http.Request{
// 		URL:    u,
// 		Method: "GET",
// 		Header: http.Header{"X-Test-Header": []string{"test value"}}}
// 	w := httptest.NewRecorder()
// 	p := Make(makeTestSettingsDispatcher("", []filters.Filter{
// 		&mock.Filter{
// 			ResponseHeaders: map[string]string{
// 				"X-Test-Response-Header-0": "response header value 0"}},
// 		&mock.Filter{
// 			ResponseHeaders: map[string]string{
// 				"X-Test-Response-Header-1": "response header value 1"}}}, true), false)
// 
// 	p.ServeHTTP(w, r)
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-0"]; !ok || h[0] != "response header value 0" {
// 		t.Error("wrong response header 0")
// 	}
// 
// 	if h, ok := w.Header()["X-Test-Response-Header-1"]; !ok || h[0] != "response header value 1" {
// 		t.Error("wrong response header 1")
// 	}
// }
