package proxytest

import (
	"net/http/httptest"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type TestProxy struct {
	URL     string
	log     *loggingtest.Logger
	routing *routing.Routing
	proxy   *proxy.Proxy
	server  *httptest.Server
}

func WithParams(fr filters.Registry, o proxy.Params, routes ...*eskip.Route) *TestProxy {
	dc := testdataclient.New(routes)
	tl := loggingtest.New()
	rt := routing.New(routing.Options{FilterRegistry: fr, DataClients: []routing.DataClient{dc}, Log: tl})
	o.Routing = rt
	pr := proxy.WithParams(o)
	tsp := httptest.NewServer(pr)

	if err := tl.WaitFor("route settings applied", 3*time.Second); err != nil {
		panic(err)
	}

	return &TestProxy{
		URL:     tsp.URL,
		log:     tl,
		routing: rt,
		proxy:   pr,
		server:  tsp}
}

func New(fr filters.Registry, routes ...*eskip.Route) *TestProxy {
	return WithParams(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, routes...)
}

func (p *TestProxy) Close() error {
	p.log.Close()
	p.routing.Close()

	err := p.proxy.Close()
	if err != nil {
		return err
	}

	p.server.Close()
	return nil
}
