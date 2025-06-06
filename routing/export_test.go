package routing

import (
	"time"

	"github.com/zalando/skipper/eskip"
)

var (
	ExportProcessRouteDef            = processRouteDef
	ExportNewMatcher                 = newMatcher
	ExportMatch                      = (*matcher).match
	ExportProcessPredicates          = processPredicates
	ExportDefaultLastSeenTimeout     = defaultLastSeenTimeout
	ExportEndpointRegistryAllMetrics = (*EndpointRegistry).allMetrics
)

func SetNow(r *EndpointRegistry, now func() time.Time) {
	r.now = now
}

func (rl *RouteLookup) ValidRoutes() []*eskip.Route {
	return rl.rt.validRoutes
}

func (rl *RouteLookup) DataClients() map[DataClient]struct{} {
	return rl.rt.clients
}
