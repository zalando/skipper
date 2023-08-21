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

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/filters/auth"
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
func New(opts skipper.Options) (*RouteServer, error) {
	rs := &RouteServer{}

	opentracingOpts := opts.OpenTracing
	if len(opentracingOpts) == 0 {
		opentracingOpts = []string{"noop"}
	}
	tracer, err := tracing.InitTracer(opentracingOpts)
	if err != nil {
		return nil, err
	}

	b := &eskipBytes{tracer: tracer, now: time.Now}
	bs := &eskipBytesStatus{b: b}
	handler := http.NewServeMux()
	handler.Handle("/health", bs)
	handler.Handle("/routes", b)
	handler.Handle("/metrics", promhttp.Handler())

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
		rh.AddrUpdater = getRedisAddresses(opts.KubernetesRedisServiceNamespace, opts.KubernetesRedisServiceName, dataclient)
		handler.Handle("/swarm/redis/shards", rh)
	}

	rs.server = &http.Server{
		Addr:              opts.Address,
		Handler:           handler,
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
	once := sync.Once{}
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
func Run(opts skipper.Options) error {
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
