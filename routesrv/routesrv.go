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

type RouteServer struct {
	server *http.Server
	poller *poller
	wg     *sync.WaitGroup
}

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
		KubernetesInCluster:               opts.KubernetesInCluster,
		KubernetesURL:                     opts.KubernetesURL,
		ProvideHealthcheck:                opts.KubernetesHealthcheck,
		ProvideHTTPSRedirect:              opts.KubernetesHTTPSRedirect,
		HTTPSRedirectCode:                 opts.KubernetesHTTPSRedirectCode,
		IngressClass:                      opts.KubernetesIngressClass,
		RouteGroupClass:                   opts.KubernetesRouteGroupClass,
		ReverseSourcePredicate:            opts.ReverseSourcePredicate,
		WhitelistedHealthCheckCIDR:        opts.WhitelistedHealthCheckCIDR,
		PathMode:                          opts.KubernetesPathMode,
		KubernetesNamespace:               opts.KubernetesNamespace,
		KubernetesEastWestRangeDomains:    opts.KubernetesEastWestRangeDomains,
		KubernetesEastWestRangePredicates: opts.KubernetesEastWestRangePredicates,
		DefaultFiltersDir:                 opts.DefaultFiltersDir,
		BackendNameTracingTag:             opts.OpenTracingBackendNameTag,
		OnlyAllowedExternalNames:          opts.KubernetesOnlyAllowedExternalNames,
		AllowedExternalNames:              opts.KubernetesAllowedExternalNames,
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

func (rs *RouteServer) StartUpdates() {
	rs.wg.Add(1)
	go rs.poller.poll(rs.wg)
}

func (rs *RouteServer) StopUpdates() {
	close(rs.poller.quit)
}

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
