package net

import (
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

const (
	defaultTimeout = 30 * time.Second
)

// Options are mostly passed to the http.Transport of the same
// name. Options.Timeout can be used as default for all timeouts, that
// are not set. You can pass an opentracing.Tracer
// https://godoc.org/github.com/opentracing/opentracing-go#Tracer,
// which can be nil to get the
// https://godoc.org/github.com/opentracing/opentracing-go#NoopTracer.
type Options struct {
	// DisableKeepAlives see https://golang.org/pkg/net/http/#Transport.DisableKeepAlives
	DisableKeepAlives bool
	// DisableCompression see https://golang.org/pkg/net/http/#Transport.DisableCompression
	DisableCompression bool
	// ForceAttemptHTTP2 see https://golang.org/pkg/net/http/#Transport.ForceAttemptHTTP2
	ForceAttemptHTTP2 bool
	// MaxIdleConns see https://golang.org/pkg/net/http/#Transport.MaxIdleConns
	MaxIdleConns int
	// MaxIdleConnsPerHost see https://golang.org/pkg/net/http/#Transport.MaxIdleConnsPerHost
	MaxIdleConnsPerHost int
	// MaxConnsPerHost see https://golang.org/pkg/net/http/#Transport.MaxConnsPerHost
	MaxConnsPerHost int
	// WriteBufferSize see https://golang.org/pkg/net/http/#Transport.WriteBufferSize
	WriteBufferSize int
	// ReadBufferSize see https://golang.org/pkg/net/http/#Transport.ReadBufferSize
	ReadBufferSize int
	// MaxResponseHeaderBytes see
	// https://golang.org/pkg/net/http/#Transport.MaxResponseHeaderBytes
	MaxResponseHeaderBytes int64
	// Timeout sets all Timeouts, that are set to 0 to the given
	// value. Basically it's the default timeout value.
	Timeout time.Duration
	// TLSHandshakeTimeout see
	// https://golang.org/pkg/net/http/#Transport.TLSHandshakeTimeout,
	// if not set or set to 0, its using Options.Timeout.
	TLSHandshakeTimeout time.Duration
	// IdleConnTimeout see
	// https://golang.org/pkg/net/http/#Transport.IdleConnTimeout,
	// if not set or set to 0, its using Options.Timeout.
	IdleConnTimeout time.Duration
	// ResponseHeaderTimeout see
	// https://golang.org/pkg/net/http/#Transport.ResponseHeaderTimeout,
	// if not set or set to 0, its using Options.Timeout.
	ResponseHeaderTimeout time.Duration
	// ExpectContinueTimeout see
	// https://golang.org/pkg/net/http/#Transport.ExpectContinueTimeout,
	// if not set or set to 0, its using Options.Timeout.
	ExpectContinueTimeout time.Duration
	// Tracer instance, can be nil to not enable tracing
	Tracer opentracing.Tracer
}

// Transport wraps an http.Transport and adds support for tracing and
// bearerToken injection.
type Transport struct {
	quit          chan struct{}
	tr            *http.Transport
	tracer        opentracing.Tracer
	spanName      string
	componentName string
	bearerToken   string
}

// NewTransport creates a wrapped http.Transport, with regular DNS
// lookups using CloseIdleConnections on every IdleConnTimeout. You
// can optionally add tracing. On teardown you have to use Close() to
// not leak a goroutine.
func NewTransport(options Options) *Transport {
	// set default tracer
	if options.Tracer == nil {
		options.Tracer = &opentracing.NoopTracer{}
	}

	if options.Timeout == 0 {
		options.Timeout = defaultTimeout
	}

	// set timeout defaults
	if options.TLSHandshakeTimeout == 0 {
		options.TLSHandshakeTimeout = options.Timeout
	}
	if options.IdleConnTimeout == 0 {
		options.IdleConnTimeout = options.Timeout
	}
	if options.ResponseHeaderTimeout == 0 {
		options.ResponseHeaderTimeout = options.Timeout
	}
	if options.ExpectContinueTimeout == 0 {
		options.ExpectContinueTimeout = options.Timeout
	}

	htransport := &http.Transport{
		DisableKeepAlives:      options.DisableKeepAlives,
		DisableCompression:     options.DisableCompression,
		ForceAttemptHTTP2:      options.ForceAttemptHTTP2,
		MaxIdleConns:           options.MaxIdleConns,
		MaxIdleConnsPerHost:    options.MaxIdleConnsPerHost,
		MaxConnsPerHost:        options.MaxConnsPerHost,
		WriteBufferSize:        options.WriteBufferSize,
		ReadBufferSize:         options.ReadBufferSize,
		MaxResponseHeaderBytes: options.MaxResponseHeaderBytes,
		ResponseHeaderTimeout:  options.ResponseHeaderTimeout,
		TLSHandshakeTimeout:    options.TLSHandshakeTimeout,
		IdleConnTimeout:        options.IdleConnTimeout,
		ExpectContinueTimeout:  options.ExpectContinueTimeout,
	}

	t := &Transport{
		quit:   make(chan struct{}),
		tr:     htransport,
		tracer: options.Tracer,
	}

	go func() {
		for {
			select {
			case <-time.After(options.IdleConnTimeout):
				htransport.CloseIdleConnections()
			case <-t.quit:
				return
			}
		}
	}()

	return t
}

// WithSpanName sets the name of the span, if you have an enabled
// tracing Transport.
func WithSpanName(t *Transport, spanName string) *Transport {
	tt := t.shallowCopy()
	tt.spanName = spanName
	return tt
}

// WithComponentTag sets the component name, if you have an enabled
// tracing Transport.
func WithComponentTag(t *Transport, componentName string) *Transport {
	tt := t.shallowCopy()
	tt.componentName = componentName
	return tt
}

// WithBearerToken adds an Authorization header with "Bearer " prefix
// and add the given bearerToken as value to all requests. To regular
// update your token you need to call this method and use the returned
// Transport.
func WithBearerToken(t *Transport, bearerToken string) *Transport {
	tt := t.shallowCopy()
	tt.bearerToken = bearerToken
	return tt
}

func (t *Transport) shallowCopy() *Transport {
	tt := *t
	return &tt
}

func (t *Transport) Close() {
	t.quit <- struct{}{}
}

// RoundTrip the request with tracing, bearer token injection and add client
// tracing: DNS, TCP/IP, TLS handshake, connection pool access. Client
// traces are added as logs into the created span.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var span opentracing.Span
	if t.spanName != "" {
		req, span = t.injectSpan(req)
		defer span.Finish()
		req = injectClientTrace(req, span)
		span.LogKV("http_do", "start")
	}
	if t.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.bearerToken)
	}
	rsp, err := t.tr.RoundTrip(req)
	if span != nil {
		span.LogKV("http_do", "stop")
		if rsp != nil {
			ext.HTTPStatusCode.Set(span, uint16(rsp.StatusCode))
		}
	}

	return rsp, err
}

func (t *Transport) injectSpan(req *http.Request) (*http.Request, opentracing.Span) {
	parentSpan := opentracing.SpanFromContext(req.Context())
	var span opentracing.Span
	if parentSpan != nil {
		req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parentSpan))
		span = t.tracer.StartSpan(t.spanName, opentracing.ChildOf(parentSpan.Context()))
	} else {
		span = t.tracer.StartSpan(t.spanName)
	}

	// add Tags
	ext.Component.Set(span, t.componentName)
	ext.HTTPUrl.Set(span, req.URL.String())
	ext.HTTPMethod.Set(span, req.Method)
	ext.SpanKind.Set(span, "client")

	_ = t.tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))

	return req, span
}

func injectClientTrace(req *http.Request, span opentracing.Span) *http.Request {
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			span.LogKV("DNS", "start")
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			span.LogKV("DNS", "end")
		},
		ConnectStart: func(string, string) {
			span.LogKV("connect", "start")
		},
		ConnectDone: func(string, string, error) {
			span.LogKV("connect", "end")
		},
		TLSHandshakeStart: func() {
			span.LogKV("TLS", "start")
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			span.LogKV("TLS", "end")
		},
		GetConn: func(string) {
			span.LogKV("get_conn", "start")
		},
		GotConn: func(httptrace.GotConnInfo) {
			span.LogKV("get_conn", "end")
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}
