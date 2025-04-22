package proxy

import (
	"bytes"
	stdlibcontext "context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"

	log "github.com/sirupsen/logrus"
)

const unknownHost = "_unknownhost_"

type flushedResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	Unwrap() http.ResponseWriter
}

type context struct {
	responseWriter       flushedResponseWriter
	request              *http.Request
	response             *http.Response
	route                *routing.Route
	deprecatedServed     bool
	servedWithResponse   bool // to support the deprecated way independently
	successfulUpgrade    bool
	pathParams           map[string]string
	stateBag             map[string]interface{}
	originalRequest      *http.Request
	originalResponse     *http.Response
	outgoingHost         string
	outgoingDebugRequest *http.Request
	executionCounter     int
	startServe           time.Time
	metrics              *filterMetrics
	tracer               opentracing.Tracer
	initialSpan          opentracing.Span
	proxySpan            opentracing.Span
	parentSpan           opentracing.Span
	proxy                *Proxy
	routeLookup          *routing.RouteLookup
	cancelBackendContext stdlibcontext.CancelFunc
	logger               filters.FilterContextLogger
	backendTime          time.Duration
}

type filterMetrics struct {
	prefix string
	impl   metrics.Metrics
}

type noopFlushedResponseWriter struct {
	ignoredHeader http.Header
}

func defaultBody() io.ReadCloser {
	return io.NopCloser(&bytes.Buffer{})
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

func (c *context) ResponseController() *http.ResponseController {
	return http.NewResponseController(c.responseWriter)
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

func (c *context) Logger() filters.FilterContextLogger {
	if c.logger == nil {
		traceId := tracing.GetTraceID(c.initialSpan)
		if traceId != "" {
			c.logger = log.WithFields(log.Fields{"trace_id": traceId})
		} else {
			c.logger = log.StandardLogger()
		}
	}
	return c.logger
}

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

func (c *context) wasExecuted() bool {
	return c.executionCounter != 0
}

func (c *context) setMetricsPrefix(prefix string) {
	c.metrics.prefix = prefix + ".custom."
}

func (c *context) Split() (filters.FilterContext, error) {
	originalRequest := c.Request()
	if c.proxy.experimentalUpgrade && isUpgradeRequest(originalRequest) {
		return nil, errors.New("context: cannot split the context that contains an upgrade request")
	}
	cc := c.clone()
	cc.stateBag = map[string]interface{}{}
	cc.responseWriter = noopFlushedResponseWriter{}
	cc.metrics = &filterMetrics{
		prefix: cc.metrics.prefix,
		impl:   cc.proxy.metrics,
	}
	u := new(url.URL)
	*u = *originalRequest.URL
	u.Host = originalRequest.Host
	cr, body, err := cloneRequestForSplit(u, originalRequest)
	if err != nil {
		c.Logger().Errorf("context: failed to clone request: %v", err)
		return nil, err
	}
	serverSpan := opentracing.SpanFromContext(originalRequest.Context())
	cr = cr.WithContext(opentracing.ContextWithSpan(cr.Context(), serverSpan))
	cr = cr.WithContext(routing.NewContext(cr.Context()))
	originalRequest.Body = body
	cc.request = cr
	return cc, nil
}

func (c *context) Loopback() {
	loopSpan := c.tracer.StartSpan(c.proxy.tracing.initialOperationName, opentracing.ChildOf(c.parentSpan.Context()))
	defer loopSpan.Finish()
	err := c.proxy.do(c, loopSpan)
	if c.response != nil && c.response.Body != nil {
		if _, err := io.Copy(io.Discard, c.response.Body); err != nil {
			c.Logger().Errorf("context: error while discarding remainder response body: %v.", err)
		}
		err := c.response.Body.Close()
		if err != nil {
			c.Logger().Errorf("context: error during closing the response body: %v", err)
		}
	}
	if c.proxySpan != nil {
		c.proxy.tracing.setTag(c.proxySpan, "shadow", "true")
		c.proxySpan.Finish()
	}

	perr, ok := err.(*proxyError)
	if ok && perr.handled {
		return
	}

	if err != nil {
		c.Logger().Errorf("context: failed to execute loopback request: %v", err)
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

func (w noopFlushedResponseWriter) Header() http.Header {
	if w.ignoredHeader == nil {
		w.ignoredHeader = make(http.Header)
	}

	return w.ignoredHeader
}
func (w noopFlushedResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}
func (w noopFlushedResponseWriter) WriteHeader(_ int)           {}
func (w noopFlushedResponseWriter) Flush()                      {}
func (w noopFlushedResponseWriter) Unwrap() http.ResponseWriter { return nil }
