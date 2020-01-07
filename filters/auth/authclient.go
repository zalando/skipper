package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
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
	url *url.URL
	tr  *net.Transport
}

func newAuthClient(baseURL, spanName string, timeout time.Duration, maxIdleConns int, tracer opentracing.Tracer) (*authClient, error) {
	if tracer == nil {
		tracer = opentracing.NoopTracer{}
	}
	if maxIdleConns <= 0 {
		maxIdleConns = defaultMaxIdleConns
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	tr := net.NewTransport(net.Options{
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
		MaxIdleConnsPerHost:   maxIdleConns,
		Tracer:                tracer,
	})
	tr = net.WithSpanName(tr, spanName)
	tr = net.WithComponentTag(tr, "skipper")

	return &authClient{url: u, tr: tr}, nil
}

func (ac *authClient) Close() {
	ac.tr.Close()
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	body := url.Values{}
	body.Add(tokenKey, token)
	req, err := http.NewRequest("POST", ac.url.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}

	if ac.url.User != nil {
		authorization := base64.StdEncoding.EncodeToString([]byte(ac.url.User.String()))
		req.Header.Add("Authorization", fmt.Sprintf("Basic %s", authorization))
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rsp, err := ac.tr.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		return nil, errInvalidToken
	}

	buf, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}
	info := make(tokenIntrospectionInfo)
	err = json.Unmarshal(buf, &info)
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

	rsp, err := ac.tr.RoundTrip(req)
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

func (ac *authClient) getWebhook(ctx filters.FilterContext) (*http.Response, error) {
	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return nil, err
	}
	copyHeader(req.Header, ctx.Request().Header)

	rsp, err := ac.tr.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	return rsp, nil
}
