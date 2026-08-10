package proxytest

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type TestProxy struct {
	URL  string
	Port string
	Log  *loggingtest.Logger

	dc      *testdataclient.Client
	routing *routing.Routing
	proxy   *proxy.Proxy
	server  *httptest.Server
}

type TestClient struct {
	*http.Client
}

// Do sends the HTTP request. If the request already has a Cookie header (manually added cookies),
// the cookie jar is temporarily disabled so that jar cookies do not interfere with the request.
// This allows tests that manually construct requests with specific cookies to avoid cross-test
// cookie interference while still supporting automatic cookie handling for full browser-like flows.
func (c *TestClient) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Cookie") != "" {
		savedJar := c.Client.Jar
		c.Client.Jar = nil
		defer func() { c.Client.Jar = savedJar }()
	}
	return c.Client.Do(req)
}

type Config struct {
	RoutingOptions routing.Options
	ProxyParams    proxy.Params
	Routes         []*eskip.Route
	Certificates   []tls.Certificate
	WaitTime       time.Duration // Optional wait time, defaults to 3s if zero
}

func WithParamsAndRoutingOptionsAndWait(fr filters.Registry, proxyParams proxy.Params, o routing.Options, wait time.Duration, routes ...*eskip.Route) *TestProxy {
	p := WithParamsAndRoutingOptionsAndWaitUnstarted(fr, proxyParams, o, wait, routes...)
	p.Start()
	return p
}

func WithParamsAndRoutingOptionsAndWaitUnstarted(fr filters.Registry, proxyParams proxy.Params, o routing.Options, wait time.Duration, routes ...*eskip.Route) *TestProxy {
	o.FilterRegistry = fr
	return Config{
		RoutingOptions: o,
		ProxyParams:    proxyParams,
		Routes:         routes,
		WaitTime:       wait,
	}.CreateUnstarted()
}

func WithParamsAndRoutingOptions(fr filters.Registry, proxyParams proxy.Params, o routing.Options, routes ...*eskip.Route) *TestProxy {
	return WithParamsAndRoutingOptionsAndWait(fr, proxyParams, o, 0, routes...)
}

func WithRoutingOptions(fr filters.Registry, o routing.Options, routes ...*eskip.Route) *TestProxy {
	return WithParamsAndRoutingOptions(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, o, routes...)
}

func WithRoutingOptionsWithWait(fr filters.Registry, o routing.Options, wait time.Duration, routes ...*eskip.Route) *TestProxy {
	return WithParamsAndRoutingOptionsAndWait(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, o, wait, routes...)
}

func WithParams(fr filters.Registry, proxyParams proxy.Params, routes ...*eskip.Route) *TestProxy {
	return WithParamsAndRoutingOptions(fr, proxyParams, routing.Options{FilterRegistry: fr}, routes...)
}

func New(fr filters.Registry, routes ...*eskip.Route) *TestProxy {
	return WithParams(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, routes...)
}

func (c Config) CreateUnstarted() *TestProxy {
	waitTime := 3 * time.Second
	if c.WaitTime > 0 {
		waitTime = c.WaitTime
	}

	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	tl := loggingtest.New()
	var dc *testdataclient.Client

	if len(c.RoutingOptions.DataClients) == 0 {
		dc = testdataclient.New(c.Routes)
		c.RoutingOptions.DataClients = []routing.DataClient{dc}
	}

	c.RoutingOptions.Log = tl
	c.RoutingOptions.PostProcessors = append(c.RoutingOptions.PostProcessors, loadbalancer.NewAlgorithmProvider(), endpointRegistry)

	rt := routing.New(c.RoutingOptions)
	c.ProxyParams.Routing = rt
	c.ProxyParams.EndpointRegistry = endpointRegistry

	pr := proxy.WithParams(c.ProxyParams)

	var tsp *httptest.Server

	if len(c.Certificates) > 0 {
		tsp = httptest.NewUnstartedServer(pr)
		tsp.TLS = &tls.Config{Certificates: c.Certificates}
	} else {
		tsp = httptest.NewUnstartedServer(pr)
	}

	if err := tl.WaitFor("route settings applied", waitTime); err != nil {
		panic(err)
	}

	_, port, _ := net.SplitHostPort(tsp.Listener.Addr().String())

	return &TestProxy{
		URL:     tsp.URL,
		Port:    port,
		Log:     tl,
		dc:      dc,
		routing: rt,
		proxy:   pr,
		server:  tsp,
	}
}

func (p *TestProxy) Start() {
	if p.server.TLS != nil {
		p.server.StartTLS()
	} else {
		p.server.Start()
	}
	p.URL = p.server.URL
}

func (c Config) Create() *TestProxy {
	p := c.CreateUnstarted()
	p.Start()
	return p
}

func (p *TestProxy) SetListener(l net.Listener) {
	p.server.Listener.Close()
	p.server.Listener = l
}

func (p *TestProxy) Client() *TestClient {
	c := p.server.Client()
	jar, _ := cookiejar.New(nil)
	c.Jar = jar
	return &TestClient{c}
}

func (p *TestProxy) Close() error {
	p.Log.Close()
	if p.dc != nil {
		p.dc.Close()
	}
	p.routing.Close()
	p.server.Close()

	return p.proxy.Close()
}

// GetBody issues a GET to the specified URL, reads and closes response body and
// returns response, response body bytes and error if any.
func (c *TestClient) GetBody(url string) (rsp *http.Response, body []byte, err error) {
	rsp, err = c.Get(url)
	if err != nil {
		return
	}
	defer rsp.Body.Close()

	body, err = io.ReadAll(rsp.Body)
	return
}
