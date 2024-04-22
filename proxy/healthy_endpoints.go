package proxy

import (
	"math/rand"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type healthyEndpoints struct {
	rnd              *rand.Rand
	endpointRegistry *routing.EndpointRegistry
}

func (h *healthyEndpoints) filterHealthyEndpoints(ctx *context, endpoints []routing.LBEndpoint, metrics metrics.Metrics) []routing.LBEndpoint {
	if h == nil {
		return endpoints
	}

	p := h.rnd.Float64()

	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		dropProbability := e.Metrics.HealthCheckDropProbability()
		if p < dropProbability {
			ctx.Logger().Infof("Dropping endpoint %q due to passive health check: p=%0.2f, dropProbability=%0.2f",
				e.Host, p, dropProbability)
			metrics.IncCounter("routing.endpoint.drop.loadbalancer")
		} else {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == 0 {
		return endpoints
	}
	return filtered
}
