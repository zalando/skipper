package builtin

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

const (
	maxAge        = 2
	metricsPrefix = "routeCreationTime."
)

// RouteCreationMetrics reports metrics about the time it took to create metrics.
// It looks for filters of type OriginMarker to determine when the source object (e.g. ingress) of the route
// was created.
// If an OriginMarker with the same type and id is seen again, the creation time is not reported again, because
// a route with the same configuration already existed before.
type RouteCreationMetrics struct {
	metrics      filters.Metrics
	originIdAges map[string]map[string]int
	initialized  bool
}

func NewRouteCreationMetrics(metrics filters.Metrics) *RouteCreationMetrics {
	return &RouteCreationMetrics{metrics: metrics, originIdAges: map[string]map[string]int{}}
}

// Do implements routing.PostProcessor and records the filter creation time.
func (m *RouteCreationMetrics) Do(routes []*routing.Route) []*routing.Route {
	for _, r := range routes {
		for origin, start := range m.startTimes(r) {
			if m.initialized {
				m.metrics.MeasureSince(metricsPrefix+origin, start)
			}
		}
	}

	if !m.initialized {
		//must be done after filling the cache
		m.initialized = true
	}

	m.pruneCache()

	return routes
}

func (m *RouteCreationMetrics) startTimes(route *routing.Route) map[string]time.Time {
	startTimes := map[string]time.Time{}

	for _, f := range route.Filters {
		origin, t := m.originStartTime(f.Filter)

		if t.IsZero() {
			continue
		}

		old, exists := startTimes[origin]
		if !exists || t.Before(old) {
			startTimes[origin] = t
		}
	}

	return startTimes
}

func (m *RouteCreationMetrics) originStartTime(f filters.Filter) (string, time.Time) {
	marker, ok := f.(*OriginMarker)

	if !ok {
		return "", time.Time{}
	}

	origin := marker.Origin
	id := marker.Id
	created := marker.Created
	if origin == "" || id == "" || created.IsZero() {
		return "", time.Time{}
	}

	originCache := m.originIdAges[origin]
	if originCache == nil {
		originCache = map[string]int{}
		m.originIdAges[origin] = originCache
	}

	_, exists := originCache[id]
	originCache[id] = 0
	if exists {
		return "", time.Time{}
	}

	log.WithFields(log.Fields{
		"origin":  origin,
		"id":      id,
		"seconds": time.Since(created).Seconds(),
	}).Debug("route creation time")

	return origin, created
}

func (m *RouteCreationMetrics) pruneCache() {
	for origin, idAges := range m.originIdAges {
		for id, age := range idAges {
			age++
			if age > maxAge {
				log.WithFields(log.Fields{
					"origin": origin,
					"id":     id,
				}).Debug("delete from route creation cache")

				delete(idAges, id)
			} else {
				idAges[id] = age
			}
		}

		if len(idAges) == 0 {
			log.WithField("origin", origin).Debug("delete from route creation cache")

			delete(m.originIdAges, origin)
		}
	}
}
