package net

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultIdleConnTimeout = 30 * time.Second
	defaultRefreshInterval = 5 * time.Minute
)

// Client adds additional features like Bearer token injection, and
// opentracing to the wrapped http.Client with the same interface as
// http.Client from the stdlib.
type Client struct {
	once   sync.Once
	client http.Client
	tr     *Transport
	log    logging.Logger
	sr     secrets.SecretsReader
}

// NewClient creates a wrapped http.Client and uses Transport to
// support OpenTracing. On teardown you have to use Close() to
// not leak a goroutine.
//
// If secrets.SecretsReader is nil, but BearerTokenFile is not empty
// string, it creates StaticDelegateSecret with a wrapped
// secrets.SecretPaths, which can be used with Kubernetes secrets to
// read from the secret an automatically updated Bearer token.
func NewClient(o Options) *Client {
	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	tr := NewTransport(o)

	sr := o.SecretsReader
	if sr == nil && o.BearerTokenFile != "" {
		if o.BearerTokenRefreshInterval == 0 {
			o.BearerTokenRefreshInterval = defaultRefreshInterval
		}
		sp := secrets.NewSecretPaths(o.BearerTokenRefreshInterval)
		if err := sp.Add(o.BearerTokenFile); err != nil {
			o.Log.Errorf("failed to read secret: %v", err)
		}
		sr = secrets.NewStaticDelegateSecret(sp, o.BearerTokenFile)
	}

	c := &Client{
		once: sync.Once{},
		client: http.Client{
			Transport:     tr,
			CheckRedirect: o.CheckRedirect,
		},
		tr:  tr,
		log: o.Log,
		sr:  sr,
	}

	return c
}

func (c *Client) Close() {
	c.once.Do(func() {
		c.tr.Close()
		if c.sr != nil {
			c.sr.Close()
		}
	})
}

func (c *Client) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(req)
}

func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) Post(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	return c.Do(req)
}

func (c *Client) PostForm(url string, data url.Values) (*http.Response, error) {
	return c.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

// Do delegates the given http.Request to the underlying http.Client
// and adds a Bearer token to the authorization header, if Client has
// a secrets.SecretsReader and the request does not contain an
// Authorization header.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.sr != nil && req.Header.Get("Authorization") == "" {
		if b, ok := c.sr.GetSecret(req.URL.String()); ok {
			req.Header.Set("Authorization", "Bearer "+string(b))
		}
	}
	return c.client.Do(req)
}

// CloseIdleConnections delegates the call to the underlying
// http.Client.
func (c *Client) CloseIdleConnections() {
	c.client.CloseIdleConnections()
}

// Options are mostly passed to the http.Transport of the same
// name. Options.Timeout can be used as default for all timeouts, that
// are not set. You can pass an opentracing.Tracer
// https://godoc.org/github.com/opentracing/opentracing-go#Tracer,
// which can be nil to get the
// https://godoc.org/github.com/opentracing/opentracing-go#NoopTracer.
type Options struct {
	// Transport see https://golang.org/pkg/net/http/#Transport
	// In case Transport is not nil, the Transport arguments are used below.
	Transport *http.Transport
	// CheckRedirect see https://golang.org/pkg/net/http/#Client
	CheckRedirect func(req *http.Request, via []*http.Request) error
	// Proxy see https://golang.org/pkg/net/http/#Transport.Proxy
	Proxy func(req *http.Request) (*url.URL, error)
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
	// DEPRECATED, use TracerWrapper instead.
	Tracer opentracing.Tracer
	// TracerWrapper instance, tracer wrapper is an abstraction around
	// the different types of tracers skipper operates with. Currently
	// skipper operates with OpenTelemetry and OpenTracing. In case
	// this option is nil tracing will not be enabled.
	OtelTracer trace.Tracer
	// OpentracingComponentTag sets component tag for all requests
	OpentracingComponentTag string
	// OpentracingSpanName sets span name for all requests
	OpentracingSpanName string

	// BearerTokenFile injects bearer token read from file, which
	// file path is the given string. In case SecretsReader is
	// provided, BearerTokenFile will be ignored.
	BearerTokenFile string
	// BearerTokenRefreshInterval refresh bearer from
	// BearerTokenFile. In case SecretsReader is provided,
	// BearerTokenFile will be ignored.
	BearerTokenRefreshInterval time.Duration
	// SecretsReader is used to read and refresh bearer tokens
	SecretsReader secrets.SecretsReader

	// Log is used for error logging
	Log logging.Logger
}

// Transport wraps an http.Transport and adds support for tracing and
// bearerToken injection.
type Transport struct {
	once          sync.Once
	quit          chan struct{}
	tr            *http.Transport
	tracer        trace.Tracer
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
	if options.Tracer == nil && options.OtelTracer == nil {
		options.OtelTracer = &tracing.TracerWrapper{Ot: &opentracing.NoopTracer{}}
	}

	if options.OtelTracer == nil && options.Tracer != nil {
		options.OtelTracer = &tracing.TracerWrapper{Ot: options.Tracer}
	}

	// set timeout defaults
	if options.TLSHandshakeTimeout == 0 {
		options.TLSHandshakeTimeout = options.Timeout
	}
	if options.IdleConnTimeout == 0 {
		if options.Timeout != 0 {
			options.IdleConnTimeout = options.Timeout
		} else {
			options.IdleConnTimeout = defaultIdleConnTimeout
		}
	}
	if options.ResponseHeaderTimeout == 0 {
		options.ResponseHeaderTimeout = options.Timeout
	}
	if options.ExpectContinueTimeout == 0 {
		options.ExpectContinueTimeout = options.Timeout
	}
	if options.Proxy == nil {
		options.Proxy = http.ProxyFromEnvironment
	}

	var htransport *http.Transport
	if options.Transport != nil {
		htransport = options.Transport
	} else {
		htransport = &http.Transport{
			Proxy:                  options.Proxy,
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
	}

	t := &Transport{
		once:   sync.Once{},
		quit:   make(chan struct{}),
		tr:     htransport,
		tracer: options.OtelTracer,
	}

	if t.tracer != nil {
		if options.OpentracingComponentTag != "" {
			t = WithComponentTag(t, options.OpentracingComponentTag)
		}
		if options.OpentracingSpanName != "" {
			t = WithSpanName(t, options.OpentracingSpanName)
		}
	}

	go func() {
		ticker := time.NewTicker(options.IdleConnTimeout)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				htransport.CloseIdleConnections()
			case <-t.quit:
				htransport.CloseIdleConnections()
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
	return &Transport{
		once:          sync.Once{},
		quit:          t.quit,
		tr:            t.tr,
		tracer:        t.tracer,
		spanName:      t.spanName,
		componentName: t.componentName,
		bearerToken:   t.bearerToken,
	}
}

func (t *Transport) Close() {
	t.once.Do(func() {
		close(t.quit)
	})
}

func (t *Transport) CloseIdleConnections() {
	t.tr.CloseIdleConnections()
}

// RoundTrip the request with tracing, bearer token injection and add client
// tracing: DNS, TCP/IP, TLS handshake, connection pool access. Client
// traces are added as logs into the created span.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var span trace.Span
	if t.spanName != "" {
		req, span = t.injectSpan(req)
		defer span.End()
		req = injectClientTrace(req, span)
		span.AddEvent("http_do", trace.WithAttributes(attribute.String("http_do", "start")))
	}
	if t.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.bearerToken)
	}
	rsp, err := t.tr.RoundTrip(req)
	if span != nil {
		span.AddEvent("http_do", trace.WithAttributes(attribute.String("http_do", "stop")))
		if rsp != nil {
			span.SetAttributes(attribute.Int(tracing.HTTPStatusCodeTag, rsp.StatusCode))
		}
	}

	return rsp, err
}

func (t *Transport) injectSpan(req *http.Request) (*http.Request, trace.Span) {
	ctx, span := t.tracer.Start(req.Context(), t.spanName)
	req = req.WithContext(ctx)

	// add Tags
	span.SetAttributes(attribute.String(tracing.ComponentTag, t.componentName))
	span.SetAttributes(attribute.String(tracing.HTTPUrlTag, req.URL.String()))
	span.SetAttributes(attribute.String(tracing.HTTPMethodTag, req.Method))
	span.SetAttributes(attribute.String(tracing.SpanKindTag, "client"))

	// Isso aqui acho que era melhor extrair da struct e receber a struct pra tentar fazer a conversao dentro do pacote
	// Vai ficar mais limpo e assim tira a preocupacao de conversao de quem usa o wrapper.
	req = tracing.Inject(ctx, req, span, t.tracer)

	return req, span
}

func injectClientTrace(req *http.Request, span trace.Span) *http.Request {
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			span.AddEvent("DNS", trace.WithAttributes(attribute.String("DNS", "start")))
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			span.AddEvent("DNS", trace.WithAttributes(attribute.String("DNS", "stop")))
		},
		ConnectStart: func(string, string) {
			span.AddEvent("connect", trace.WithAttributes(attribute.String("connect", "start")))
		},
		ConnectDone: func(string, string, error) {
			span.AddEvent("connect", trace.WithAttributes(attribute.String("connect", "end")))
		},
		TLSHandshakeStart: func() {
			span.AddEvent("TLS", trace.WithAttributes(attribute.String("TLS", "start")))
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			span.AddEvent("TLS", trace.WithAttributes(attribute.String("TLS", "end")))
		},
		GetConn: func(string) {
			span.AddEvent("get_conn", trace.WithAttributes(attribute.String("get_conn", "start")))
		},
		GotConn: func(httptrace.GotConnInfo) {
			span.AddEvent("get_conn", trace.WithAttributes(attribute.String("get_conn", "end")))
		},
		WroteHeaders: func() {
			span.AddEvent("wrote_headers", trace.WithAttributes(attribute.String("wrote_headers", "done")))
		},
		WroteRequest: func(wri httptrace.WroteRequestInfo) {
			if wri.Err != nil {
				span.AddEvent("wrote_request", trace.WithAttributes(attribute.String("wrote_request", ensureUTF8(wri.Err.Error()))))
			} else {
				span.AddEvent("wrote_request", trace.WithAttributes(attribute.String("wrote_request", "done")))
			}
		},
		GotFirstResponseByte: func() {
			span.AddEvent("got_first_byte", trace.WithAttributes(attribute.String("got_first_byte", "done")))
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

func ensureUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return fmt.Sprintf("invalid utf-8: %q", s)
}
