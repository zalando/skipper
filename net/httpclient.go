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
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/secrets"
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
			Timeout:       o.Timeout,
			Transport:     tr,
			CheckRedirect: o.CheckRedirect,
		},
		tr: tr,
		sr: sr,
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
	Tracer opentracing.Tracer
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

	// BeforeSend is a hook function that runs just before executing RoundTrip(*http.Request)
	BeforeSend func(*http.Request)
	// AfterResponse is a hook function that runs just after executing RoundTrip(*http.Request)
	AfterResponse func(*http.Response, error)
}

// Transport wraps an http.Transport and adds support for tracing and
// bearerToken injection.
type Transport struct {
	once          sync.Once
	quit          chan struct{}
	tr            *http.Transport
	tracer        opentracing.Tracer
	spanName      string
	componentName string
	bearerToken   string
	beforeSend    func(*http.Request)
	afterResponse func(*http.Response, error)
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
		once:          sync.Once{},
		quit:          make(chan struct{}),
		tr:            htransport,
		tracer:        options.Tracer,
		beforeSend:    options.BeforeSend,
		afterResponse: options.AfterResponse,
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
		beforeSend:    t.beforeSend,
		afterResponse: t.afterResponse,
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
	if t.beforeSend != nil {
		t.beforeSend(req)
	}
	rsp, err := t.tr.RoundTrip(req)
	if t.afterResponse != nil {
		t.afterResponse(rsp, err)
	}
	if span != nil {
		span.LogKV("http_do", "stop")
		if rsp != nil {
			ext.HTTPStatusCode.Set(span, uint16(rsp.StatusCode))
		}
	}

	return rsp, err
}

func (t *Transport) injectSpan(req *http.Request) (*http.Request, opentracing.Span) {
	spanOpts := []opentracing.StartSpanOption{opentracing.Tags{
		string(ext.Component):  t.componentName,
		string(ext.SpanKind):   "client",
		string(ext.HTTPMethod): req.Method,
		string(ext.HTTPUrl):    req.URL.String(),
	}}
	if parentSpan := opentracing.SpanFromContext(req.Context()); parentSpan != nil {
		spanOpts = append(spanOpts, opentracing.ChildOf(parentSpan.Context()))
	}
	span := t.tracer.StartSpan(t.spanName, spanOpts...)
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))

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
		WroteHeaders: func() {
			span.LogKV("wrote_headers", "done")
		},
		WroteRequest: func(wri httptrace.WroteRequestInfo) {
			if wri.Err != nil {
				span.LogKV("wrote_request", ensureUTF8(wri.Err.Error()))
			} else {
				span.LogKV("wrote_request", "done")
			}
		},
		GotFirstResponseByte: func() {
			span.LogKV("got_first_byte", "done")
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
