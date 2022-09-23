package routesrv

import (
	"context"
	"fmt"
	"sync"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
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
	client         routing.DataClient
	b              *eskipBytes
	timeout        time.Duration
	quit           chan struct{}
	defaultFilters *eskip.DefaultFilters

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
	pollingStarted.SetToCurrentTime()
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()
		if p.defaultFilters != nil {
			routes = p.defaultFilters.Do(routes)
		}
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
		case <-time.After(p.timeout):
		}
	}
}
