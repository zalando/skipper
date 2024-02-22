package proxy

import "github.com/zalando/skipper/routing"

type Healthchecker struct {
	Registry routing.EndpointRegistry
}

// TODO: run this periodically to update healthcheck stats
func (h *Healthchecker) update() {
	h.Registry.Visit(func(m routing.Metrics) {
		total, failed := m.TotalRequests(), m.FailedRequests()
		fp := 0.0
		if total > 10 { // require min total requests (TODO: configurable)
			fr := float64(failed) / float64(total) // safe as total > 0
			fr = min(fr, 0.90)                     // cap the failed request ratio to 90% (TODO: configurable)
			fp = fr                                // drop endpoint with probability proportional to the failed request ratio
		}

		m.ResetTotalRequests()
		m.ResetFailedRequests()
		m.SetFailProbability(fp)
	})
}
