package net

import (
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/tracing"
)

const (
	tracingTagURL = "http.url"
)

type SkipperRoundTripper interface {
	http.RoundTripper
	// Do executes the http request roundtrip with given
	// parentSpan and new created span from string.
	Do(*http.Request, opentracing.Span, string) (*http.Response, error)
}

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
	// Tracer
	Tracer opentracing.Tracer
}

type Transport struct {
	tr     *http.Transport
	tracer opentracing.Tracer
}

func NewHTTPRoundTripper(options Options, quit <-chan struct{}) *Transport {
	// set default tracer
	if options.Tracer == nil {
		options.Tracer = &opentracing.NoopTracer{}
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

	go func() {
		for {
			select {
			case <-time.After(options.IdleConnTimeout):
				htransport.CloseIdleConnections()
			case <-quit:
				return
			}
		}
	}()

	return &Transport{
		tr:     htransport,
		tracer: options.Tracer,
	}
}

// implement RoundTripper interface
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.tr.RoundTrip(req)
}

// Do
func (t *Transport) Do(req *http.Request, parentSpan opentracing.Span, spanName string) (*http.Response, error) {
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parentSpan))
	span := t.injectSpan(parentSpan, spanName, req)
	defer span.Finish()
	req = injectClientTrace(req, span)

	span.LogKV("http_do", "start")
	rsp, err := t.tr.RoundTrip(req)
	span.LogKV("http_do", "stop")

	return rsp, err
}

func (t *Transport) injectSpan(parentSpan opentracing.Span, childSpanName string, req *http.Request) opentracing.Span {
	span := tracing.CreateSpan(childSpanName, req.Context(), t.tracer)
	span.SetTag(tracingTagURL, req.URL.String())
	_ = t.tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	return span
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
