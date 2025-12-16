package proxy

import (
	"math/rand/v2"
	"sync"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type healthyEndpoints struct {
	mu  sync.Mutex
	rnd *rand.Rand

	maxUnhealthyEndpointsRatio float64
}

func (h *healthyEndpoints) filterHealthyEndpoints(ctx *context, endpoints []routing.LBEndpoint, metrics metrics.Metrics) []routing.LBEndpoint {
	if h == nil {
		return endpoints
	}

	span := ot.SpanFromContext(ctx.request.Context())

	h.mu.Lock()
	random := h.rnd.Float64()
	h.mu.Unlock()

	unhealthyEndpointsCount := 0
	maxUnhealthyEndpointsCount := float64(len(endpoints)) * h.maxUnhealthyEndpointsRatio
	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		dropProbability := e.Metrics.HealthCheckDropProbability()
		if random < dropProbability {
			ctx.Logger().Debugf("Dropping endpoint %q due to passive health check: p=%0.2f, dropProbability=%0.2f",
				e.Host, random, dropProbability)
			metrics.IncCounter("passive-health-check.endpoints.dropped")
			unhealthyEndpointsCount++
		} else {
			filtered = append(filtered, e)
		}

		if float64(unhealthyEndpointsCount) > maxUnhealthyEndpointsCount {
			if span != nil {
				span.SetTag("phc.endpoints.insufficient", true)
			}
			return endpoints
		}
	}

	if len(filtered) == 0 {
		if span != nil {
			span.SetTag("phc.endpoints.insufficient", true)
		}
		return endpoints
	}

	if len(filtered) < len(endpoints) {
		if span != nil {
			span.SetTag("phc.endpoints.dropped", true)
			span.LogKV(
				"event", "phc",
				"dropped_endpoints", unhealthyEndpointsCount,
			)
		}
		metrics.IncCounter("passive-health-check.requests.passed")
	}

	return filtered
}
