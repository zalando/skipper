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

	var (
		routesCount, routesBytes int
		initialized              bool
		msg                      string
	)

	log.WithField("timeout", p.timeout).Info(LogPollingStarted)
	ticker := time.NewTicker(p.timeout)
	defer ticker.Stop()
	pollingStarted.SetToCurrentTime()

	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()

		routes = p.process(routes)

		routesCount = len(routes)

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
				"message", msg,
			)
		case routesCount > 0:
			routesBytes, initialized = p.b.formatAndSet(routes)
			logger := log.WithFields(log.Fields{"count": routesCount, "bytes": routesBytes})
			if initialized {
				logger.Info(LogRoutesInitialized)
				span.SetTag("routes.initialized", true)
				routesInitialized.SetToCurrentTime()
			} else {
				logger.Info(LogRoutesUpdated)
			}
			routesUpdated.SetToCurrentTime()
			span.SetTag("routes.count", routesCount)
			span.SetTag("routes.bytes", routesBytes)
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
