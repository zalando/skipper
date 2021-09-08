package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/cmd/routesrv/options"
	"github.com/zalando/skipper/config"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

// eskipBytes keeps eskip-formatted routes as a byte slice and
// provides synchronized r/w access to them. Additionally it can
// serve as an HTTP handler exposing its content.
type eskipBytes struct {
	data        []byte
	mu          sync.RWMutex
	initialized bool

	tracer ot.Tracer
}

func (e *eskipBytes) bytes() ([]byte, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.data, e.initialized
}

func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (int, bool) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{}, routes...)

	e.mu.Lock()
	oldData := e.data
	e.data = buf.Bytes()
	e.initialized = true
	e.mu.Unlock()

	return len(oldData), oldData == nil
}

func (e *eskipBytes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := tracing.CreateSpan("serve_routes", r.Context(), e.tracer)
	defer span.Finish()

	if data, initialized := e.bytes(); initialized {
		w.Write(data)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

// eskipBytesStatus serves as an HTTP health check for the referenced eskipBytes.
// Reports healthy only when the bytes were initialized (set at least once).
type eskipBytesStatus struct {
	b *eskipBytes
}

const msgRoutesNotInitialized = "routes were not initialized yet"

func (s *eskipBytesStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, initialized := s.b.bytes(); initialized {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, msgRoutesNotInitialized, http.StatusServiceUnavailable)
	}
}

type pollerMetrics struct {
	startedTimestamp     prometheus.Gauge
	initializedTimestamp prometheus.Gauge
	updatedTimestamp     prometheus.Gauge
}

func newPollerMetrics() *pollerMetrics {
	return &pollerMetrics{
		startedTimestamp: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
			Subsystem: "poller",
			Name:      "started_timestamp",
			Help:      "UNIX time when the routes polling has started",
		}),
		initializedTimestamp: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
			Subsystem: "poller",
			Name:      "routes_initialized_timestamp",
			Help:      "UNIX time when the first routes were received and stored",
		}),
		updatedTimestamp: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
			Subsystem: "poller",
			Name:      "routes_updated_timestamp",
			Help:      "UNIX time of the last routes update (initial load counts as well)",
		}),
	}
}

type poller struct {
	client  routing.DataClient
	b       *eskipBytes
	timeout time.Duration
	quit    chan struct{}

	tracer  ot.Tracer
	metrics *pollerMetrics
}

func (p *poller) poll() {
	var (
		routesLen   int
		msg         string
		initialized bool
		size        int
	)

	log.Infof("starting polling with timeout %s", p.timeout)
	p.metrics.startedTimestamp.SetToCurrentTime()
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()
		routesLen = len(routes)

		switch {
		case err != nil:
			msg = fmt.Sprintf("failed to fetch routes: %s", err)

			log.Errorf(msg)

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", msg,
			)
		case routesLen == 0:
			msg = "received empty routes; ignoring"

			log.Error(msg)

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", msg,
			)
		case routesLen > 0:
			size, initialized = p.b.formatAndSet(routes)
			if initialized {
				log.Info("routes initialized")
				span.SetTag("initialized", true)
				p.metrics.initializedTimestamp.SetToCurrentTime()
			} else {
				log.Debug("routes updated")
			}
			p.metrics.updatedTimestamp.SetToCurrentTime()
			span.SetTag("routes.count", routesLen)
			span.SetTag("routes.bytes", size)
		}

		span.Finish()

		select {
		case <-p.quit:
			return
		case <-time.After(p.timeout):
		}
	}
}

func newServer(address string, b *eskipBytes, s *eskipBytesStatus) *http.Server {
	handler := http.NewServeMux()

	handler.Handle("/health", s)
	handler.Handle("/routes", b)
	handler.Handle("/metrics", promhttp.Handler())

	return &http.Server{Addr: address, Handler: handler}
}

func run(o options.Options) error {
	tracer, err := tracing.InitTracer(o.OpenTracing)
	if err != nil {
		return err
	}

	b := &eskipBytes{tracer: tracer}
	s := &eskipBytesStatus{b: b}

	dataclient, err := kubernetes.New(kubernetes.Options{
		KubernetesInCluster:               o.KubernetesInCluster,
		KubernetesURL:                     o.KubernetesURL,
		ProvideHealthcheck:                o.KubernetesHealthcheck,
		ProvideHTTPSRedirect:              o.KubernetesHTTPSRedirect,
		HTTPSRedirectCode:                 o.KubernetesHTTPSRedirectCode,
		IngressClass:                      o.KubernetesIngressClass,
		RouteGroupClass:                   o.KubernetesRouteGroupClass,
		ReverseSourcePredicate:            o.ReverseSourcePredicate,
		WhitelistedHealthCheckCIDR:        o.WhitelistedHealthCheckCIDR,
		PathMode:                          o.KubernetesPathMode,
		KubernetesNamespace:               o.KubernetesNamespace,
		KubernetesEnableEastWest:          o.KubernetesEnableEastWest,
		KubernetesEastWestDomain:          o.KubernetesEastWestDomain,
		KubernetesEastWestRangeDomains:    o.KubernetesEastWestRangeDomains,
		KubernetesEastWestRangePredicates: o.KubernetesEastWestRangePredicates,
		DefaultFiltersDir:                 o.DefaultFiltersDir,
		OriginMarker:                      o.EnableRouteCreationMetrics,
		BackendNameTracingTag:             o.OpenTracingBackendNameTag,
		OnlyAllowedExternalNames:          o.KubernetesOnlyAllowedExternalNames,
		AllowedExternalNames:              o.KubernetesAllowedExternalNames,
	})
	if err != nil {
		return err
	}
	poller := &poller{
		client:  dataclient,
		timeout: o.SourcePollTimeout,
		b:       b,
		quit:    make(chan struct{}, 1),
		tracer:  tracer,
		metrics: newPollerMetrics(),
	}
	go poller.poll()

	server := newServer(o.Address, b, s)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Info("shutting down")
		close(poller.quit)
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error("unable to shut down the server: ", err)
		}
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Error("unable to start the server: ", err)
	}

	return nil
}

func main() {
	cfg := config.NewConfig()
	cfg.Parse()
	log.SetLevel(cfg.ApplicationLogLevel)
	log.Fatal(run(cfg.ToRouteSrvOptions()))
}
