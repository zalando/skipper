package proxy

import (
	"math/rand"

	"github.com/zalando/skipper/routing"
)

type healthyEndpoints struct {
	rnd              *rand.Rand
	endpointRegistry *routing.EndpointRegistry
}

func (h *healthyEndpoints) filterHealthyEndpoints(endpoints []routing.LBEndpoint, rt *routing.Route) []routing.LBEndpoint {
	if h == nil {
		return endpoints
	}

	p := h.rnd.Float64()

	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		if p < e.Metrics.HealthCheckDropProbability() {
			/* drop */
		} else {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == 0 {
		return endpoints
	}
	return filtered
}
