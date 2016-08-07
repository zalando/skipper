package proxy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

const (
	streamingDelay    time.Duration = 30 * time.Millisecond
	sourcePollTimeout time.Duration = 6 * time.Millisecond
)

type requestCheck func(*http.Request)

type priorityRoute struct {
	route  *routing.Route
	params map[string]string
	match  func(r *http.Request) bool
}

type (
	preserveOriginalSpec   struct{}
	preserveOriginalFilter struct{}
)

type syncResponseWriter struct {
	mx         sync.Mutex
	statusCode int
	header     http.Header
	body       *bytes.Buffer
}

func hasArg(arg string) bool {
	for _, a := range os.Args {
		if a == arg {
			return true
		}
	}

	return false
}

func (cors *preserveOriginalSpec) Name() string { return "preserveOriginal" }

func (cors *preserveOriginalSpec) CreateFilter(_ []interface{}) (filters.Filter, error) {
	return &preserveOriginalFilter{}, nil
}

func preserveHeader(from, to http.Header) {
	for key, vals := range from {
		to[key+"-Preserved"] = vals
	}
}

func (corf *preserveOriginalFilter) Request(ctx filters.FilterContext) {
	preserveHeader(ctx.OriginalRequest().Header, ctx.Request().Header)
}

func (corf *preserveOriginalFilter) Response(ctx filters.FilterContext) {
	preserveHeader(ctx.OriginalResponse().Header, ctx.Response().Header)
}

func (prt *priorityRoute) Match(r *http.Request) (*routing.Route, map[string]string) {
	if prt.match(r) {
		return prt.route, prt.params
	}

	return nil, nil
}

func newSyncResponseWriter() *syncResponseWriter {
	return &syncResponseWriter{header: make(http.Header), body: bytes.NewBuffer(nil)}
}

func (srw *syncResponseWriter) Header() http.Header {
	return srw.header
}

func (srw *syncResponseWriter) WriteHeader(statusCode int) {
	srw.statusCode = statusCode
}

func (srw *syncResponseWriter) Write(b []byte) (int, error) {
	srw.mx.Lock()
	defer srw.mx.Unlock()
	return srw.body.Write(b)
}

func (srw *syncResponseWriter) Read(b []byte) (int, error) {
	srw.mx.Lock()
	defer srw.mx.Unlock()
	return srw.body.Read(b)
}

func (srw *syncResponseWriter) Flush() {}

func (srw *syncResponseWriter) Len() int {
	srw.mx.Lock()
	defer srw.mx.Unlock()
	return srw.body.Len()
}

func voidCheck(*http.Request) {}

func writeParts(w io.Writer, parts int, data []byte) {
	partSize := len(data) / parts
	i := 0
	for ; i+partSize <= len(data); i += partSize {
		w.Write(data[i : i+partSize])
		time.Sleep(streamingDelay)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	w.Write(data[i:])
}

func startTestServer(payload []byte, parts int, check requestCheck) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)

		w.Header().Set("X-Test-Response-Header", "response header value")

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

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	if ct, ok := w.Header()["Content-Type"]; !ok || ct[0] != "text/plain" {
		t.Errorf("wrong content type. Expected 'text/plain' but got '%s'", w.Header().Get("Content-Type"))
	}

	if cl, ok := w.Header()["Content-Length"]; !ok || cl[0] != strconv.Itoa(len(payload)) {
		t.Error("wrong content length")
	}

	if xpb, ok := w.Header()["X-Powered-By"]; !ok || xpb[0] != "Skipper" {
		t.Error("Wrong X-Powered-By header value")
	}

	if xpb, ok := w.Header()["Server"]; !ok || xpb[0] != "Skipper" {
		t.Error("Wrong Server header value")
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

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

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

	doc := fmt.Sprintf(`
		route1: Path("/host-one/*any") -> "%s";
		route2: Path("/host-two/*any") -> "%s"
	`, s1.URL, s2.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

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

// This test is sensitive for timing, and occasionally fails.
// To run this test, set `-args stream` for the test command.
func TestStreaming(t *testing.T) {
	if !hasArg("stream") {
		t.Skip()
	}

	const expectedParts = 3

	payload := []byte("some data to stream")
	s := startTestServer(payload, expectedParts, voidCheck)
	defer s.Close()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET"}
	w := newSyncResponseWriter()

	parts := 0
	total := 0
	done := make(chan int)
	go p.ServeHTTP(w, r)
	go func() {
		readPayload := make([]byte, len(payload))
		for {
			n, err := w.Read(readPayload)
			if err != nil && err != io.EOF {
				close(done)
				return
			}

			if n == 0 {
				time.Sleep(streamingDelay)
				continue
			}

			readPayload = readPayload[n:]
			parts++
			total += n

			if len(readPayload) == 0 {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		if parts < expectedParts {
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
	fr.Register(builtin.NewRequestHeader())
	fr.Register(builtin.NewResponseHeader())

	doc := fmt.Sprintf(`hello:
		Path("/hello") ->
		requestHeader("X-Test-Request-Header", "request header value") ->
		responseHeader("X-Test-Response-Header", "response header value") ->
		"%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		Log:            tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	p.ServeHTTP(w, r)

	if h, ok := w.Header()["X-Test-Response-Header"]; !ok || h[0] != "response header value" {
		t.Error("missing response header")
	}
}

type breaker struct {
	resp *http.Response
}

func (b *breaker) Request(c filters.FilterContext)                       { c.Serve(b.resp) }
func (_ *breaker) Response(filters.FilterContext)                        {}
func (b *breaker) CreateFilter(fc []interface{}) (filters.Filter, error) { return b, nil }
func (_ *breaker) Name() string                                          { return "breaker" }

func TestBreakFilterChain(t *testing.T) {
	s := startTestServer([]byte("Hello World!"), 0, func(r *http.Request) {
		t.Error("This should never be called")
	})
	defer s.Close()

	fr := make(filters.Registry)
	fr.Register(builtin.NewRequestHeader())
	resp1 := &http.Response{
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(new(bytes.Buffer)),
		StatusCode: http.StatusUnauthorized,
		Status:     "Impossible body",
	}
	fr.Register(&breaker{resp1})
	fr.Register(builtin.NewResponseHeader())

	doc := fmt.Sprintf(`breakerDemo:
		Path("/breaker") ->
		requestHeader("X-Expected", "request header") ->
		responseHeader("X-Expected", "response header") ->
		breaker() ->
		requestHeader("X-Unexpected", "foo") ->
		responseHeader("X-Unexpected", "bar") ->
		"%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		Log:            tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	r, _ := http.NewRequest("GET", "https://www.example.org/breaker", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if _, has := r.Header["X-Expected"]; !has {
		t.Error("Request is missing the expected header (added during filter chain winding)")
	}

	if _, has := w.Header()["X-Expected"]; !has {
		t.Error("Response is missing the expected header (added during filter chain unwinding)")
	}

	if _, has := r.Header["X-Unexpected"]; has {
		t.Error("Request has an unexpected header from a filter after the breaker in the chain")
	}

	if _, has := w.Header()["X-Unexpected"]; has {
		t.Error("Response has an unexpected header from a filter after the breaker in the chain")
	}

	if w.Code != http.StatusUnauthorized && w.Body.String() != "Impossible body" {
		t.Errorf("Wrong status code/body. Expected 401 - Impossible body but got %d - %s", w.Code, w.Body.String())
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
	fr.Register(builtin.NewResponseHeader())

	doc := `hello: Path("/hello") -> responseHeader("X-Test-Response-Header", "response header value") -> <shunt>`
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		Log:            tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	p.ServeHTTP(w, r)

	if h, ok := w.Header()["X-Test-Response-Header"]; !ok || h[0] != "response header value" {
		t.Error("wrong response header")
	}
}

func TestProcessesRequestWithPriorityRoute(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "test-value")
	}))
	defer s.Close()

	req, err := http.NewRequest(
		"GET",
		"https://example.org",
		nil)
	if err != nil {
		t.Error(err)
	}

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Error(err)
	}

	prt := &priorityRoute{&routing.Route{Scheme: u.Scheme, Host: u.Host}, nil, func(r *http.Request) bool {
		return r == req
	}}

	doc := `hello: Path("/hello") -> <shunt>`
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone, PriorityRoutes: []PriorityRoute{prt}})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)
	if w.Header().Get("X-Test-Header") != "test-value" {
		t.Error("failed match priority route")
	}
}

func TestProcessesRequestWithPriorityRouteOverStandard(t *testing.T) {
	s0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "priority-value")
	}))
	defer s0.Close()

	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "normal-value")
	}))
	defer s0.Close()

	req, err := http.NewRequest(
		"GET",
		"https://example.org/hello/world",
		nil)
	if err != nil {
		t.Error(err)
	}

	u, err := url.Parse(s0.URL)
	if err != nil {
		t.Error(err)
	}

	prt := &priorityRoute{&routing.Route{Scheme: u.Scheme, Host: u.Host}, nil, func(r *http.Request) bool {
		return r == req
	}}

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s1.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone, PriorityRoutes: []PriorityRoute{prt}})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)
	if w.Header().Get("X-Test-Header") != "priority-value" {
		t.Error("failed match priority route")
	}
}

func TestFlusherImplementation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, "))
		time.Sleep(15 * time.Millisecond)
		w.Write([]byte("world!"))
	})

	ts := httptest.NewServer(h)
	defer ts.Close()

	doc := fmt.Sprintf(`* -> "%s"`, ts.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	rt := routing.New(routing.Options{
		PollTimeout: sourcePollTimeout,
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: FlagsNone})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	a := fmt.Sprintf(":%d", 1<<16-rand.Intn(1<<15))
	ps := &http.Server{Addr: a, Handler: p}
	go ps.ListenAndServe()

	// let the server start listening
	time.Sleep(15 * time.Millisecond)

	rsp, err := http.Get("http://127.0.0.1" + a)
	if err != nil {
		t.Error(err)
		return
	}
	defer rsp.Body.Close()
	b, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		t.Error(err)
		return
	}
	if string(b) != "Hello, world!" {
		t.Error("failed to receive response")
	}
}

func TestOriginalRequestResponse(t *testing.T) {
	s := startTestServer(nil, 0, func(r *http.Request) {
		if th, ok := r.Header["X-Test-Header-Preserved"]; !ok || th[0] != "test value" {
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

	doc := fmt.Sprintf(`hello: Path("/hello") -> preserveOriginal() -> "%s"`, s.URL)
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Error(err)
	}

	tl := loggingtest.New()
	defer tl.Close()

	fr := builtin.MakeRegistry()
	fr.Register(&preserveOriginalSpec{})
	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		Log:            tl})
	defer rt.Close()

	p := WithParams(Params{Routing: rt, Flags: PreserveOriginal})
	defer p.Close()

	tl.WaitFor("route settings applied", time.Second)

	p.ServeHTTP(w, r)

	if th, ok := w.Header()["X-Test-Response-Header-Preserved"]; !ok || th[0] != "response header value" {
		t.Error("wrong response header", ok)
	}
}

func TestHostHeader(t *testing.T) {
	// start a test backend that returns the received host header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Host", r.Host)
	}))
	defer backend.Close()

	// take the generated host part of the backend
	bu, err := url.Parse(backend.URL)
	if err != nil {
		t.Error("failed to parse test backend url")
		return
	}
	backendHost := bu.Host

	for _, ti := range []struct {
		msg          string
		flags        Flags
		routeFmt     string
		incomingHost string
		expectedHost string
	}{{
		"no proxy preserve",
		FlagsNone,
		`route: Any() -> "%s"`,
		"www.example.org",
		backendHost,
	}, {
		"no proxy preserve, route preserve not",
		FlagsNone,
		`route: Any() -> preserveHost("false") -> "%s"`,
		"www.example.org",
		backendHost,
	}, {
		"no proxy preserve, route preserve",
		FlagsNone,
		`route: Any() -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"www.example.org",
	}, {
		"no proxy preserve, route preserve not, explicit host last",
		FlagsNone,
		`route: Any() -> preserveHost("false") -> requestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve, explicit host last",
		FlagsNone,
		`route: Any() -> preserveHost("true") -> requestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve not, explicit host first",
		FlagsNone,
		`route: Any() -> requestHeader("Host", "custom.example.org") -> preserveHost("false") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve, explicit host last",
		FlagsNone,
		`route: Any() -> requestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve",
		PreserveHost,
		`route: Any() -> "%s"`,
		"www.example.org",
		"www.example.org",
	}, {
		"proxy preserve, route preserve not",
		PreserveHost,
		`route: Any() -> preserveHost("false") -> "%s"`,
		"www.example.org",
		backendHost,
	}, {
		"proxy preserve, route preserve",
		PreserveHost,
		`route: Any() -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"www.example.org",
	}, {
		"proxy preserve, route preserve not, explicit host last",
		PreserveHost,
		`route: Any() -> preserveHost("false") -> requestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve, explicit host last",
		PreserveHost,
		`route: Any() -> preserveHost("true") -> requestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve not, explicit host first",
		PreserveHost,
		`route: Any() -> requestHeader("Host", "custom.example.org") -> preserveHost("false") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve, explicit host last",
		PreserveHost,
		`route: Any() -> requestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"debug proxy, route not found",
		PreserveHost | Debug,
		`route: Path("/hello") -> requestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"",
	}, {
		"debug proxy, shunt route",
		PreserveHost | Debug,
		`route: Any() -> <shunt>`,
		"www.example.org",
		"",
	}, {
		"debug proxy, full circle",
		PreserveHost | Debug,
		`route: Any() -> requestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}} {
		// replace the host in the route format
		f := ti.routeFmt + `;healthcheck: Path("/healthcheck") -> "%s"`
		route := fmt.Sprintf(f, backend.URL, backend.URL)

		// create a dataclient with the route
		dc, err := testdataclient.NewDoc(route)
		if err != nil {
			t.Error(ti.msg, "failed to parse route")
			continue
		}

		// start a proxy server
		tl := loggingtest.New()
		r := routing.New(routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			PollTimeout:    42 * time.Microsecond,
			DataClients:    []routing.DataClient{dc},
			Log:            tl})
		p := WithParams(Params{Routing: r, Flags: ti.flags})
		ps := httptest.NewServer(p)
		closeAll := func() {
			ps.Close()
			p.Close()
			r.Close()
			tl.Close()
		}

		// wait for the routing table was activated
		tl.WaitFor("route settings applied", time.Second)

		req, err := http.NewRequest("GET", ps.URL, nil)
		if err != nil {
			t.Error(ti.msg, err)
			closeAll()
			continue
		}

		req.Host = ti.incomingHost
		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Error(ti.msg, "failed to make request")
			closeAll()
			continue
		}

		if ti.flags.Debug() {
			closeAll()
			return
		}

		if rsp.Header.Get("X-Received-Host") != ti.expectedHost {
			t.Error(ti.msg, "wrong host", rsp.Header.Get("X-Received-Host"), ti.expectedHost)
		}

		closeAll()
	}
}
