package proxytest

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
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

type Config struct {
	RoutingOptions routing.Options
	ProxyParams    proxy.Params
	Routes         []*eskip.Route
	Certificates   []tls.Certificate
}

func WithParamsAndRoutingOptions(fr filters.Registry, proxyParams proxy.Params, o routing.Options, routes ...*eskip.Route) *TestProxy {
	o.FilterRegistry = fr
	return Config{
		RoutingOptions: o,
		ProxyParams:    proxyParams,
		Routes:         routes,
	}.Create()
}

func WithRoutingOptions(fr filters.Registry, o routing.Options, routes ...*eskip.Route) *TestProxy {
	o.FilterRegistry = fr
	return Config{
		RoutingOptions: o,
		ProxyParams:    proxy.Params{CloseIdleConnsPeriod: -time.Second},
		Routes:         routes,
	}.Create()
}

func WithParams(fr filters.Registry, proxyParams proxy.Params, routes ...*eskip.Route) *TestProxy {
	return Config{
		RoutingOptions: routing.Options{FilterRegistry: fr},
		ProxyParams:    proxyParams,
		Routes:         routes,
	}.Create()
}

func New(fr filters.Registry, routes ...*eskip.Route) *TestProxy {
	return Config{
		RoutingOptions: routing.Options{FilterRegistry: fr},
		ProxyParams:    proxy.Params{CloseIdleConnsPeriod: -time.Second},
		Routes:         routes,
	}.Create()
}

func (c Config) Create() *TestProxy {
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
		tsp.StartTLS()
	} else {
		tsp = httptest.NewServer(pr)
	}

	if err := tl.WaitFor("route settings applied", 3*time.Second); err != nil {
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

func (p *TestProxy) Client() *TestClient {
	return &TestClient{p.server.Client()}
}

func (tp *TestProxy) UpdateRoutes(routes []*eskip.Route, deleted []string) error {
	if tp.dc == nil {
		panic("no test data client available for route updates")
	}

	tp.dc.Update(routes, deleted)
	_, _, err := tp.dc.LoadUpdate()

	if err != nil {
		return err
	}
	return nil
}

func (p *TestProxy) GetRoutes() map[string]*eskip.Route {
	if p.dc == nil {
		return nil
	}
	return p.dc.GetRoutes()
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
