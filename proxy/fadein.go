package proxy

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/zalando/skipper/routing"
)

type fadeIn struct {
	mu  sync.Mutex
	rnd *rand.Rand
}

func (f *fadeIn) fadeInScore(lifetime time.Duration, duration time.Duration, exponent float64) float64 {
	fadingIn := lifetime > 0 && lifetime < duration
	if !fadingIn {
		return 1
	}

	return math.Pow(float64(lifetime)/float64(duration), exponent)
}

func (f *fadeIn) filterFadeIn(endpoints []routing.LBEndpoint, rt *routing.Route) []routing.LBEndpoint {
	if rt.LBFadeInDuration <= 0 {
		return endpoints
	}

	now := time.Now()
	f.mu.Lock()
	threshold := f.rnd.Float64()
	f.mu.Unlock()

	filtered := make([]routing.LBEndpoint, 0, len(endpoints))
	for _, e := range endpoints {
		age := now.Sub(e.Metrics.DetectedTime())
		f := f.fadeInScore(
			age,
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
