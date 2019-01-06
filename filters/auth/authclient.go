package auth

import (
	"encoding/json"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	webhookSpanName        = "webhook"
	tokenInfoSpanName      = "tokeninfo"
	tokenIntrospectionSpan = "tokenintrospection"
)

type authClient struct {
	url    *url.URL
	client *http.Client
	mu     sync.Mutex
	quit   chan struct{}
}

func newAuthClient(baseURL string, timeout time.Duration) (*authClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	quit := make(chan struct{})
	client, err := createHTTPClient(timeout, quit)
	if err != nil {
		return nil, fmt.Errorf("Unable to create http client: %v", err)
	}
	return &authClient{url: u, client: client, quit: quit}, nil
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info, ac.client, ctx.Tracer(), ctx.ParentSpan(), tokenIntrospectionSpan)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string, ctx filters.FilterContext) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a, ac.client, ctx.Tracer(), ctx.ParentSpan(), tokenInfoSpanName)
	return a, err
}

func (ac *authClient) getWebhook(ctx filters.FilterContext) (int, error) {
	return ac.doClonedGet(ctx)
}

// doClonedGet requests url with the same headers and query as the
// incoming request and returns with http statusCode and error.
func (ac *authClient) doClonedGet(ctx filters.FilterContext) (int, error) {
	tracer := ctx.Tracer()
	parentSpan := ctx.ParentSpan()
	request := ctx.Request()
	span := tracer.StartSpan(webhookSpanName, opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()
	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return -1, err
	}

	tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	copyHeader(req.Header, request.Header)

	rsp, err := ac.client.Do(req)
	if err != nil {
		return -1, err
	}
	defer rsp.Body.Close()

	return rsp.StatusCode, nil
}

func createHTTPClient(timeout time.Duration, quit chan struct{}) (*http.Client, error) {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
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

	return &http.Client{
		Transport: transport,
	}, nil
}

// jsonGet does a get to the url with accessToken as the bearer token in the authorization header. Writes response body into doc.
func jsonGet(
	url *url.URL,
	accessToken string,
	doc interface{},
	client *http.Client,
	tracer opentracing.Tracer,
	parentSpan opentracing.Span,
	childSpanName string,
) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set(authHeaderName, authHeaderPrefix+accessToken)

	span := injectSpan(tracer, parentSpan, childSpanName, req)
	if span != nil {
		defer span.Finish()
	}

	rsp, err := client.Do(req)
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
	if tracer != nil && parentSpan != nil && childSpanName != "" {
		span := tracer.StartSpan(childSpanName, opentracing.ChildOf(parentSpan.Context()))
		tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
		return span
	}
	return nil
}

// jsonPost does a form post to the url with auth in the body if auth was provided. Writes response body into doc.
func jsonPost(
	u *url.URL,
	auth string,
	doc *tokenIntrospectionInfo,
	client *http.Client,
	tracer opentracing.Tracer,
	parentSpan opentracing.Span,
	spanName string,
) error {
	body := url.Values{}
	body.Add(tokenKey, auth)
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(body.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	span := injectSpan(tracer, parentSpan, spanName, req)
	if span != nil {
		defer span.Finish()
	}
	rsp, err := client.Do(req)
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
