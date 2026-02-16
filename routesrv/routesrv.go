package routesrv

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	ot "github.com/opentracing/opentracing-go"
	sotel "github.com/zalando/skipper/otel"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel"
	otBridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/metrics"
)

// RouteServer is used to serve eskip-formatted routes,
// that originate from the polled data source.
type RouteServer struct {
	server         *http.Server
	supportServer  *http.Server
	poller         *poller
	wg             *sync.WaitGroup
	tracerShutdown func(context.Context) error
}

const otelTracerName = "routesrv"

// New returns an initialized route server according to the passed options.
// This call does not start data source updates automatically. Kept routes
// will stay in an uninitialized state, till StartUpdates is called and
// in effect data source is queried and routes initialized/updated.
func New(opts skipper.Options) (*RouteServer, error) {
	if opts.PrometheusRegistry == nil {
		opts.PrometheusRegistry = prometheus.NewRegistry()
	}

	mopt := metrics.Options{
		Format:               metrics.PrometheusKind,
		Prefix:               "routesrv",
		PrometheusRegistry:   opts.PrometheusRegistry,
		EnableDebugGcMetrics: true,
		EnableRuntimeMetrics: true,
		EnableProfile:        opts.EnableProfile,
		BlockProfileRate:     opts.BlockProfileRate,
		MutexProfileFraction: opts.MutexProfileFraction,
		MemProfileRate:       opts.MemProfileRate,
	}
	m := metrics.NewMetrics(mopt)
	metricsHandler := metrics.NewHandler(mopt, m)

	rs := &RouteServer{}

	tracer, shutdown, err := tracerInstance(&opts)
	if err != nil {
		return nil, err
	}
	rs.tracerShutdown = shutdown

	b := &eskipBytes{
		tracer:  tracer,
		metrics: m,
		now:     time.Now,
	}
	bs := &eskipBytesStatus{
		b: b,
	}
	mux := http.NewServeMux()
	mux.Handle("/health", bs)
	mux.Handle("/routes", b)
	supportHandler := http.NewServeMux()
	supportHandler.Handle("/metrics", metricsHandler)
	supportHandler.Handle("/metrics/", metricsHandler)

	if opts.EnableProfile {
		supportHandler.Handle("/debug/pprof", metricsHandler)
		supportHandler.Handle("/debug/pprof/", metricsHandler)
	}

	if !opts.Kubernetes {
		return nil, fmt.Errorf(`option "Kubernetes" is required`)
	}

	dataclient, err := kubernetes.New(opts.KubernetesDataClientOptions())
	if err != nil {
		return nil, err
	}
	var oauthConfig *auth.OAuthConfig
	if opts.EnableOAuth2GrantFlow /* explicitly enable grant flow */ {
		oauthConfig = &auth.OAuthConfig{}
		oauthConfig.CallbackPath = opts.OAuth2CallbackPath
	}

	var rh *RedisHandler
	// in case we have kubernetes dataclient and we can detect redis instances, we patch redisOptions
	if opts.KubernetesRedisServiceNamespace != "" && opts.KubernetesRedisServiceName != "" {
		log.Infof("Use endpoints %s/%s to fetch updated redis shards", opts.KubernetesRedisServiceNamespace, opts.KubernetesRedisServiceName)
		rh = &RedisHandler{}
		_, err := dataclient.LoadAll()
		if err != nil {
			return nil, err
		}
		rh.AddrUpdater = getRedisAddresses(&opts, dataclient, m)
		mux.Handle("/swarm/redis/shards", rh)
	}

	var vh *ValkeyHandler
	// in case we have kubernetes dataclient and we can detect valkey instances, we patch valkeyOptions
	if opts.KubernetesValkeyServiceNamespace != "" && opts.KubernetesValkeyServiceName != "" {
		log.Infof("Use endpoints %s/%s to fetch updated valkey shards", opts.KubernetesValkeyServiceNamespace, opts.KubernetesValkeyServiceName)
		vh = &ValkeyHandler{}
		_, err := dataclient.LoadAll()
		if err != nil {
			return nil, err
		}
		vh.AddrUpdater = getValkeyAddresses(&opts, dataclient, m)
		mux.Handle("/swarm/valkey/shards", vh)
	}

	rs.server = &http.Server{
		Addr:              opts.Address,
		Handler:           mux,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 1 * time.Minute,
	}

	rs.supportServer = &http.Server{
		Addr:              opts.SupportListener,
		Handler:           supportHandler,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 1 * time.Minute,
	}

	rs.poller = &poller{
		client:         dataclient,
		timeout:        opts.SourcePollTimeout,
		b:              b,
		quit:           make(chan struct{}),
		defaultFilters: opts.DefaultFilters,
		editRoute:      opts.EditRoute,
		cloneRoute:     opts.CloneRoute,
		oauth2Config:   oauthConfig,
		tracer:         tracer,
		metrics:        m,
	}

	rs.wg = &sync.WaitGroup{}

	return rs, nil
}

func tracerInstance(o *skipper.Options) (ot.Tracer, func(context.Context) error, error) {
	var (
		otelTracer trace.Tracer
		tracer     ot.Tracer
		shutdown   func(context.Context) error
		err        error
	)

	if o.OpenTelemetry != nil {
		shutdown, err = sotel.Init(context.Background(), o.OpenTelemetry)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup OpenTelemetry: %w", err)
		}

		// Setup OpenTracing bridge tracer
		bridgeTracer, wrapperTracerProvider := otBridge.NewTracerPair(otel.Tracer(otelTracerName))

		bridgeTracer.SetWarningHandler(func(msg string) { log.Warnf("OpenTracing bridge warning: %s", msg) })
		otel.SetTracerProvider(wrapperTracerProvider)

		tracer = bridgeTracer
		// Obtain tracer from wrappedTracerProvider
		otelTracer = otel.Tracer(otelTracerName)
	} else {
		if o.OpenTracingTracer != nil {
			tracer = o.OpenTracingTracer
		} else {
			opentracingOpts := o.OpenTracing
			if len(opentracingOpts) == 0 {
				opentracingOpts = []string{"noop"}
			}
			tracer, err = tracing.InitTracer(opentracingOpts)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to setup OpenTracing: %w", err)
			}
		}
		// Could be a noop tracer or a wrapper if library user configured OpenTracing bridge tracer
		otelTracer = otel.Tracer(otelTracerName)
		shutdown = func(context.Context) error { return nil }
	}

	_ = otelTracer // unused for now

	return tracer, shutdown, nil
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

func (rs *RouteServer) startSupportListener() {
	if rs.supportServer != nil {
		err := rs.supportServer.ListenAndServe()
		if err != nil {
			log.Errorf("Failed support listener: %v", err)
		}
	}
}

func newShutdownFunc(rs *RouteServer) func(delay time.Duration) {
	once := sync.Once{}
	rs.wg.Add(1)

	return func(delay time.Duration) {
		once.Do(func() {
			defer rs.wg.Done()
			defer rs.StopUpdates()

			log.Infof("shutting down the server in %s...", delay)
			time.Sleep(delay)
			if rs.supportServer != nil {
				if err := rs.supportServer.Shutdown(context.Background()); err != nil {
					log.Error("unable to shut down the support server: ", err)
				}
				log.Info("supportServer shut down")
			}
			if err := rs.server.Shutdown(context.Background()); err != nil {
				log.Error("unable to shut down the server: ", err)
			}
			rs.tracerShutdown(context.Background())
			log.Info("server shut down")
		})
	}
}

func run(rs *RouteServer, opts skipper.Options, sigs chan os.Signal) error {
	var err error

	shutdown := newShutdownFunc(rs)

	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		shutdown(opts.WaitForHealthcheckInterval)
	}()

	rs.StartUpdates()

	go rs.startSupportListener()
	if err = rs.server.ListenAndServe(); err != http.ErrServerClosed {
		go shutdown(0)
	} else {
		err = nil
	}

	rs.wg.Wait()

	return err
}

// Run starts a route server set up according to the passed options.
// It is a blocking call designed to be used as a single call/entry point,
// when running the route server as a standalone binary. It returns, when
// the server is closed, which can happen due to server startup errors or
// gracefully handled SIGTERM signal. In case of a server startup error,
// the error is returned as is.
func Run(opts skipper.Options) error {
	rs, err := New(opts)
	if err != nil {
		return err
	}
	sigs := make(chan os.Signal, 1)
	return run(rs, opts, sigs)
}
