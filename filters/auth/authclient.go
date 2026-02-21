package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	cli *net.Client
}

type tokeninfoClient interface {
	getTokeninfo(token string, ctx filters.FilterContext) (map[string]any, error)
	Close()
}

var _ tokeninfoClient = &authClient{}

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

	cli := net.NewClient(net.Options{
		ResponseHeaderTimeout:   timeout,
		TLSHandshakeTimeout:     timeout,
		MaxIdleConnsPerHost:     maxIdleConns,
		Tracer:                  tracer,
		OpentracingComponentTag: "skipper",
		OpentracingSpanName:     spanName,
	})

	return &authClient{url: u, cli: cli}, nil
}

func (ac *authClient) Close() {
	ac.cli.Close()
}

func bindContext(ctx filters.FilterContext, req *http.Request) *http.Request {
	return req.WithContext(ctx.Request().Context())
}

func (ac *authClient) getTokenintrospect(token string, ctx filters.FilterContext) (tokenIntrospectionInfo, error) {
	body := url.Values{}
	body.Add(tokenKey, token)
	req, err := http.NewRequest("POST", ac.url.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}

	req = bindContext(ctx, req)

	if ac.url.User != nil {
		authorization := base64.StdEncoding.EncodeToString([]byte(ac.url.User.String()))
		req.Header.Add("Authorization", fmt.Sprintf("Basic %s", authorization))
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rsp, err := ac.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		io.Copy(io.Discard, rsp.Body)
		if rsp.StatusCode != 403 && rsp.StatusCode != 401 {
			return nil, fmt.Errorf("failed with status code: %d", rsp.StatusCode)
		}
		return nil, errInvalidToken
	}

	buf, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}
	info := make(tokenIntrospectionInfo)
	err = json.Unmarshal(buf, &info)
	return info, err
}

func (ac *authClient) getTokeninfo(token string, ctx filters.FilterContext) (map[string]any, error) {
	var doc map[string]any

	req, err := http.NewRequest("GET", ac.url.String(), nil)
	if err != nil {
		return doc, err
	}

	req = bindContext(ctx, req)

	if token != "" {
		req.Header.Set(authHeaderName, authHeaderPrefix+token)
	}

	rsp, err := ac.cli.Do(req)
	if err != nil {
		return doc, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		io.Copy(io.Discard, rsp.Body)
		if rsp.StatusCode != 403 && rsp.StatusCode != 401 {
			return nil, fmt.Errorf("failed with status code: %d", rsp.StatusCode)
		}
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

	req = bindContext(ctx, req)
	copyHeader(req.Header, ctx.Request().Header)

	rsp, err := ac.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	return rsp, nil
}
