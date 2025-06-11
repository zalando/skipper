package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"

	teePredicate "github.com/zalando/skipper/predicates/tee"
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
	mu         sync.Mutex
	statusCode int
	header     http.Header
	body       *bytes.Buffer
}

type testProxy struct {
	log     *loggingtest.Logger
	dc      *testdataclient.Client
	routing *routing.Routing
	proxy   *Proxy
}

type listener struct {
	inner    net.Listener
	lastConn chan net.Conn
}

type testLog struct {
	mu sync.Mutex

	buf      bytes.Buffer
	oldOut   io.Writer
	oldLevel log.Level
}

func NewTestLog() *testLog {
	oldOut := log.StandardLogger().Out
	oldLevel := log.GetLevel()
	log.SetLevel(log.DebugLevel)

	tl := &testLog{oldOut: oldOut, oldLevel: oldLevel}
	log.SetOutput(tl)
	return tl
}

func (l *testLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(p)
}

func (l *testLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.String()
}

func (l *testLog) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf.Reset()
}

func (l *testLog) Close() {
	log.SetOutput(l.oldOut)
	log.SetLevel(l.oldLevel)
}

func (l *testLog) WaitForN(exp string, n int, to time.Duration) error {
	timeout := time.After(to)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for log entry: %s", exp)
		case <-ticker.C:
			if l.Count(exp) >= n {
				return nil
			}
		}
	}
}

func (l *testLog) WaitFor(exp string, to time.Duration) error {
	return l.WaitForN(exp, 1, to)
}

func (l *testLog) Count(exp string) int {
	return strings.Count(l.String(), exp)
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
	srw.mu.Lock()
	defer srw.mu.Unlock()
	return srw.body.Write(b)
}

func (srw *syncResponseWriter) Read(b []byte) (int, error) {
	srw.mu.Lock()
	defer srw.mu.Unlock()
	return srw.body.Read(b)
}

func (srw *syncResponseWriter) Flush() {}

func (srw *syncResponseWriter) Len() int {
	srw.mu.Lock()
	defer srw.mu.Unlock()
	return srw.body.Len()
}

func newTestProxyWithFiltersAndParams(fr filters.Registry, doc string, params Params, preprocs []routing.PreProcessor) (*testProxy, error) {
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		return nil, err
	}

	if fr == nil {
		fr = builtin.MakeRegistry()
	}

	tl := loggingtest.New()
	if params.EndpointRegistry == nil {
		params.EndpointRegistry = routing.NewEndpointRegistry(routing.RegistryOptions{})
	}
	opts := routing.Options{
		FilterRegistry: fr,
		PollTimeout:    sourcePollTimeout,
		DataClients:    []routing.DataClient{dc},
		PostProcessors: []routing.PostProcessor{loadbalancer.NewAlgorithmProvider(), params.EndpointRegistry},
		Log:            tl,
		Predicates:     []routing.PredicateSpec{teePredicate.New()},
	}
	if len(preprocs) > 0 {
		opts.PreProcessors = preprocs
	}
	rt := routing.New(opts)

	params.Routing = rt
	p := WithParams(params)
	p.log = tl

	if err := tl.WaitFor("route settings applied", time.Second); err != nil {
		return nil, err
	}

	return &testProxy{tl, dc, rt, p}, nil
}

func newTestProxyWithFilters(fr filters.Registry, doc string, flags Flags, pr ...PriorityRoute) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(fr, doc, Params{Flags: flags, PriorityRoutes: pr}, nil)
}

func newTestProxyWithFiltersAndPreProcessors(fr filters.Registry, doc string, flags Flags, preprocs []routing.PreProcessor) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(fr, doc, Params{Flags: flags}, preprocs)
}

func newTestProxyWithParams(doc string, params Params) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(nil, doc, params, nil)
}

func newTestProxy(doc string, flags Flags, pr ...PriorityRoute) (*testProxy, error) {
	return newTestProxyWithFiltersAndParams(nil, doc, Params{Flags: flags, PriorityRoutes: pr}, nil)
}

func (tp *testProxy) close() {
	tp.log.Close()
	tp.dc.Close()
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

	select {
	case <-l.lastConn:
	default:
	}

	l.lastConn <- c
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

func TestRetries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		method string
		body   func() io.Reader
		want   []int
	}{
		{
			name:   "GET request with nil body",
			method: "GET",
			body:   func() io.Reader { return nil },
			want:   []int{200, 200},
		},
		{
			name:   "GET request with http.NoBody",
			method: "GET",
			body:   func() io.Reader { return http.NoBody },
			want:   []int{200, 200},
		},
		{
			name:   "POST request without body",
			method: "POST",
			body:   func() io.Reader { return nil },
			want:   []int{200, 200},
		},
		{
			name:   "POST request with body",
			method: "POST",
			body:   func() io.Reader { return strings.NewReader("hello") },
			want:   []int{200, 502},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "backend reply")
			}))
			defer backend.Close()

			unavailableBackend := "http://127.0.0.5:9" // refuses connections

			doc := fmt.Sprintf(`hello: * -> <roundRobin, "%s", "%s">`, unavailableBackend, backend.URL)
			tp, err := newTestProxy(doc, FlagsNone)
			require.NoError(t, err)

			ps := httptest.NewServer(tp.proxy)
			defer func() {
				ps.Close()
				tp.close()
			}()

			// To avoid guessing which endpoint round robin picks first,
			// make two requests and compare response codes ignoring request order
			var codes []int
			for i := 0; i < 2; i++ {
				req, err := http.NewRequest(tt.method, ps.URL, tt.body())
				require.NoError(t, err)

				rsp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)

				rsp.Body.Close()
				codes = append(codes, rsp.StatusCode)
			}
			assert.ElementsMatch(t, tt.want, codes)
		})
	}
}

func TestSetRequestUrlFromRequest(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		originalURL *url.URL
		expectedURL *url.URL
		req         *http.Request
	}{{
		"Scheme and Host are set when empty",
		&url.URL{Scheme: "", Host: ""},
		&url.URL{Scheme: "http", Host: "example.com"},
		&http.Request{TLS: nil, Host: "example.com"},
	}, {
		"Scheme and Host are not modified when already set",
		&url.URL{Scheme: "http", Host: "example.com"},
		&url.URL{Scheme: "http", Host: "example.com"},
		&http.Request{TLS: &tls.ConnectionState{}, Host: "example2.com"},
	}, {
		"Scheme is set to http when TLS not set",
		&url.URL{Scheme: ""},
		&url.URL{Scheme: "http"},
		&http.Request{TLS: nil},
	}, {
		"Scheme is set to https when TLS is set",
		&url.URL{Scheme: ""},
		&url.URL{Scheme: "https"},
		&http.Request{TLS: &tls.ConnectionState{}},
	}} {
		u, _ := url.Parse(ti.originalURL.String())
		setRequestURLFromRequest(u, ti.req)

		beq := reflect.DeepEqual(ti.expectedURL, u)
		if !beq {
			t.Error(ti.msg, "<urls don't match>", ti.expectedURL, u)
		}
	}
}

func TestSetRequestUrlForDynamicBackend(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		expectedUrl *url.URL
		stateBag    map[string]interface{}
	}{{
		"DynamicBackendURLKey is set",
		&url.URL{Scheme: "https", Host: "example.com"},
		map[string]interface{}{filters.DynamicBackendURLKey: "https://example.com"},
	}, {
		"DynamicBackendURLKey is set with not url",
		&url.URL{},
		map[string]interface{}{filters.DynamicBackendURLKey: "some string"},
	}, {
		"DynamicBackendHostKey is set",
		&url.URL{Host: "example.com"},
		map[string]interface{}{filters.DynamicBackendHostKey: "example.com"},
	}, {
		"DynamicBackendSchemeKey is set",
		&url.URL{Scheme: "http"},
		map[string]interface{}{filters.DynamicBackendSchemeKey: "http"},
	}, {
		"All keys are set, DynamicBackendURLKey has priority",
		&url.URL{Scheme: "https", Host: "priority.com"},
		map[string]interface{}{
			filters.DynamicBackendSchemeKey: "http",
			filters.DynamicBackendHostKey:   "example.com",
			filters.DynamicBackendURLKey:    "https://priority.com"},
	}} {
		u := &url.URL{}
		setRequestURLForDynamicBackend(u, ti.stateBag)

		beq := reflect.DeepEqual(ti.expectedUrl, u)
		if !beq {
			t.Error(ti.msg, "<urls don't match>", ti.expectedUrl, u)
		}
	}
}

func TestGetRoundtripForDynamicBackend(t *testing.T) {
	payload := []byte("Hello World!")

	s := startTestServer(payload, 0, func(r *http.Request) {
		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}
	})

	defer s.Close()

	fr := make(filters.Registry)
	fr.Register(builtin.NewSetDynamicBackendHost())
	fr.Register(builtin.NewSetDynamicBackendScheme())
	fr.Register(builtin.NewSetDynamicBackendUrl())

	w := httptest.NewRecorder()

	bu, _ := url.ParseRequestURI(s.URL)
	doc := fmt.Sprintf(
		`dynamic: Method("GET") -> setDynamicBackendScheme(%q) ->setDynamicBackendHost(%q) -> <dynamic>;`+
			`dynamic2: Method("POST") -> setDynamicBackendUrl(%q) -> <dynamic>;`+
			`dynamic3: Path("/defaults") -> <dynamic>;`, bu.Scheme, bu.Host, s.URL)

	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	u1, _ := url.ParseRequestURI("https://example1.com")
	r1 := &http.Request{
		URL:    u1,
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	tp.proxy.ServeHTTP(w, r1)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	u2, _ := url.ParseRequestURI("https://example2.com")
	r2 := &http.Request{
		URL:    u2,
		Method: "POST",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	tp.proxy.ServeHTTP(w, r2)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
	}

	u3 := &url.URL{Path: "/defaults"}
	r3 := &http.Request{
		URL:    u3,
		Method: "HEAD",
		Host:   bu.Host,
		Header: http.Header{"X-Test-Header": []string{"test value"}},
	}
	tp.proxy.ServeHTTP(w, r3)

	if w.Code != http.StatusOK {
		t.Error("wrong status", w.Code)
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

func TestFastCgi(t *testing.T) {
	testTables := []struct {
		path        string
		payload     []byte
		requestURI  string
		httpRetCode int
	}{
		{"/hello", []byte("Hello, World!"), "https://www.example.org/hello", http.StatusOK},
		{"/world", []byte("404 page not found\n"), "https://www.example.org/world/test.php", http.StatusNotFound},
	}

	for _, table := range testTables {
		payload := table.payload
		http.HandleFunc(table.path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test-Response-Header", "response header value")

			if len(payload) <= 0 {
				return
			}

			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusOK)

			w.Write(payload)
		})

		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		defer l.Close()

		go fcgi.Serve(l, nil)

		doc := fmt.Sprintf(`fastcgi: * -> "%s"`, "fastcgi://"+l.Addr().String())
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

		u, _ = url.ParseRequestURI(table.requestURI)
		r = &http.Request{
			URL:    u,
			Method: "GET"}
		w = httptest.NewRecorder()
		tp.proxy.ServeHTTP(w, r)
		if w.Code != table.httpRetCode || !bytes.Equal(w.Body.Bytes(), table.payload) {
			t.Errorf("wrong routing for %s, body got:%s want:%s", table.requestURI, w.Body.Bytes(), table.payload)
			t.Errorf("wrong routing for %s, status got: %d, want: %d.", table.requestURI, w.Code, table.httpRetCode)
		}
	}

}

func TestFastCgiServiceUnavailable(t *testing.T) {
	tp, err := newTestProxy(`fastcgi: * -> "fastcgi://invalid.test"`, FlagsNone)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := http.Get(ps.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got: %v", rsp)
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
	fr.Register(builtin.NewAppendRequestHeader())
	fr.Register(builtin.NewAppendResponseHeader())

	doc := fmt.Sprintf(`hello: Path("/hello")
		-> appendRequestHeader("X-Test-Request-Header", "request header value")
		-> appendResponseHeader("X-Test-Response-Header", "response header value")
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

func TestAppliesFiltersAndDefaultFilters(t *testing.T) {
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
	fr.Register(builtin.NewDropQuery())
	fr.Register(builtin.NewAppendRequestHeader())
	fr.Register(builtin.NewAppendResponseHeader())

	doc := fmt.Sprintf(`hello: Path("/hello")
		-> dropQuery("f00")
		-> "%s"
	`, s.URL)

	appendFilter, err := eskip.ParseFilters(`appendResponseHeader("X-Test-Response-Header", "response header value")`)
	if err != nil {
		t.Errorf("Failed to parse append filter: %v", err)
	}
	prependFilter, err := eskip.ParseFilters(`appendRequestHeader("X-Test-Request-Header", "request header value")`)
	if err != nil {
		t.Errorf("Failed to parse prepend filter: %v", err)
	}

	tp, err := newTestProxyWithFiltersAndPreProcessors(fr, doc, FlagsNone, []routing.PreProcessor{
		&eskip.DefaultFilters{
			Append:  appendFilter,
			Prepend: prependFilter,
		},
	})

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
	fr.Register(builtin.NewAppendRequestHeader())
	resp1 := &http.Response{
		Header:     make(http.Header),
		Body:       io.NopCloser(new(bytes.Buffer)),
		StatusCode: http.StatusUnauthorized,
		Status:     "Impossible body",
	}
	fr.Register(&shunter{resp1})
	fr.Register(builtin.NewAppendResponseHeader())

	doc := fmt.Sprintf(`breakerDemo:
		Path("/shunter") ->
		appendRequestHeader("X-Expected", "request header") ->
		appendResponseHeader("X-Expected", "response header") ->
		shunter() ->
		appendRequestHeader("X-Unexpected", "foo") ->
		appendResponseHeader("X-Unexpected", "bar") ->
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

type nilFilterSpec struct{}

func (*nilFilterSpec) Name() string                                              { return "nilFilter" }
func (*nilFilterSpec) CreateFilter(config []interface{}) (filters.Filter, error) { return nil, nil }

func TestFilterPanic(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	var backendRequests int32
	s := startTestServer([]byte("Hello World!"), 0, func(r *http.Request) {
		atomic.AddInt32(&backendRequests, 1)
	})
	defer s.Close()

	fr := make(filters.Registry)
	fr.Register(builtin.NewAppendRequestHeader())
	fr.Register(builtin.NewAppendResponseHeader())
	fr.Register(new(nilFilterSpec))

	doc := fmt.Sprintf(`test:
		Path("/foo") ->
		appendRequestHeader("X-Expected", "before") ->
		appendResponseHeader("X-Expected", "before") ->
		nilFilter() ->
		appendRequestHeader("X-Expected", "after") ->
		appendResponseHeader("X-Expected", "after") ->
		"%s"`, s.URL)

	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	require.NoError(t, err)
	defer tp.close()

	r := httptest.NewRequest("GET", "/foo", nil)
	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, int32(0), backendRequests, "expected no backend request")
	assert.Equal(t, []string{"before"}, r.Header["X-Expected"], "panic expected to skip the rest of the request filters")
	assert.NotContains(t, w.Header(), "X-Expected", "panic expected to skip all of the response filters")

	const msg = "panic caused by: runtime error: invalid memory address or nil pointer dereference"
	if err = testLog.WaitFor(msg, 100*time.Millisecond); err != nil {
		t.Errorf("expected '%s' in logs", msg)
	}
}

func TestFilterPanicPrintStackRate(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	fr := make(filters.Registry)
	fr.Register(new(nilFilterSpec))

	tp, err := newTestProxyWithFilters(fr, `* -> nilFilter() -> <shunt>`, FlagsNone)
	require.NoError(t, err)
	defer tp.close()

	const (
		panicsCaused  = 10
		stacksPrinted = 3 // see Proxy.onPanicSometimes
	)

	for i := 0; i < panicsCaused; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		tp.proxy.ServeHTTP(w, r)
	}

	const errorMsg = "error while proxying"
	if err = testLog.WaitForN(errorMsg, 10, 100*time.Millisecond); err != nil {
		t.Errorf(`expected "%s" to be logged exactly %d times`, errorMsg, panicsCaused)
	}

	const stackMsg = "github.com/zalando/skipper/proxy.TestFilterPanicPrintStackRate"
	if err = testLog.WaitForN(stackMsg, 3, 100*time.Millisecond); err != nil {
		t.Errorf(`expected "%s" to be logged exactly %d times`, stackMsg, stacksPrinted)
	}
}

func TestProcessesRequestWithShuntBackend(t *testing.T) {
	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	reqBody := strings.NewReader("sample request body")
	r := &http.Request{
		URL:    u,
		Method: "POST",
		Body:   io.NopCloser(reqBody),
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()

	fr := make(filters.Registry)
	fr.Register(builtin.NewAppendResponseHeader())

	doc := `hello: Path("/hello") -> appendResponseHeader("X-Test-Response-Header", "response header value") -> <shunt>`
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
	_, err = reqBody.ReadByte()
	if err != io.EOF {
		t.Error("request body was not read")
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
	defer s1.Close()

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

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := http.Get(ps.URL)
	if err != nil {
		t.Error(err)
		return
	}
	defer rsp.Body.Close()

	b, err := io.ReadAll(rsp.Body)
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
		`route: Any() -> preserveHost("false") -> setRequestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve, explicit host last",
		FlagsNone,
		`route: Any() -> preserveHost("true") -> setRequestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve not, explicit host first",
		FlagsNone,
		`route: Any() -> setRequestHeader("Host", "custom.example.org") -> preserveHost("false") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"no proxy preserve, route preserve, explicit host last",
		FlagsNone,
		`route: Any() -> setRequestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
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
		`route: Any() -> preserveHost("false") -> setRequestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve, explicit host last",
		PreserveHost,
		`route: Any() -> preserveHost("true") -> setRequestHeader("Host", "custom.example.org") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve not, explicit host first",
		PreserveHost,
		`route: Any() -> setRequestHeader("Host", "custom.example.org") -> preserveHost("false") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"proxy preserve, route preserve, explicit host last",
		PreserveHost,
		`route: Any() -> setRequestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
		"www.example.org",
		"custom.example.org",
	}, {
		"debug proxy, route not found",
		PreserveHost | Debug,
		`route: Path("/hello") -> setRequestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
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
		`route: Any() -> setRequestHeader("Host", "custom.example.org") -> preserveHost("true") -> "%s"`,
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
		defer rsp.Body.Close()

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
	defer p.close()

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

	if rsp.Header.Get("Content-Length") != "12" { // len("Bad Gateway\n")
		t.Errorf("expected content length of 12, got %s", rsp.Header.Get("Content-Length"))
	}

	if rsp.Header.Get("Transfer-Encoding") != "" {
		t.Error("unexpected transfer encoding")
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

		var lastConn net.Conn
		select {
		case lastConn = <-l.lastConn:
		default:
		}

		if lastConn == nil {
			t.Error("failed to capture connection")
			return
		}

		if err := lastConn.Close(); err != nil {
			t.Error(err)
			return
		}
	}

	backend := httptest.NewUnstartedServer(http.HandlerFunc(handler))
	defer backend.Close()

	l = &listener{inner: backend.Listener, lastConn: make(chan net.Conn, 1)}
	backend.Listener = l
	backend.Start()

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
	rsp.Body.Close()

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
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Error("failed to retry failing connection")
	}
}

func TestResponseHeaderTimeout(t *testing.T) {
	const timeout = 10 * time.Millisecond

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * timeout)
	}))
	defer s.Close()

	params := Params{
		ResponseHeaderTimeout: timeout,
	}
	p, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, s.URL), params)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	// Prevent retry by using POST
	rsp, err := ps.Client().Post(ps.URL, "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got: %v", rsp)
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
	routes = strings.ReplaceAll(routes, "${backend-down}", backendDown.URL)
	routes = strings.ReplaceAll(routes, "${backend-default}", backendDefault.URL)
	routes = strings.ReplaceAll(routes, "${backend-set}", backendSet.URL)

	p, err := newTestProxy(routes, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}
	defer p.close()

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
	defer p.close()

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
		b, err := io.ReadAll(r.Body)
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
	defer backend.Close()

	p, err := newTestProxy(fmt.Sprintf(`* -> "%s"`, backend.URL), FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}
	defer p.close()

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
	p.Close()
	if p.defaultHTTPStatus != http.StatusBadGateway {
		t.Errorf("expected default HTTP status %d, got %d", http.StatusBadGateway, p.defaultHTTPStatus)
	}

	params.DefaultHTTPStatus = http.StatusNetworkAuthenticationRequired + 1
	p = WithParams(params)
	p.Close()
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

func TestLogsAccess(t *testing.T) {
	var accessLog bytes.Buffer
	logging.Init(logging.Options{AccessLogOutput: &accessLog})

	response := "7 bytes"

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"Connection": []string{"token"}}}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> status(%d) -> inlineContent("%s") -> <shunt>`, http.StatusTeapot, response)

	tp, err := newTestProxy(doc, FlagsNone)
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	output := accessLog.String()
	if !strings.Contains(output, fmt.Sprintf(`"%s - -" %d %d "-" "-"`, r.Method, http.StatusTeapot, len(response))) {
		t.Error("failed to log access", output)
	}
}

func TestDisableAccessLog(t *testing.T) {
	var buf bytes.Buffer
	logging.Init(logging.Options{
		AccessLogOutput: &buf})

	response := "7 bytes"

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"Connection": []string{"token"}}}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> status(%d) -> inlineContent("%s") -> <shunt>`, http.StatusTeapot, response)

	tp, err := newTestProxyWithParams(doc, Params{
		AccessLogDisabled: true,
	})
	if err != nil {
		t.Error(err)
		return
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if buf.Len() != 0 {
		t.Error("failed to disable access log")
	}
}

func TestDisableAccessLogWithFilter(t *testing.T) {
	for _, ti := range []struct {
		msg          string
		filter       string
		responseCode int
		disabled     bool
	}{
		{
			msg:          "disable-log-for-all",
			filter:       "disableAccessLog()",
			responseCode: 201,
			disabled:     true,
		},
		{
			msg:          "disable-log-match-exact",
			filter:       "disableAccessLog(200)",
			responseCode: 200,
			disabled:     true,
		},
		{
			msg:          "disable-log-match-prefix",
			filter:       "disableAccessLog(3)",
			responseCode: 302,
			disabled:     true,
		},
		{
			msg:          "disable-log-no-match",
			filter:       "disableAccessLog(1,20,300)",
			responseCode: 500,
			disabled:     false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			var buf bytes.Buffer
			logging.Init(logging.Options{
				AccessLogOutput: &buf})

			response := "7 bytes"

			u, _ := url.ParseRequestURI("https://www.example.org/hello")
			r := &http.Request{
				URL:    u,
				Method: "GET",
				Header: http.Header{"Connection": []string{"token"}}}
			w := httptest.NewRecorder()

			doc := fmt.Sprintf(`hello: Path("/hello") -> %s -> status(%d) -> inlineContent("%s") -> <shunt>`, ti.filter, ti.responseCode, response)

			tp, err := newTestProxyWithParams(doc, Params{
				AccessLogDisabled: false,
			})
			if err != nil {
				t.Error(err)
				return
			}

			defer tp.close()

			tp.proxy.ServeHTTP(w, r)

			if ti.disabled != (buf.Len() == 0) {
				t.Error("failed to disable access log")
			}
		})
	}
}

func TestEnableAccessLogWithFilter(t *testing.T) {
	for _, ti := range []struct {
		msg          string
		filter       string
		responseCode int
		shouldLog    bool
	}{
		{
			msg:          "enable-log-for-all",
			filter:       "enableAccessLog()",
			responseCode: 201,
			shouldLog:    true,
		},
		{
			msg:          "enable-log-match-exact",
			filter:       "enableAccessLog(200)",
			responseCode: 200,
			shouldLog:    true,
		},
		{
			msg:          "enable-log-match-prefix",
			filter:       "enableAccessLog(3)",
			responseCode: 302,
			shouldLog:    true,
		},
		{
			msg:          "enable-log-no-match",
			filter:       "enableAccessLog(1,20,300)",
			responseCode: 500,
			shouldLog:    false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			var buf bytes.Buffer
			logging.Init(logging.Options{
				AccessLogOutput: &buf})

			response := "7 bytes"

			u, _ := url.ParseRequestURI("https://www.example.org/hello")
			r := &http.Request{
				URL:    u,
				Method: "GET",
				Header: http.Header{"Connection": []string{"token"}}}
			w := httptest.NewRecorder()

			doc := fmt.Sprintf(`hello: Path("/hello") -> %s -> status(%d) -> inlineContent("%s") -> <shunt>`, ti.filter, ti.responseCode, response)

			tp, err := newTestProxyWithParams(doc, Params{
				AccessLogDisabled: true,
			})
			if err != nil {
				t.Error(err)
				return
			}

			defer tp.close()

			tp.proxy.ServeHTTP(w, r)

			output := buf.String()
			if ti.shouldLog != strings.Contains(output, fmt.Sprintf(`"%s - -" %d %d "-" "-"`, r.Method, ti.responseCode, len(response))) {
				t.Error("failed to log access", output)
			}
		})
	}
}

func TestAccessLogOnFailedRequest(t *testing.T) {
	testLog := NewTestLog()
	defer testLog.Close()

	logging.Init(logging.Options{AccessLogOutput: testLog})

	p, err := newTestProxy(`* -> "http://bad-gateway.test"`, FlagsNone)
	if err != nil {
		t.Fatalf("Failed to create test proxy: %v", err)
		return
	}
	defer p.close()

	ps := httptest.NewServer(p.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL)
	if err != nil {
		t.Fatalf("Failed to GET: %v", err)
		return
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway {
		t.Errorf("failed to return 502 Bad Gateway on failing backend connection: %d", rsp.StatusCode)
	}

	const expected = `"GET / HTTP/1.1" 502 12 "-" "Go-http-client/1.1"`
	if err = testLog.WaitFor(expected, 100*time.Millisecond); err != nil {
		t.Errorf("Failed to get accesslog %v: %v", expected, err)
		t.Logf("%s", cmp.Diff(testLog.String(), expected))
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

func TestUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		userAgent string
	}{
		{name: "no user agent"},
		{name: "with user agent", userAgent: "test ua"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startTestServer(nil, 0, func(r *http.Request) {
				if got := r.Header.Get("User-Agent"); tc.userAgent != got {
					t.Errorf("user agent mismatch: expected %q, got %q", tc.userAgent, got)
				}
			})
			defer s.Close()

			r := httptest.NewRequest("GET", "http://example.com/foo", nil)
			if tc.userAgent != "" {
				r.Header.Set("User-Agent", tc.userAgent)
			}

			w := httptest.NewRecorder()

			doc := fmt.Sprintf(`* -> "%s"`, s.URL)

			tp, err := newTestProxy(doc, FlagsNone)
			if err != nil {
				t.Fatal(err)
			}
			defer tp.close()

			tp.proxy.ServeHTTP(w, r)
		})
	}
}

func benchmarkAccessLog(b *testing.B, filter string, responseCode int) {
	response := "some bytes"

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{"Connection": []string{"token"}}}

	accessLogFilter := filter
	if filter == "" {
		accessLogFilter = ""
	} else {
		accessLogFilter = fmt.Sprintf("-> %v", filter)
	}
	doc := fmt.Sprintf(`hello: Path("/hello") %s -> status(%d) -> inlineContent("%s") -> <shunt>`, accessLogFilter, responseCode, response)

	tp, err := newTestProxyWithParams(doc, Params{
		AccessLogDisabled: false,
	})
	if err != nil {
		b.Error(err)
		return
	}

	defer tp.close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tp.proxy.ServeHTTP(httptest.NewRecorder(), r)
		}
	})
}

func TestForwardToProxy(t *testing.T) {
	for _, ti := range []struct {
		tls                *tls.ConnectionState
		outgoingURL        string
		incomingHost       string
		expectedProxyURL   string
		expectedRequestURL string
	}{{
		tls:                nil,
		outgoingURL:        "http://proxy.example.com/anything?key=val",
		incomingHost:       "example.com",
		expectedProxyURL:   "http://proxy.example.com",
		expectedRequestURL: "http://example.com/anything?key=val",
	}, {
		tls:                nil,
		outgoingURL:        "https://proxy.example.com/anything?key=val",
		incomingHost:       "example.com",
		expectedProxyURL:   "https://proxy.example.com",
		expectedRequestURL: "http://example.com/anything?key=val",
	}, {
		tls:                &tls.ConnectionState{},
		outgoingURL:        "http://proxy.example.com/anything?key=val",
		incomingHost:       "example.com",
		expectedProxyURL:   "http://proxy.example.com",
		expectedRequestURL: "https://example.com/anything?key=val",
	}} {
		outgoingURL, _ := url.Parse(ti.outgoingURL)

		outgoing := &http.Request{
			URL:    outgoingURL,
			Header: make(http.Header),
		}

		incoming := &http.Request{
			Host: ti.incomingHost,
			TLS:  ti.tls,
		}

		outgoing = forwardToProxy(incoming, outgoing)

		assert.Equal(t, ti.expectedRequestURL, outgoing.URL.String())

		proxyURL, err := proxyFromContext(outgoing)

		assert.NoError(t, err)
		assert.Equal(t, ti.expectedProxyURL, proxyURL.String())
	}
}

func TestProxyFromEmptyContext(t *testing.T) {
	proxyUrl, err := proxyFromContext(&http.Request{})

	assert.NoError(t, err)
	assert.Nil(t, proxyUrl)
}

func BenchmarkAccessLogNoFilter(b *testing.B) { benchmarkAccessLog(b, "", 200) }
func BenchmarkAccessLogDisablePrint(b *testing.B) {
	benchmarkAccessLog(b, "disableAccessLog(1,3)", 200)
}
func BenchmarkAccessLogDisable(b *testing.B) { benchmarkAccessLog(b, "disableAccessLog(1,3,200)", 200) }
func BenchmarkAccessLogEnablePrint(b *testing.B) {
	benchmarkAccessLog(b, "enableAccessLog(1,200,3)", 200)
}
func BenchmarkAccessLogEnable(b *testing.B) { benchmarkAccessLog(b, "enableAccessLog(1,3)", 200) }

func TestInitPassiveHealthChecker(t *testing.T) {
	for i, ti := range []struct {
		inputArg        map[string]string
		expectedEnabled bool
		expectedParams  *PassiveHealthCheck
		expectedError   error
	}{
		{
			inputArg:        map[string]string{},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   nil,
		},
		{
			inputArg: map[string]string{
				"period":                        "somethingInvalid",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid period value: somethingInvalid"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: true,
			expectedParams: &PassiveHealthCheck{
				Period:                     1 * time.Minute,
				MinRequests:                10,
				MaxDropProbability:         0.9,
				MinDropProbability:         0.05,
				MaxUnhealthyEndpointsRatio: 0.3,
			},
			expectedError: nil,
		},
		{
			inputArg: map[string]string{
				"period":                        "-1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid period value: -1m"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "somethingInvalid",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid minRequests value: somethingInvalid"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "-10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid minRequests value: -10"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "somethingInvalid",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid maxDropProbability value: somethingInvalid"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "-0.1",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid maxDropProbability value: -0.1"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "3.1415",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid maxDropProbability value: 3.1415"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "somethingInvalid",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid minDropProbability value: somethingInvalid"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "-0.1",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid minDropProbability value: -0.1"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "3.1415",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid minDropProbability value: 3.1415"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.05",
				"min-drop-probability":          "0.9",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: minDropProbability should be less than maxDropProbability"),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "-0.1",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf(`passive health check: invalid maxUnhealthyEndpointsRatio value: "-0.1"`),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "3.1415",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf(`passive health check: invalid maxUnhealthyEndpointsRatio value: "3.1415"`),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "somethingInvalid",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf(`passive health check: invalid maxUnhealthyEndpointsRatio value: "somethingInvalid"`),
		},
		{
			inputArg: map[string]string{
				"period":                        "1m",
				"min-requests":                  "10",
				"max-drop-probability":          "0.9",
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
				"non-existing":                  "non-existing",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: invalid parameter: key=non-existing,value=non-existing"),
		},
		{
			inputArg: map[string]string{
				"period":       "1m",
				"min-requests": "10",
				/* forgot max-drop-probability */
				"min-drop-probability":          "0.05",
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: false,
			expectedParams:  nil,
			expectedError:   fmt.Errorf("passive health check: missing required parameters [max-drop-probability]"),
		},
		{
			inputArg: map[string]string{
				"period":               "1m",
				"min-requests":         "10",
				"max-drop-probability": "0.9",
				/* using default min-drop-probability */
				"max-unhealthy-endpoints-ratio": "0.3",
			},
			expectedEnabled: true,
			expectedParams: &PassiveHealthCheck{
				Period:                     1 * time.Minute,
				MinRequests:                10,
				MaxDropProbability:         0.9,
				MinDropProbability:         0.0,
				MaxUnhealthyEndpointsRatio: 0.3,
			},
			expectedError: nil,
		},
		{
			inputArg: map[string]string{
				"period":               "1m",
				"min-requests":         "10",
				"max-drop-probability": "0.9",
				"min-drop-probability": "0.05",
				/* using default max-unhealthy-endpoints-ratio */
			},
			expectedEnabled: true,
			expectedParams: &PassiveHealthCheck{
				Period:                     1 * time.Minute,
				MinRequests:                10,
				MaxDropProbability:         0.9,
				MinDropProbability:         0.05,
				MaxUnhealthyEndpointsRatio: 1.0,
			},
			expectedError: nil,
		},
	} {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			enabled, params, err := InitPassiveHealthChecker(ti.inputArg)
			assert.Equal(t, ti.expectedEnabled, enabled)
			assert.Equal(t, ti.expectedError, err)
			if enabled {
				assert.Equal(t, ti.expectedParams, params)
			}
		})
	}
}
