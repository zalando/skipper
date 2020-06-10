package proxy

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

const unknownHost = "_unknownhost_"

type flushedResponseWriter interface {
	http.ResponseWriter
	http.Flusher
}

type context struct {
	responseWriter       flushedResponseWriter
	request              *http.Request
	response             *http.Response
	route                *routing.Route
	deprecatedServed     bool
	servedWithResponse   bool // to support the deprecated way independently
	pathParams           map[string]string
	stateBag             map[string]interface{}
	originalRequest      *http.Request
	originalResponse     *http.Response
	outgoingHost         string
	debugFilterPanics    []interface{}
	outgoingDebugRequest *http.Request
	loopCounter          int
	startServe           time.Time
	metrics              *filterMetrics
	tracer               opentracing.Tracer
	proxySpan            opentracing.Span
	parentSpan           opentracing.Span
	proxy                *Proxy
	routeLookup          *routing.RouteLookup
}

type filterMetrics struct {
	prefix string
	impl   metrics.Metrics
}

type teeTie struct {
	r io.Reader
	w *io.PipeWriter
}

func defaultBody() io.ReadCloser {
	return ioutil.NopCloser(&bytes.Buffer{})
}

func defaultResponse(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       defaultBody(),
		Request:    r,
	}
}

func cloneURL(u *url.URL) *url.URL {
	uc := *u
	return &uc
}

func cloneRequestMetadata(r *http.Request) *http.Request {
	return &http.Request{
		Method:           r.Method,
		URL:              cloneURL(r.URL),
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Trailer:          cloneHeader(r.Trailer),
		Body:             defaultBody(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
		TLS:              r.TLS,
	}
}

func cloneResponseMetadata(r *http.Response) *http.Response {
	return &http.Response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Trailer:          cloneHeader(r.Trailer),
		Body:             defaultBody(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Request:          r.Request,
		TLS:              r.TLS,
	}
}

// this is required during looping to preserve the original set of
// params in the outer routes
func appendParams(to, from map[string]string) map[string]string {
	if to == nil {
		to = make(map[string]string)
	}

	for k, v := range from {
		to[k] = v
	}

	return to
}

func newContext(
	w flushedResponseWriter,
	r *http.Request,
	p *Proxy,
) *context {
	c := &context{
		responseWriter: w,
		request:        r,
		stateBag:       make(map[string]interface{}),
		outgoingHost:   r.Host,
		metrics:        &filterMetrics{impl: p.metrics},
		proxy:          p,
		routeLookup:    p.routing.Get(),
	}

	if p.flags.PreserveOriginal() {
		c.originalRequest = cloneRequestMetadata(r)
	}

	return c
}

func (c *context) applyRoute(route *routing.Route, params map[string]string, preserveHost bool) {
	c.route = route
	if preserveHost {
		c.outgoingHost = c.request.Host
	} else {
		c.outgoingHost = route.Host
	}

	c.pathParams = appendParams(c.pathParams, params)
}

func (c *context) ensureDefaultResponse() {
	if c.response == nil {
		c.response = defaultResponse(c.request)
		return
	}

	if c.response.Header == nil {
		c.response.Header = make(http.Header)
	}

	if c.response.Body == nil {
		c.response.Body = defaultBody()
	}
}

func (c *context) deprecatedShunted() bool {
	return c.deprecatedServed
}

func (c *context) shunted() bool {
	return c.servedWithResponse
}

func (c *context) setResponse(r *http.Response, preserveOriginal bool) {
	c.response = r
	if preserveOriginal {
		c.originalResponse = cloneResponseMetadata(r)
	}
}

func (c *context) ResponseWriter() http.ResponseWriter { return c.responseWriter }
func (c *context) Request() *http.Request              { return c.request }
func (c *context) Response() *http.Response            { return c.response }
func (c *context) MarkServed()                         { c.deprecatedServed = true }
func (c *context) Served() bool                        { return c.deprecatedServed || c.servedWithResponse }
func (c *context) PathParam(key string) string         { return c.pathParams[key] }
func (c *context) StateBag() map[string]interface{}    { return c.stateBag }
func (c *context) BackendUrl() string                  { return c.route.Backend }
func (c *context) OriginalRequest() *http.Request      { return c.originalRequest }
func (c *context) OriginalResponse() *http.Response    { return c.originalResponse }
func (c *context) OutgoingHost() string                { return c.outgoingHost }
func (c *context) SetOutgoingHost(h string)            { c.outgoingHost = h }
func (c *context) Metrics() filters.Metrics            { return c.metrics }
func (c *context) Tracer() opentracing.Tracer          { return c.tracer }
func (c *context) ParentSpan() opentracing.Span        { return c.parentSpan }

func (c *context) Serve(r *http.Response) {
	r.Request = c.Request()

	if r.Header == nil {
		r.Header = make(http.Header)
	}

	if r.Body == nil {
		r.Body = defaultBody()
	}

	c.servedWithResponse = true
	c.response = r
}

func (c *context) metricsHost() string {
	if c.route == nil || len(c.route.HostRegexps) == 0 {
		return unknownHost
	}

	return c.request.Host
}

func (c *context) clone() *context {
	cc := *c

	// preserve the original path params by cloning the set:
	cc.pathParams = appendParams(nil, c.pathParams)

	return &cc
}

func (c *context) setMetricsPrefix(prefix string) {
	c.metrics.prefix = prefix + ".custom."
}

func (tt *teeTie) Read(b []byte) (int, error) {
	n, err := tt.r.Read(b)

	if err != nil && err != io.EOF {
		_ = tt.w.CloseWithError(err)
		return n, err
	}

	if n > 0 {
		if _, werr := tt.w.Write(b[:n]); werr != nil {
			log.Error("context: error while tee request", werr)
		}
	}

	if err == io.EOF {
		_ = tt.w.Close()
	}

	return n, err
}

func (tt *teeTie) Close() error { return nil }

func cloneRequest(req *http.Request) (*http.Request, io.ReadCloser, error) {
	u := new(url.URL)
	*u = *req.URL
	h := make(http.Header)
	for k, v := range req.Header {
		h[k] = v
	}

	var teeBody io.ReadCloser
	mainBody := req.Body

	// see proxy.go:231
	if req.ContentLength != 0 {
		pr, pw := io.Pipe()
		teeBody = pr
		mainBody = &teeTie{mainBody, pw}
	}

	clone, err := http.NewRequest(req.Method, u.String(), teeBody)
	if err != nil {
		return nil, nil, err
	}

	clone.Header = h
	clone.ContentLength = req.ContentLength

	return clone, mainBody, nil
}

func (c *context) Split() (filters.FilterContext, error) {
	originalRequest := c.Request()
	cc := c.clone()
	cr, body, err := cloneRequest(originalRequest)
	if err != nil {
		return nil, err
	}
	originalRequest.Body = body
	cc.request = cr
	return cc, nil
}

func (c *context) Loopback() {
	err := c.proxy.Do(c)
	if err != nil {
		log.Error("context: failed to execute loopback request", err)
	}
}

func (m *filterMetrics) IncCounter(key string) {
	m.impl.IncCounter(m.prefix + key)
}

func (m *filterMetrics) IncCounterBy(key string, value int64) {
	m.impl.IncCounterBy(m.prefix+key, value)
}

func (m *filterMetrics) MeasureSince(key string, start time.Time) {
	m.impl.MeasureSince(m.prefix+key, start)
}

func (m *filterMetrics) IncFloatCounterBy(key string, value float64) {
	m.impl.IncFloatCounterBy(m.prefix+key, value)
}
