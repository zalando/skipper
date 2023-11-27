package routesrv

import (
	"context"
	"fmt"
	"sort"
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

	var lastRoutesById map[string]string
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()
		routes = p.process(routes)
		routesCount := len(routes)

		switch {
		case err != nil:
			log.WithError(err).Error(LogRoutesFetchingFailed)
			p.metrics.IncCounter("routes.fetch_errors")

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", fmt.Sprintf("%s: %s", LogRoutesFetchingFailed, err),
			)
		case routesCount == 0:
			log.Error(LogRoutesEmpty)
			p.metrics.IncCounter("routes.empty")

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", LogRoutesEmpty,
			)
		case routesCount > 0:
			routesBytes, routesEtag, initialized, updated := p.b.formatAndSet(routes)
			logger := log.WithFields(log.Fields{"count": routesCount, "bytes": routesBytes, "etag": routesEtag})
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
			span.SetTag("routes.count", routesCount)
			span.SetTag("routes.bytes", routesBytes)
			span.SetTag("routes.etag", routesEtag)

			if updated && log.IsLevelEnabled(log.DebugLevel) {
				routesById := mapRoutes(routes)
				logChanges(routesById, lastRoutesById)
				lastRoutesById = routesById
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

	// sort the routes, otherwise it will lead to different etag values for the same route list for different orders
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Id < routes[j].Id
	})

	return routes
}

func (p *poller) setGaugeToCurrentTime(name string) {
	p.metrics.UpdateGauge(name, (float64(time.Now().UnixNano()) / 1e9))
}

func mapRoutes(routes []*eskip.Route) map[string]string {
	byId := make(map[string]string)
	for _, r := range routes {
		byId[r.Id] = r.String()
	}
	return byId
}

func logChanges(routesById map[string]string, lastRoutesById map[string]string) {
	inserted := notIn(routesById, lastRoutesById)
	for i, id := range inserted {
		log.WithFields(log.Fields{
			"op":    "inserted",
			"id":    id,
			"route": routesById[id],
		}).Debugf("Inserted route %d of %d", i+1, len(inserted))
	}

	deleted := notIn(lastRoutesById, routesById)
	for i, id := range deleted {
		log.WithFields(log.Fields{
			"op":    "deleted",
			"id":    id,
			"route": lastRoutesById[id],
		}).Debugf("Deleted route %d of %d", i+1, len(deleted))
	}

	updated := valueMismatch(routesById, lastRoutesById)
	for i, id := range updated {
		log.WithFields(log.Fields{
			"op":    "updated",
			"id":    id,
			"route": routesById[id],
		}).Debugf("Updated route %d of %d", i+1, len(updated))
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
