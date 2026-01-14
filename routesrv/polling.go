package routesrv

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	ot "github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

const (
	LogPollingStarted       = "starting polling"
	LogPollingStopped       = "polling stopped"
	LogRoutesFetchingFailed = "failed to fetch routes"
	LogRoutesEmpty          = "received empty routes; ignoring"
	LogRoutesInitialized    = "routes initialized"
	LogRoutesUpdated        = "routes updated"
	LogRoutesNoChange       = "routes did not change"
)

type poller struct {
	client  routing.DataClient
	b       *eskipBytes
	timeout time.Duration
	quit    chan struct{}

	// Preprocessors
	defaultFilters *eskip.DefaultFilters
	oauth2Config   *auth.OAuthConfig
	editRoute      []*eskip.Editor
	cloneRoute     []*eskip.Clone

	// visibility
	tracer  ot.Tracer
	metrics metrics.Metrics
}

func (p *poller) poll(wg *sync.WaitGroup) {
	defer wg.Done()

	log.WithField("timeout", p.timeout).Info(LogPollingStarted)
	ticker := time.NewTicker(p.timeout)
	defer ticker.Stop()
	p.setGaugeToCurrentTime("polling_started_timestamp")

	var (
		lastRoutesByID map[string]string
		lastRoutes     []*eskip.Route
	)

	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()
		if err != nil {
			log.WithError(err).Error(LogRoutesFetchingFailed)
			p.metrics.IncCounter("routes.fetch_errors")

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", fmt.Sprintf("%s: %s", LogRoutesFetchingFailed, err),
			)

		} else if hasChanged(lastRoutes, routes) {
			lastRoutes = routes

			routes = p.process(routes)
			routesCount := len(routes)
			span.SetTag("routes.count", routesCount)

			switch {
			case routesCount == 0:
				p.handleEmptyRoutes()
			case routesCount > 0:
				routesBytes, routesHash, initialized, updated := p.b.formatAndSet(routes)
				logger := log.WithFields(log.Fields{"count": routesCount, "bytes": routesBytes, "hash": routesHash})
				if initialized {
					logger.Info(LogRoutesInitialized)
					span.SetTag("routes.initialized", true)
					p.setGaugeToCurrentTime("routes.initialized_timestamp")
				}
				if updated {
					logger.Info(LogRoutesUpdated)
					span.SetTag("routes.updated", true)
					p.setGaugeToCurrentTime("routes.updated_timestamp")
					p.metrics.UpdateGauge("routes.total", float64(routesCount))
					p.metrics.UpdateGauge("routes.byte", float64(routesBytes))
				}
				span.SetTag("routes.bytes", routesBytes)
				span.SetTag("routes.hash", routesHash)

				if updated && log.IsLevelEnabled(log.DebugLevel) {
					routesByID := mapRoutes(routes)
					logChanges(routesByID, lastRoutesByID)
					lastRoutesByID = routesByID
				}
			}
		} else {
			log.Info(LogRoutesNoChange)
			span.SetTag("routes.updated", false)
			span.SetTag("routes.count", len(lastRoutes))
			if len(routes) == 0 {
				p.handleEmptyRoutes()
			}
		}

		span.Finish()

		select {
		case <-p.quit:
			log.Info(LogPollingStopped)
			return
		case <-ticker.C:
		}
	}
}

func (p *poller) handleEmptyRoutes() {
	log.Info(LogRoutesEmpty)
	p.metrics.IncCounter("routes.empty")
}

func (p *poller) process(routes []*eskip.Route) []*eskip.Route {

	if p.defaultFilters != nil {
		routes = p.defaultFilters.Do(routes)
	}
	if p.oauth2Config != nil {
		routes = p.oauth2Config.NewGrantPreprocessor().Do(routes)
	}
	for _, editor := range p.editRoute {
		routes = editor.Do(routes)
	}

	for _, cloner := range p.cloneRoute {
		routes = cloner.Do(routes)
	}

	// sort the routes; otherwise, it will lead to different etag values for the same route list for different orders
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Id < routes[j].Id
	})

	return routes
}

func (p *poller) setGaugeToCurrentTime(name string) {
	p.metrics.UpdateGauge(name, (float64(time.Now().UnixNano()) / 1e9))
}

func hasChanged(lastRoutes, newRoutes []*eskip.Route) bool {
	if len(lastRoutes) != len(newRoutes) {
		return true
	}
	sort.SliceStable(newRoutes, func(i, j int) bool {
		comp := strings.Compare(newRoutes[i].Id, newRoutes[j].Id)
		switch comp {
		case 0:
			return true
		case 1:
			return true
		case -1:
			return false
		}
		log.Fatalf("Failed to run strings.Compare(%q,%q)", newRoutes[i].Id, newRoutes[j].Id)
		return true // never happens
	})

	return !eskip.EqLists(lastRoutes, newRoutes)
}

func mapRoutes(routes []*eskip.Route) map[string]string {
	byID := make(map[string]string)
	for _, r := range routes {
		byID[r.Id] = r.String()
	}
	return byID
}

func logChanges(routesByID map[string]string, lastRoutesByID map[string]string) {
	logf := func(op string, id string, format string, args ...any) {
		level := log.GetLevel()
		fields := log.Fields{"op": op, "id": id}
		if level == log.TraceLevel {
			fields["route"] = routesByID[id]
		}
		log.WithFields(fields).Logf(level, format, args...)
	}

	inserted := notIn(routesByID, lastRoutesByID)
	for i, id := range inserted {
		logf("inserted", id, "Inserted route %d of %d", i+1, len(inserted))
	}

	deleted := notIn(lastRoutesByID, routesByID)
	for i, id := range deleted {
		logf("deleted", id, "Deleted route %d of %d", i+1, len(deleted))
	}

	updated := valueMismatch(routesByID, lastRoutesByID)
	for i, id := range updated {
		logf("updated", id, "Updated route %d of %d", i+1, len(updated))
	}
}

func notIn(a, b map[string]string) []string {
	var ids []string
	for id := range a {
		if _, ok := b[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func valueMismatch(a, b map[string]string) []string {
	var ids []string
	for id, va := range a {
		if vb, ok := b[id]; ok && va != vb {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
