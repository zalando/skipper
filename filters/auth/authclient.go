package auth

import (
	"encoding/json"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
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

func (ac *authClient) getTokenintrospect(token string) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info, ac.client)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a, ac.client)
	return a, err
}

const webhookSpanName = "webhook"

func (ac *authClient) getWebhook(r *http.Request, tracer opentracing.Tracer, parentSpan opentracing.Span) (int, error) {
	return doClonedGet(ac.url, ac.client, r, tracer, parentSpan, webhookSpanName)
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
func jsonGet(url *url.URL, accessToken string, doc interface{}, client *http.Client) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set(authHeaderName, authHeaderPrefix+accessToken)

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

// jsonPost does a form post to the url with auth in the body if auth was provided. Writes response body into doc.
func jsonPost(u *url.URL, auth string, doc *tokenIntrospectionInfo, client *http.Client) error {
	body := url.Values{}
	body.Add(tokenKey, auth)

	rsp, err := client.PostForm(u.String(), body)
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

// doClonedGet requests url with the same headers and query as the
// incoming request and returns with http statusCode and error.
func doClonedGet(u *url.URL, client *http.Client, incoming *http.Request, tracer opentracing.Tracer,
	parentSpan opentracing.Span, childSpanName string) (int, error) {
	span := tracer.StartSpan(childSpanName, opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return -1, err
	}

	tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	copyHeader(req.Header, incoming.Header)

	rsp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer rsp.Body.Close()

	return rsp.StatusCode, nil
}
