package auth

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
)

const (
	webhookSpanName        = "webhook"
	tokenInfoSpanName      = "tokeninfo"
	tokenIntrospectionSpan = "tokenintrospection"
)

const (
	defaultMaxIdleConns = 64
	tracingTagURL       = "http.url"
)

type authClient struct {
	url  *url.URL
	rt   http.RoundTripper
	mu   sync.Mutex
	quit chan struct{}
}

func newAuthClient(baseURL string, timeout time.Duration, maxIdleConns int) (*authClient, error) {
	if maxIdleConns <= 0 {
		maxIdleConns = defaultMaxIdleConns
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	quit := make(chan struct{})
	tr := createHTTPClient(timeout, quit, maxIdleConns)
	return &authClient{url: u, rt: tr, quit: quit}, nil
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info, ac.rt, ctx.Tracer(), ctx.ParentSpan(), tokenIntrospectionSpan)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string, ctx filters.FilterContext) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a, ac.rt, ctx.Tracer(), ctx.ParentSpan(), tokenInfoSpanName)
	return a, err
}

func (ac *authClient) getWebhook(ctx filters.FilterContext) (int, error) {
	return ac.doClonedGet(ctx, webhookSpanName)
}

// doClonedGet requests url with the same headers and query as the
// incoming request and returns with http statusCode and error.
func (ac *authClient) doClonedGet(ctx filters.FilterContext, spanName string) (int, error) {
	// prepare cloned request
	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return -1, err
	}
	copyHeader(req.Header, ctx.Request().Header)
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), ctx.ParentSpan()))
	span := injectSpan(ctx.Tracer(), ctx.ParentSpan(), spanName, req)
	defer span.Finish()
	req = injectClientTrace(req, span)

	span.LogKV("http_do", "start")
	rsp, err := ac.rt.RoundTrip(req)
	span.LogKV("http_do", "stop")
	if err != nil {
		return -1, err
	}
	defer rsp.Body.Close()

	return rsp.StatusCode, nil
}

func createHTTPClient(timeout time.Duration, quit chan struct{}, maxIdleConns int) *http.Transport {
	transport := &http.Transport{
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
		MaxIdleConnsPerHost:   maxIdleConns,
	}

	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				transport.CloseIdleConnections()
			case <-quit:
				return
			}
		}
	}()

	return transport
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

// jsonGet does a get to the url with accessToken as the bearer token
// in the authorization header. Writes response body into doc.
func jsonGet(
	url *url.URL,
	accessToken string,
	doc interface{},
	rt http.RoundTripper,
	tracer opentracing.Tracer,
	parentSpan opentracing.Span,
	childSpanName string,
) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parentSpan))

	if accessToken != "" {
		req.Header.Set(authHeaderName, authHeaderPrefix+accessToken)
	}

	span := injectSpan(tracer, parentSpan, childSpanName, req)
	defer span.Finish()
	req = injectClientTrace(req, span)

	span.LogKV("http_do", "start")
	rsp, err := rt.RoundTrip(req)
	span.LogKV("http_do", "stop")
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}

	d := json.NewDecoder(rsp.Body)
	return d.Decode(doc)
}

func injectSpan(tracer opentracing.Tracer, parentSpan opentracing.Span, childSpanName string, req *http.Request) opentracing.Span {
	if tracer == nil {
		tracer = &opentracing.NoopTracer{}
	}
	span := tracing.CreateSpan(childSpanName, req.Context(), tracer)
	span.SetTag(tracingTagURL, req.URL.String())
	_ = tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	return span
}

// jsonPost does a form post to the url with auth in the body if auth was provided. Writes response body into doc.
func jsonPost(
	u *url.URL,
	auth string,
	doc *tokenIntrospectionInfo,
	rt http.RoundTripper,
	tracer opentracing.Tracer,
	parentSpan opentracing.Span,
	spanName string,
) error {
	body := url.Values{}
	body.Add(tokenKey, auth)
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parentSpan))

	if u.User != nil {
		authorization := base64.StdEncoding.EncodeToString([]byte(u.User.String()))
		req.Header.Add("Authorization", fmt.Sprintf("Basic %s", authorization))
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	span := injectSpan(tracer, parentSpan, spanName, req)
	defer span.Finish()
	req = injectClientTrace(req, span)

	span.LogKV("http_do", "start")
	rsp, err := rt.RoundTrip(req)
	span.LogKV("http_do", "stop")
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}
	buf := make([]byte, rsp.ContentLength)
	_, err = rsp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	return json.Unmarshal(buf, &doc)
}
