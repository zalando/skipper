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
	initialized bool
	mu          sync.RWMutex

	tracer ot.Tracer
}

// bytes returns a slice to stored bytes, which are safe for reading,
// and if there were already initialized.
func (e *eskipBytes) bytes() ([]byte, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.data, e.initialized
}

// formatAndSet takes a slice of routes and stores them eskip-formatted
// in a synchronized way. It returns a number of stored bytes and a boolean,
// being true, when the stored bytes were set for the first time.
func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (int, bool) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{}, routes...)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.data = buf.Bytes()
	oldInitialized := e.initialized
	e.initialized = true

	return len(e.data), !oldInitialized
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
	pollingStarted    prometheus.Gauge
	routesInitialized prometheus.Gauge
	routesUpdated     prometheus.Gauge
}

func newPollerMetrics() *pollerMetrics {
	return &pollerMetrics{
		pollingStarted: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
			Name:      "polling_started_timestamp",
			Help:      "UNIX time when the routes polling has started",
		}),
		routesInitialized: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
			Name:      "routes_initialized_timestamp",
			Help:      "UNIX time when the first routes were received and stored",
		}),
		routesUpdated: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "routesrv",
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

func (p *poller) poll(wg *sync.WaitGroup) {
	defer wg.Done()

	var (
		routesCount, routesBytes int
		initialized              bool
		msg                      string
	)

	log.Infof("starting polling with timeout %s", p.timeout)
	p.metrics.pollingStarted.SetToCurrentTime()
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), p.tracer)

		routes, err := p.client.LoadAll()
		routesCount = len(routes)

		switch {
		case err != nil:
			msg = fmt.Sprintf("failed to fetch routes: %s", err)

			log.Errorf(msg)

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", msg,
			)
		case routesCount == 0:
			msg = "received empty routes; ignoring"

			log.Error(msg)

			span.SetTag("error", true)
			span.LogKV(
				"event", "error",
				"message", msg,
			)
		case routesCount > 0:
			routesBytes, initialized = p.b.formatAndSet(routes)
			if initialized {
				log.Info("routes initialized")
				span.SetTag("routes.initialized", true)
				p.metrics.routesInitialized.SetToCurrentTime()
			} else {
				log.Info("routes updated")
			}
			p.metrics.routesUpdated.SetToCurrentTime()
			span.SetTag("routes.count", routesCount)
			span.SetTag("routes.bytes", routesBytes)
		}

		span.Finish()

		select {
		case <-p.quit:
			log.Info("polling stopped")
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

type shutdownFunc func(delay time.Duration)

func newShutdownFunc(wg *sync.WaitGroup, poller *poller, server *http.Server) shutdownFunc {
	once := &sync.Once{}
	wg.Add(1)

	return func(delay time.Duration) {
		once.Do(func() {
			defer wg.Done()
			defer close(poller.quit)

			log.Infof("shutting down the server in %s...", delay)
			time.Sleep(delay)
			if err := server.Shutdown(context.Background()); err != nil {
				log.Error("unable to shut down the server: ", err)
			}
			log.Info("server shut down")
		})
	}
}

func run(o options.Options) error {
	tracer, err := tracing.InitTracer(o.OpenTracing)
	if err != nil {
		return err
	}

	b := &eskipBytes{tracer: tracer}
	s := &eskipBytesStatus{b: b}

	wg := &sync.WaitGroup{}

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
		KubernetesEastWestRangeDomains:    o.KubernetesEastWestRangeDomains,
		KubernetesEastWestRangePredicates: o.KubernetesEastWestRangePredicates,
		DefaultFiltersDir:                 o.DefaultFiltersDir,
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
		quit:    make(chan struct{}),
		tracer:  tracer,
		metrics: newPollerMetrics(),
	}
	wg.Add(1)
	go poller.poll(wg)

	server := newServer(o.Address, b, s)

	shutdown := newShutdownFunc(wg, poller, server)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		shutdown(o.WaitForHealthcheckInterval)
	}()

	if err = server.ListenAndServe(); err != http.ErrServerClosed {
		go shutdown(0)
	} else {
		err = nil
	}

	wg.Wait()

	return err
}

func main() {
	cfg := config.NewConfig()
	cfg.Parse()
	log.SetLevel(cfg.ApplicationLogLevel)
	err := run(cfg.ToRouteSrvOptions())
	if err != nil {
		log.Fatal(err)
	}
}
