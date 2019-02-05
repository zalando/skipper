package proxytest

import (
	"net/http/httptest"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type TestProxy struct {
	URL     string
	Log     *loggingtest.Logger
	routing *routing.Routing
	proxy   *proxy.Proxy
	server  *httptest.Server
}

func WithRoutingOptions(fr filters.Registry, o routing.Options, routes ...*eskip.Route) *TestProxy {
	return newTestProxy(fr, o, proxy.Params{CloseIdleConnsPeriod: -time.Second}, routes...)
}

func WithParams(fr filters.Registry, proxyParams proxy.Params, routes ...*eskip.Route) *TestProxy {
	return newTestProxy(fr, routing.Options{}, proxyParams, routes...)
}

func newTestProxy(fr filters.Registry, routingOptions routing.Options, proxyParams proxy.Params, routes ...*eskip.Route) *TestProxy {
	tl := loggingtest.New()

	if len(routingOptions.DataClients) == 0 {
		dc := testdataclient.New(routes)
		routingOptions.DataClients = []routing.DataClient{dc}
	}

	routingOptions.FilterRegistry = fr
	routingOptions.Log = tl
	routingOptions.PostProcessors = []routing.PostProcessor{loadbalancer.NewAlgorithmProvider()}

	rt := routing.New(routingOptions)

	proxyParams.Routing = rt
	if proxyParams.OpenTracer == nil {
		proxyParams.OpenTracer = &opentracing.NoopTracer{}
	}
	pr := proxy.WithParams(proxyParams)
	tsp := httptest.NewServer(pr)

	if err := tl.WaitFor("route settings applied", 3*time.Second); err != nil {
		panic(err)
	}

	return &TestProxy{
		URL:     tsp.URL,
		Log:     tl,
		routing: rt,
		proxy:   pr,
		server:  tsp,
	}
}

func New(fr filters.Registry, routes ...*eskip.Route) *TestProxy {
	return WithParams(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, routes...)
}

func (p *TestProxy) Close() error {
	p.Log.Close()
	p.routing.Close()

	err := p.proxy.Close()
	if err != nil {
		return err
	}

	p.server.Close()
	return nil
}
