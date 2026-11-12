package hedge

import (
	"slices"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type shadowTrafficPreProcessor struct {
	registry filters.Registry
}

// NewShadowTrafficPreProcessor creates a routing.PreProcessor that appends
// disableHedge() to every route that produces or receives tee or shadow
// traffic, so that hedged requests and shadow traffic are never mixed.
// It does not change any route if the registry has no disableHedge filter,
// because routes with an unknown filter are dropped.
func NewShadowTrafficPreProcessor(registry filters.Registry) routing.PreProcessor {
	return shadowTrafficPreProcessor{registry: registry}
}

var shadowFilters = []string{
	filters.TeeName,
	"Tee",
	filters.TeenfName,
	filters.TeeLoopbackName,
	filters.TeeResponseName,
}

func (p shadowTrafficPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	if _, ok := p.registry[filters.DisableHedgeName]; !ok {
		log.Errorf("hedge: filter %s is not registered, tee and shadow traffic routes are not excluded from hedged requests", filters.DisableHedgeName)
		return routes
	}

	for _, r := range routes {
		if !usesShadowTraffic(r) || hasDisableHedge(r) {
			continue
		}

		log.Infof("hedge: route %q uses tee or shadow traffic, adding %s()", r.Id, filters.DisableHedgeName)
		r.Filters = append(r.Filters, &eskip.Filter{Name: filters.DisableHedgeName})
	}
	return routes
}

func usesShadowTraffic(r *eskip.Route) bool {
	for _, f := range r.Filters {
		if slices.Contains(shadowFilters, f.Name) {
			return true
		}
	}
	for _, p := range r.Predicates {
		if p.Name == predicates.TeeName {
			return true
		}
	}
	return false
}

func hasDisableHedge(r *eskip.Route) bool {
	for _, f := range r.Filters {
		if f.Name == filters.DisableHedgeName {
			return true
		}
	}
	return false
}
