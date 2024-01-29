package proxy

import (
	"math"
	"math/rand"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

func returnFast(rt *routing.Route) bool {
	if rt.BackendType != eskip.LBBackend {
		return true
	}

	return rt.LBFadeInDuration <= 0
}

func fadeIn(lifetime time.Duration, duration time.Duration, exponent float64) float64 {
	fadingIn := lifetime > 0 && lifetime < duration
	if !fadingIn {
		return 1
	}

	return math.Pow(float64(lifetime)/float64(duration), exponent)
}

func filterFadeIn(endpoints []routing.LBEndpoint, rt *routing.Route, registry *routing.EndpointRegistry, rnd *rand.Rand) []routing.LBEndpoint {
	if returnFast(rt) {
		return endpoints
	}

	now := time.Now()
	threshold := rnd.Float64()

	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		f := fadeIn(
			now.Sub(e.Metrics.DetectedTime()),
			rt.LBFadeInDuration,
			rt.LBFadeInExponent,
		)
		if threshold < f {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == 0 {
		return endpoints
	}
	return filtered
}
