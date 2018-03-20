package proxy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
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

type testProxy struct {
	log     *loggingtest.Logger
	routing *routing.Routing
	proxy   *Proxy
}

type listener struct {
	inner    net.Listener
	lastConn net.Conn
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

func newTestProxyWithFiltersAndParams(fr filters.Registry, doc string, params Params) (*testProxy, error) {
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		return nil, err
	}

	if fr == nil {
		fr = builtin.MakeRegistry()
	}

	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		Log:            tl})
	params.Routing = rt
	p := WithParams(params)
	p.log = tl

	if err := tl.WaitFor("route settings applied", time.Second); err != nil {
		return nil, err
	}

	return &testProxy{tl, rt, p}, nil
}

func newTestProxyWithFilters(fr filters.Registry, doc string, flags Flags, pr ...PriorityRoute) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(fr, doc, Params{Flags: flags, PriorityRoutes: pr})
}

func newTestProxyWithParams(doc string, params Params) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(nil, doc, params)
}

func newTestProxy(doc string, flags Flags, pr ...PriorityRoute) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(nil, doc, Params{Flags: flags, PriorityRoutes: pr})
}

func (tp *testProxy) close() {
	tp.log.Close()
	tp.routing.Close()
	tp.proxy.Close()
}

func hasArg(arg string) bool {
	for _, a := range os.Args {
		if a == arg {
			return true
		}
	}

	return false
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

func (l *listener) Accept() (c net.Conn, err error) {
	c, err = l.inner.Accept()
	if err != nil {
		return
	}

	l.lastConn = c
	return
}

func (l *listener) Close() error {
	return l.inner.Close()
}

func (l *listener) Addr() net.Addr {
	return l.inner.Addr()
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
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error()
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	if ct, ok := w.Header()["Content-Type"]; !ok || ct[0] != "text/plain" {
		t.Errorf("wrong content type. Expected 'text/plain' but got '%s'", w.Header().Get("Content-Type"))
	}

	if cl, ok := w.Header()["Content-Length"]; !ok || cl[0] != strconv.Itoa(len(payload)) {
		t.Error("wrong content length")
	}

	if xpb, ok := w.Header()["Server"]; !ok || xpb[0] != "Skipper" {
		t.Error("Wrong Server header value")
	}

	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Error("wrong content", w.Body.String())
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
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	if w.Body.Len() != 0 {
		t.Error("wrong content", w.Body.String())
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
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

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
	tp.proxy.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), payload1) {
		t.Error("wrong routing 1")
	}

	u, _ = url.ParseRequestURI("https://www.example.org/host-two/some/path")
	r = &http.Request{
		URL:    u,
		Method: "GET"}
	w = httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, r)
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
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET"}
	w := newSyncResponseWriter()

	parts := 0
	total := 0
	done := make(chan int)
	go tp.proxy.ServeHTTP(w, r)
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

	doc := fmt.Sprintf(`hello: Path("/hello")
		-> requestHeader("X-Test-Request-Header", "request header value")
		-> responseHeader("X-Test-Response-Header", "response header value")
		-> "%s"
	`, s.URL)
	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if h, ok := w.Header()["X-Test-Response-Header"]; !ok || h[0] != "response header value" {
		t.Error("missing response header")
	}
}

type shunter struct {
	resp *http.Response
}

func (b *shunter) Request(c filters.FilterContext)                       { c.Serve(b.resp) }
func (*shunter) Response(filters.FilterContext)                          {}
func (b *shunter) CreateFilter(fc []interface{}) (filters.Filter, error) { return b, nil }
func (*shunter) Name() string                                            { return "shunter" }

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
	fr.Register(&shunter{resp1})
	fr.Register(builtin.NewResponseHeader())

	doc := fmt.Sprintf(`breakerDemo:
		Path("/shunter") ->
		requestHeader("X-Expected", "request header") ->
		responseHeader("X-Expected", "response header") ->
		shunter() ->
		requestHeader("X-Unexpected", "foo") ->
		responseHeader("X-Unexpected", "bar") ->
		"%s"`, s.URL)
	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	r, _ := http.NewRequest("GET", "https://www.example.org/shunter", nil)
	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, r)

	if _, has := r.Header["X-Expected"]; !has {
		t.Error("Request is missing the expected header (added during filter chain winding)")
	}

	if _, has := w.Header()["X-Expected"]; !has {
		t.Error("Response is missing the expected header (added during filter chain unwinding)")
	}

	if _, has := r.Header["X-Unexpected"]; has {
		t.Error("Request has an unexpected header from a filter after the shunter in the chain")
	}

	if _, has := w.Header()["X-Unexpected"]; has {
		t.Error("Response has an unexpected header from a filter after the shunter in the chain")
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
	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

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
		return r.URL.Host == req.URL.Host && r.URL.Scheme == req.URL.Scheme
	}}

	doc := `hello: Path("/hello") -> <shunt>`
	tp, err := newTestProxy(doc, FlagsNone, prt)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, req)
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
		return r.URL.Host == req.URL.Host && r.URL.Scheme == req.URL.Scheme
	}}

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s1.URL)
	tp, err := newTestProxy(doc, FlagsNone, prt)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, req)
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
	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	a := fmt.Sprintf(":%d", 1<<16-rand.Intn(1<<15))
	ps := &http.Server{Addr: a, Handler: tp.proxy}
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

	fr := builtin.MakeRegistry()
	fr.Register(&preserveOriginalSpec{})

	doc := fmt.Sprintf(`hello: Path("/hello") -> preserveOriginal() -> "%s"`, s.URL)
	tp, err := newTestProxyWithFilters(fr, doc, PreserveOriginal)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

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

		tp, err := newTestProxy(route, ti.flags)
		if err != nil {
			t.Error(err)
			return
		}

		ps := httptest.NewServer(tp.proxy)
		closeAll := func() {
			ps.Close()
			tp.close()
		}

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

func TestBackendServiceUnavailable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s.Close()

	p, err := newTestProxy(fmt.Sprintf(`* -> "%s"`, s.URL), 0)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.proxy.Close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	rsp, err := http.Get(ps.URL)
	if err != nil {
		t.Error(err)
		return
	}

	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway {
		t.Error("failed to return 502 Bad Gateway on failing backend connection")
	}
}

func TestRoundtripperRetry(t *testing.T) {
	closeServer := false
	var l *listener
	handler := func(http.ResponseWriter, *http.Request) {
		if !closeServer {
			return
		}

		closeServer = false

		if l.lastConn == nil {
			t.Error("failed to capture connection")
			return
		}

		if err := l.lastConn.Close(); err != nil {
			t.Error(err)
			return
		}
	}

	backend := httptest.NewServer(http.HandlerFunc(handler))
	defer backend.Close()

	l = &listener{inner: backend.Listener}
	backend.Listener = l

	tp, err := newTestProxy(fmt.Sprintf(`* -> "%s"`, backend.URL), 0)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	proxy := httptest.NewServer(tp.proxy)
	defer proxy.Close()

	// create a connection in the pool:
	rsp, err := http.Get(proxy.URL)
	if err != nil {
		t.Error(err)
		return
	}

	if rsp.StatusCode != http.StatusOK {
		t.Error("failed to retry failing connection")
	}

	// repeat with one failing request on the server
	closeServer = true

	rsp, err = http.Get(proxy.URL)
	if err != nil {
		t.Error(err)
		return
	}

	if rsp.StatusCode != http.StatusOK {
		t.Error("failed to retry failing connection")
	}
}

func TestBranding(t *testing.T) {
	routesTpl := `
        shunt: Path("/shunt") -> status(200) -> <shunt>;

        connectionError: Path("/connection-error") -> "${backend-down}";

        default: Path("/default") -> "${backend-default}";

        backendSet: Path("/backend-set") -> "${backend-set}";

        shuntFilterSet: Path("/shunt-filter-set")
            -> setResponseHeader("Server", "filter")
            -> status(200)
            -> <shunt>;

        connectionErrorFilterSet: Path("/connection-error-filter-set")
            -> setResponseHeader("Server", "filter")
            -> "${backend-down}";

        defaultFilterSet: Path("/default-filter-set")
            -> setResponseHeader("Server", "filter")
            -> "${backend-default}";

        backendSetFilterSet: Path("/backend-set-filter-set")
            -> setResponseHeader("Server", "filter")
            -> "${backend-set}";

        shuntFilterDrop: Path("/shunt-filter-drop")
            -> dropResponseHeader("Server")
            -> status(200)
            -> <shunt>;

        connectionErrorFilterDrop: Path("/connection-error-filter-drop")
            -> dropResponseHeader("Server")
            -> "${backend-down}";

        defaultFilterDrop: Path("/default-filter-drop")
            -> dropResponseHeader("Server")
            -> "${backend-default}";

        backendSetFilterDrop: Path("/backend-set-filter-drop")
            -> dropResponseHeader("Server")
            -> "${backend-set}";
    `

	backendDefault := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backendDefault.Close()

	backendSet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "backend")
	}))
	defer backendSet.Close()

	backendDown := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	backendDown.Close()

	routes := routesTpl
	routes = strings.Replace(routes, "${backend-down}", backendDown.URL, -1)
	routes = strings.Replace(routes, "${backend-default}", backendDefault.URL, -1)
	routes = strings.Replace(routes, "${backend-set}", backendSet.URL, -1)

	p, err := newTestProxy(routes, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.proxy.Close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	for _, ti := range []struct {
		uri    string
		header string
	}{
		{"/shunt", "Skipper"},
		{"/connection-error", "Skipper"},
		{"/default", "Skipper"},
		{"/backend-set", "backend"},
		{"/shunt-filter-set", "filter"},

		// filters are not executed on backend connection errors
		{"/connection-error-filter-set", "Skipper"},

		{"/default-filter-set", "filter"},
		{"/backend-set-filter-set", "filter"},
		{"/shunt-filter-drop", ""},

		// filters are not executed on backend connection errors
		{"/connection-error-filter-drop", "Skipper"},

		{"/default-filter-drop", ""},
		{"/backend-set-filter-drop", ""},
	} {
		t.Run(ti.uri, func(t *testing.T) {
			rsp, err := http.Get(ps.URL + ti.uri)
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode == http.StatusNotFound {
				t.Error("not found")
				return
			}

			if rsp.Header.Get("Server") != ti.header {
				t.Errorf(
					"failed to set the right server header; got: %s; expected: %s",
					rsp.Header.Get("Server"),
					ti.header,
				)
			}
		})
	}
}

func TestFixNoAppLogFor404(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	p, err := newTestProxy("", FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.proxy.Close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	rsp, err := http.Get(ps.URL)
	if err != nil {
		t.Error(err)
		return
	}

	defer rsp.Body.Close()

	if err := p.log.WaitFor(unknownRouteID, 120*time.Millisecond); err == nil {
		t.Error("unexpected log on route lookup failed")
	} else if err != loggingtest.ErrWaitTimeout {
		t.Error(err)
	}
}

func TestRequestContentHeaders(t *testing.T) {
	const contentLength = 1 << 15

	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}

		if len(b) != contentLength {
			t.Error("failed to forward content")
			return
		}

		if r.URL.Path == "/with-content-length" {
			if r.ContentLength != contentLength {
				t.Error("failed to forward content length")
				return
			}

			if len(r.TransferEncoding) != 0 {
				t.Error("unexpected transfer encoding")
				return
			}

			return
		}

		if r.ContentLength > 0 {
			t.Error("unexpected content length")
		}

		if len(r.TransferEncoding) != 1 || r.TransferEncoding[0] != "chunked" {
			t.Error("failed to set chunked encoding")
			return
		}
	}))

	p, err := newTestProxy(fmt.Sprintf(`* -> "%s"`, backend.URL), FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.proxy.Close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	req := func(withContentLength bool) {
		path := "/without-content-length"
		if withContentLength {
			path = "/with-content-length"
		}

		req, err := http.NewRequest(
			"GET",
			ps.URL+path,
			io.LimitReader(rand.New(rand.NewSource(42)), contentLength),
		)
		if err != nil {
			t.Error(err)
			return
		}

		if withContentLength {
			req.ContentLength = contentLength
		}

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Error(err)
			return
		}

		rsp.Body.Close()
	}

	req(false)
	req(true)
}

func TestSettingDefaultHTTPStatus(t *testing.T) {
	params := Params{
		DefaultHTTPStatus: http.StatusBadGateway,
	}
	p := WithParams(params)
	if p.defaultHTTPStatus != http.StatusBadGateway {
		t.Errorf("expected default HTTP status %d, got %d", http.StatusBadGateway, p.defaultHTTPStatus)
	}

	params.DefaultHTTPStatus = http.StatusNetworkAuthenticationRequired + 1
	p = WithParams(params)
	if p.defaultHTTPStatus != http.StatusNotFound {
		t.Errorf("expected default HTTP status %d, got %d", http.StatusNotFound, p.defaultHTTPStatus)
	}
}

func TestHopHeaderRemoval(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, 0, func(r *http.Request) {
		if r.Method != "GET" {
			t.Error("wrong request method")
		}

		if r.Header["Connection"] != nil {
			t.Error("expected Connection header to be missing")
		}
	})

	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"Connection": []string{"token"}}}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)

	tp, err := newTestProxy(doc, HopHeadersRemoval)
	if err != nil {
		t.Error()
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}
}

func TestHopHeaderRemovalDisabled(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, 0, func(r *http.Request) {
		if r.Method != "GET" {
			t.Error("wrong request method")
		}

		if th, ok := r.Header["Connection"]; !ok || th[0] != "token" {
			t.Error("wrong Connection header")
		}
	})

	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"Connection": []string{"token"}}}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)

	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error()
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}
}
