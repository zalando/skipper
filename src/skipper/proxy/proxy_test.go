package proxy

import "testing"
import "net/http"
import "net/http/httptest"
import "github.com/mailgun/route"
import "strconv"
import "net/url"
import "bytes"
import "skipper/settings"

type requestCheck func(*http.Request)

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

func makeTestSettingsSource(url string) settings.Source {
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

func startTestServer(payload []byte, check requestCheck) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)
		if len(payload) > 0 {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		}

		w.WriteHeader(200)
		w.Write(payload)
	}))
}

func TestGetRoundtrip(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, func(r *http.Request) {
		if r.Method != "GET" {
			t.Error("wrong request method")
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}
	})
	defer s.Close()

	url, _ := url.ParseRequestURI("http://localhost:9090/hello/")
	r := &http.Request{
		URL:    url,
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
	s := startTestServer(nil, func(r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong request method", r.Method)
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}
	})
	defer s.Close()

	url, _ := url.ParseRequestURI("http://localhost:9090/hello/")
	r := &http.Request{
		URL:    url,
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
