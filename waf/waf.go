package waf

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

const wafRoute = `PathSubtree("/login") -> logHeader("request") -> <shunt>`

type WAF struct {
	routing *routing.Routing
	ro      routing.Options
	dc      []routing.DataClient
	Handler http.Handler
}

func New() *WAF {
	dcs := make([]routing.DataClient, 0)
	dc, err := routestring.New(wafRoute)
	if err != nil {
		log.Fatalf("Failed to create WAF dataclient")
	}

	dcs = append(dcs, dc)
	return &WAF{
		dc: dcs,
	}
}

func (waf *WAF) AddRoutingOptions(ro routing.Options) {
	waf.ro = routing.Options{
		DataClients: waf.dc,

		// copy from proxy routing.Options
		FilterRegistry:  ro.FilterRegistry,
		MatchingOptions: ro.MatchingOptions,
		PollTimeout:     ro.PollTimeout,
		Predicates:      ro.Predicates,
		UpdateBuffer:    ro.UpdateBuffer,
		SuppressLogs:    ro.SuppressLogs,
	}
}

func (waf *WAF) Dataclients() []routing.DataClient {
	return waf.dc
}

func (waf *WAF) Run() {
	waf.routing = routing.New(waf.ro)
	tick := time.NewTicker(time.Second)
	for range tick.C {
		routes := make([]*eskip.Route, 0)
		for _, dc := range waf.dc {
			rr, err := dc.LoadAll()
			if err != nil {
				continue
			}
			routes = append(routes, rr...)
		}
	}
}

func (waf *WAF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	println("we are the WAF")
	if waf.routing != nil {
		rr, m := waf.routing.Route(r)
		if rr != nil {
			println("blocked by WAF", m)
			return
		}
	}
	waf.Handler.ServeHTTP(w, r)

}
