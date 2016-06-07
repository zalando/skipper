package proxytest

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"net/http/httptest"
	"time"
)

type TestProxy struct {
	URL    string
	proxy  *proxy.Proxy
	server *httptest.Server
}

func WithParams(fr filters.Registry, o proxy.Params, routes ...*eskip.Route) *TestProxy {
	dc := testdataclient.New(routes)
	rt := routing.New(routing.Options{FilterRegistry: fr, DataClients: []routing.DataClient{dc}})
	o.Routing = rt
	pr := proxy.WithParams(o)
	tsp := httptest.NewServer(pr)
	time.Sleep(12 * time.Millisecond) // propagate data client routes
	return &TestProxy{tsp.URL, pr, tsp}
}

func New(fr filters.Registry, routes ...*eskip.Route) *TestProxy {
	return WithParams(fr, proxy.Params{CloseIdleConnsPeriod: -time.Second}, routes...)
}

func (p *TestProxy) Close() error {
	err := p.proxy.Close()
	if err != nil {
		return err
	}

	p.server.Close()
	return nil
}
