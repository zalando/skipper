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

func New(fr filters.Registry, routes ...*eskip.Route) *httptest.Server {
	dc := testdataclient.New(routes)
	rt := routing.New(routing.Options{FilterRegistry: fr, DataClients: []routing.DataClient{dc}})
	pr := proxy.New(rt, proxy.OptionsNone)
	tsp := httptest.NewServer(pr)
	time.Sleep(12 * time.Millisecond) // propagate data client routes
	return tsp
}
