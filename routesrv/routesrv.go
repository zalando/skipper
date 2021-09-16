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

func Run(o Options) error {
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
