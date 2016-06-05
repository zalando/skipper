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

func NewOptions(fr filters.Registry, o proxy.ProxyOptions, routes ...*eskip.Route) *httptest.Server {
	dc := testdataclient.New(routes)
	rt := routing.New(routing.Options{FilterRegistry: fr, DataClients: []routing.DataClient{dc}})
	o.Routing = rt
	pr := proxy.NewProxy(o)
	tsp := httptest.NewServer(pr)
	time.Sleep(12 * time.Millisecond) // propagate data client routes
	return tsp
}

func New(fr filters.Registry, routes ...*eskip.Route) *httptest.Server {
	return NewOptions(fr, proxy.ProxyOptions{}, routes...)
}
