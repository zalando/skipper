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
	lastUpdated time.Time
	tracer      ot.Tracer
}

func (e *eskipBytes) bytes() []byte {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.data
}

func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (bool, int) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{}, routes...)

	e.mu.Lock()
	defer e.mu.Unlock()
	oldData := e.data
	e.data = buf.Bytes()
	e.lastUpdated = time.Now()

	return oldData == nil, len(e.data)
}

func (e *eskipBytes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := tracing.CreateSpan("serve_routes", context.TODO(), e.tracer)
	defer span.Finish()

	data := e.bytes()
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
	} else {
		w.Write(data)
	}
}

// eskipBytesStatus provide metadata about the state of the referenced eskipBytes.
// It can also serve as an HTTP health check for it (only reports healthy when the bytes
// were initialized).
type eskipBytesStatus struct {
	b *eskipBytes
}

var msgRoutesNotInitialized = []byte("routes were not initialized yet")

func (s *eskipBytesStatus) initialized() bool {
	return s.b.bytes() != nil
}

func (s *eskipBytesStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.initialized() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write(msgRoutesNotInitialized)
	}
}

func pollRoutes(client routing.DataClient, timeout time.Duration, b *eskipBytes, quit chan struct{}, tracer ot.Tracer) {
	var (
		routesLen   int
		msg         string
		initialized bool
		size        int
	)

	log.Infof("starting polling with timeout %s", timeout)
	for {
		span := tracing.CreateSpan("poll_routes", context.TODO(), tracer)

		routes, err := client.LoadAll()
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
			initialized, size = b.formatAndSet(routes)
			if initialized {
				log.Info("routes initialized")
				span.SetTag("initialized", true)
			} else {
				log.Debug("routes updated")
			}
			span.SetTag("routes.received_len", routesLen)
			span.SetTag("routes.stored_bytes", size)
		}

		span.Finish()

		select {
		case <-quit:
			return
		case <-time.After(timeout):
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

	b := &eskipBytes{}
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
	quit := make(chan struct{}, 1)
	go pollRoutes(dataclient, o.SourcePollTimeout, b, quit, tracer)

	server := newServer(o.Address, b, s)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Info("shutting down")
		close(quit)
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
