package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
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
	rt   net.SkipperRoundTripper
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
	tr := net.NewHTTPRoundTripper(net.Options{
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
		MaxIdleConnsPerHost:   maxIdleConns,
	}, quit)
	return &authClient{url: u, rt: tr, quit: quit}, nil
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info, ac.rt, ctx.ParentSpan(), tokenIntrospectionSpan)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string, ctx filters.FilterContext) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a, ac.rt, ctx.ParentSpan(), tokenInfoSpanName)
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

	rsp, err := ac.rt.Do(req, ctx.ParentSpan(), spanName)
	if err != nil {
		return -1, err
	}
	defer rsp.Body.Close()

	return rsp.StatusCode, nil
}

// jsonGet does a get to the url with accessToken as the bearer token
// in the authorization header. Writes response body into doc.
func jsonGet(
	url *url.URL,
	accessToken string,
	doc interface{},
	rt net.SkipperRoundTripper,
	parentSpan opentracing.Span,
	childSpanName string,
) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	if accessToken != "" {
		req.Header.Set(authHeaderName, authHeaderPrefix+accessToken)
	}

	rsp, err := rt.Do(req, parentSpan, childSpanName)
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

// jsonPost does a form post to the url with auth in the body if auth was provided. Writes response body into doc.
func jsonPost(
	u *url.URL,
	auth string,
	doc *tokenIntrospectionInfo,
	rt net.SkipperRoundTripper,
	parentSpan opentracing.Span,
	spanName string,
) error {
	body := url.Values{}
	body.Add(tokenKey, auth)
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}

	if u.User != nil {
		authorization := base64.StdEncoding.EncodeToString([]byte(u.User.String()))
		req.Header.Add("Authorization", fmt.Sprintf("Basic %s", authorization))
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rsp, err := rt.Do(req, parentSpan, spanName)
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
