package routesrv

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
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

var (
	pollingStarted = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "routesrv",
		Name:      "polling_started_timestamp",
		Help:      "UNIX time when the routes polling has started",
	})
	routesInitialized = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "routesrv",
		Name:      "routes_initialized_timestamp",
		Help:      "UNIX time when the first routes were received and stored",
	})
	routesUpdated = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "routesrv",
		Name:      "routes_updated_timestamp",
		Help:      "UNIX time of the last routes update (initial load counts as well)",
	})
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

	// tracer
	tracer ot.Tracer
}

func (p *poller) poll(wg *sync.WaitGroup) {
	defer wg.Done()

	log.WithField("timeout", p.timeout).Info(LogPollingStarted)
	ticker := time.NewTicker(p.timeout)
	defer ticker.Stop()
	pollingStarted.SetToCurrentTime()

	var lastRoutesById map[string]string
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()

		routes = p.process(routes)

		routesCount := len(routes)

		switch {
		case err != nil:
			log.WithError(err).Error(LogRoutesFetchingFailed)

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", fmt.Sprintf("%s: %s", LogRoutesFetchingFailed, err),
			)
		case routesCount == 0:
			log.Error(LogRoutesEmpty)

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
				routesInitialized.SetToCurrentTime()
			}
			if updated {
				logger.Info(LogRoutesUpdated)
				span.SetTag("routes.updated", true)
				routesUpdated.SetToCurrentTime()
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

func mapRoutes(routes []*eskip.Route) map[string]string {
	byId := make(map[string]string)
	for _, r := range routes {
		byId[r.Id] = r.String()
	}
	return byId
}

func logChanges(routesById map[string]string, lastRoutesById map[string]string) {
	added := notIn(routesById, lastRoutesById)
	for i, id := range added {
		log.Debugf("added (%d/%d): %s", i+1, len(added), id)
	}

	removed := notIn(lastRoutesById, routesById)
	for i, id := range removed {
		log.Debugf("removed (%d/%d): %s", i+1, len(removed), id)
	}

	changed := valueMismatch(routesById, lastRoutesById)
	for i, id := range changed {
		log.Debugf("changed (%d/%d): %s", i+1, len(changed), id)
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
