package routesrv

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/tracing"
)

// RouteServer is used to serve eskip-formatted routes,
// that originate from the polled data source.
type RouteServer struct {
	server *http.Server
	poller *poller
	wg     *sync.WaitGroup
}

// New returns an initialized route server according to the passed options.
// This call does not start data source updates automatically. Kept routes
// will stay in an uninitialized state, till StartUpdates is called and
// in effect data source is queried and routes initialized/updated.
func New(opts Options) (*RouteServer, error) {
	rs := &RouteServer{}

	opentracingOpts := opts.OpenTracing
	if len(opentracingOpts) == 0 {
		opentracingOpts = []string{"noop"}
	}
	tracer, err := tracing.InitTracer(opentracingOpts)
	if err != nil {
		return nil, err
	}

	b := &eskipBytes{tracer: tracer}
	bs := &eskipBytesStatus{b: b}
	handler := http.NewServeMux()
	handler.Handle("/health", bs)
	handler.Handle("/routes", b)
	handler.Handle("/metrics", promhttp.Handler())
	rs.server = &http.Server{Addr: opts.Address, Handler: handler}

	dataclient, err := kubernetes.New(kubernetes.Options{
		AllowedExternalNames:              opts.KubernetesAllowedExternalNames,
		BackendNameTracingTag:             opts.OpenTracingBackendNameTag,
		DefaultFiltersDir:                 opts.DefaultFiltersDir,
		KubernetesInCluster:               opts.KubernetesInCluster,
		KubernetesURL:                     opts.KubernetesURL,
		KubernetesNamespace:               opts.KubernetesNamespace,
		KubernetesEnableEastWest:          opts.KubernetesEnableEastWest,
		KubernetesEastWestDomain:          opts.KubernetesEastWestDomain,
		KubernetesEastWestRangeDomains:    opts.KubernetesEastWestRangeDomains,
		KubernetesEastWestRangePredicates: opts.KubernetesEastWestRangePredicates,
		HTTPSRedirectCode:                 opts.KubernetesHTTPSRedirectCode,
		IngressClass:                      opts.KubernetesIngressClass,
		OnlyAllowedExternalNames:          opts.KubernetesOnlyAllowedExternalNames,
		OriginMarker:                      opts.OriginMarker,
		PathMode:                          opts.KubernetesPathMode,
		ProvideHealthcheck:                opts.KubernetesHealthcheck,
		ProvideHTTPSRedirect:              opts.KubernetesHTTPSRedirect,
		ReverseSourcePredicate:            opts.ReverseSourcePredicate,
		RouteGroupClass:                   opts.KubernetesRouteGroupClass,
		WhitelistedHealthCheckCIDR:        opts.WhitelistedHealthCheckCIDR,
	})
	if err != nil {
		return nil, err
	}
	rs.poller = &poller{
		client:  dataclient,
		timeout: opts.SourcePollTimeout,
		b:       b,
		quit:    make(chan struct{}),
		tracer:  tracer,
	}

	rs.wg = &sync.WaitGroup{}

	return rs, nil
}

// StartUpdates starts the data source polling process.
func (rs *RouteServer) StartUpdates() {
	rs.wg.Add(1)
	go rs.poller.poll(rs.wg)
}

// StopUpdates stop the data source polling process.
func (rs *RouteServer) StopUpdates() {
	rs.poller.quit <- struct{}{}
}

// ServeHTTP serves kept eskip-formatted routes under /routes
// endpoint. Additionally it provides a simple health check under
// /health and Prometheus-compatible metrics under /metrics.
func (rs *RouteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rs.server.Handler.ServeHTTP(w, r)
}

func newShutdownFunc(rs *RouteServer) func(delay time.Duration) {
	once := &sync.Once{}
	rs.wg.Add(1)

	return func(delay time.Duration) {
		once.Do(func() {
			defer rs.wg.Done()
			defer rs.StopUpdates()

			log.Infof("shutting down the server in %s...", delay)
			time.Sleep(delay)
			if err := rs.server.Shutdown(context.Background()); err != nil {
				log.Error("unable to shut down the server: ", err)
			}
			log.Info("server shut down")
		})
	}
}

// Run starts a route server set up according to the passed options.
// It is a blocking call designed to be used as a single call/entry point,
// when running the route server as a standalone binary. It returns, when
// the server is closed, which can happen due to server startup errors or
// gracefully handled SIGTERM signal. In case of a server startup error,
// the error is returned as is.
func Run(opts Options) error {
	rs, err := New(opts)
	if err != nil {
		return err
	}

	shutdown := newShutdownFunc(rs)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		shutdown(opts.WaitForHealthcheckInterval)
	}()

	rs.StartUpdates()

	if err = rs.server.ListenAndServe(); err != http.ErrServerClosed {
		go shutdown(0)
	} else {
		err = nil
	}

	rs.wg.Wait()

	return err
}
