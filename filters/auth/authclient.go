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
	webhookSpanName            = "webhook"
	tokenInfoSpanName          = "tokeninfo"
	tokenIntrospectionSpanName = "tokenintrospection"
)

const (
	defaultMaxIdleConns = 64
)

type authClient struct {
	url  *url.URL
	tr   *net.Transport
	mu   sync.Mutex
	quit chan struct{}
}

func newAuthClient(baseURL, spanName string, timeout time.Duration, maxIdleConns int, tracer opentracing.Tracer) (*authClient, error) {
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
		Tracer:                tracer,
	}, quit)
	tr = net.WithSpanName(tr, spanName)
	tr = net.WithComponentTag(tr, "skipper")

	return &authClient{url: u, tr: tr, quit: quit}, nil
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	body := url.Values{}
	body.Add(tokenKey, token)
	req, err := http.NewRequest("POST", ac.url.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return info, err
	}

	if ac.url.User != nil {
		authorization := base64.StdEncoding.EncodeToString([]byte(ac.url.User.String()))
		req.Header.Add("Authorization", fmt.Sprintf("Basic %s", authorization))
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rsp, err := ac.tr.Do(req)
	if err != nil {
		return info, err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return info, errInvalidToken
	}
	buf := make([]byte, rsp.ContentLength)
	_, err = rsp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return info, err
	}
	err = json.Unmarshal(buf, &info)
	if err != nil {
		return info, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string, ctx filters.FilterContext) (map[string]interface{}, error) {
	var doc map[string]interface{}

	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return doc, err
	}
	if token != "" {
		req.Header.Set(authHeaderName, authHeaderPrefix+token)
	}

	rsp, err := ac.tr.Do(req)
	if err != nil {
		return doc, err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return doc, errInvalidToken
	}

	d := json.NewDecoder(rsp.Body)
	err = d.Decode(&doc)
	return doc, err
}

func (ac *authClient) getWebhook(ctx filters.FilterContext) (int, error) {
	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return -1, err
	}
	copyHeader(req.Header, ctx.Request().Header)

	rsp, err := ac.tr.Do(req)
	if err != nil {
		return -1, err
	}
	defer rsp.Body.Close()

	return rsp.StatusCode, nil
}
