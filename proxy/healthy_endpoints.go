package proxy

import (
	"math/rand"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type healthyEndpoints struct {
	rnd                        *rand.Rand
	endpointRegistry           *routing.EndpointRegistry
	maxUnhealthyEndpointsRatio float64
}

func (h *healthyEndpoints) filterHealthyEndpoints(ctx *context, endpoints []routing.LBEndpoint, metrics metrics.Metrics) []routing.LBEndpoint {
	if h == nil {
		return endpoints
	}

	p := h.rnd.Float64()

	unhealthyEndpointsCount := 0
	maxUnhealthyEndpointsCount := float64(len(endpoints)) * h.maxUnhealthyEndpointsRatio
	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		dropProbability := e.Metrics.HealthCheckDropProbability()
		if p < dropProbability {
			ctx.Logger().Debugf("Dropping endpoint %q due to passive health check: p=%0.2f, dropProbability=%0.2f",
				e.Host, p, dropProbability)
			metrics.IncCounter("passive-health-check.endpoints.dropped")
			unhealthyEndpointsCount++
		} else {
			filtered = append(filtered, e)
		}

		if float64(unhealthyEndpointsCount) > maxUnhealthyEndpointsCount {
			return endpoints
		}
	}

	if len(filtered) == 0 {
		return endpoints
	}

	if len(filtered) < len(endpoints) {
		metrics.IncCounter("passive-health-check.requests.passed")
	}

	return filtered
}
