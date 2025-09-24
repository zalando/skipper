package skipper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters"
	flog "github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	fscheduler "github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/scheduler"
	"github.com/zalando/skipper/secrets/certregistry"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	listenDelay   = 15 * time.Millisecond
	listenTimeout = 9 * listenDelay
)

func listenAndServe(proxy http.Handler, o *Options) error {
	return listenAndServeQuit(proxy, o, nil, nil, nil, nil)
}

func testListener() bool {
	for _, a := range os.Args {
		if a == "listener" {
			return true
		}
	}

	return false
}

func waitConn(req func() (*http.Response, error)) (*http.Response, error) {
	to := time.After(listenTimeout)
	for {
		rsp, err := req()
		if err == nil {
			return rsp, nil
		}

		select {
		case <-to:
			return nil, err
		default:
			time.Sleep(listenDelay)
		}
	}
}

func waitConnGet(url string) (*http.Response, error) {
	return waitConn(func() (*http.Response, error) {
		return (&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true}}}).Get(url)
	})
}

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})
	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
}

func TestOptionsFilterRegistry(t *testing.T) {
	t.Run("custom filters", func(t *testing.T) {
		o := &Options{
			CustomFilters: []filters.Spec{auth.NewBearerInjector(nil)},
		}
		fr := o.filterRegistry()

		assert.Contains(t, fr, filters.SetRequestHeaderName)
		assert.Contains(t, fr, filters.LuaName)
		assert.Contains(t, fr, filters.BearerInjectorName)
	})

	t.Run("disabled filters", func(t *testing.T) {
		o := &Options{
			CustomFilters:   []filters.Spec{auth.NewBearerInjector(nil)},
			DisabledFilters: []string{filters.LuaName, filters.BearerInjectorName},
		}
		fr := o.filterRegistry()

		assert.Contains(t, fr, filters.SetRequestHeaderName)
		assert.NotContains(t, fr, filters.LuaName)
		assert.NotContains(t, fr, filters.BearerInjectorName)
	})

	t.Run("register filters", func(t *testing.T) {
		o := &Options{
			CustomFilters: []filters.Spec{auth.NewBearerInjector(nil)},
			RegisterFilters: func(registry filters.Registry) {
				registry.Register(auth.NewWebhook(0))

				// Check that built-in and CustomFilters are already registered
				assert.Contains(t, registry, filters.SetRequestHeaderName)
				assert.Contains(t, registry, filters.BearerInjectorName)
			},
		}
		fr := o.filterRegistry()

		assert.Contains(t, fr, filters.SetRequestHeaderName)
		assert.Contains(t, fr, filters.BearerInjectorName)
		assert.Contains(t, fr, filters.WebhookName)
	})
}

func TestOptionsOpenTracingTracerInstanceOverridesOpenTracing(t *testing.T) {
	tracer := tracingtest.NewTracer()
	o := Options{
		OpenTracingTracer: tracer,
		OpenTracing:       []string{"noop"},
	}

	tr, err := o.openTracingTracerInstance()
	assert.NoError(t, err)
	assert.Same(t, tracer, tr)
}

func TestOptionsOpenTracingTracerInstanceFallbacksToOpenTracingWhenTracerIsNil(t *testing.T) {
	o := Options{
		OpenTracing: []string{"noop"},
	}

	tr, err := o.openTracingTracerInstance()
	assert.NoError(t, err)
	assert.NotNil(t, tr)
}

func TestOptionsOpenTracingTracerInstanceReturnsErrorWhenNoTracerConfigIsSpecified(t *testing.T) {
	o := Options{}

	tr, err := o.openTracingTracerInstance()
	assert.Error(t, err)
	assert.Nil(t, tr)
}

func TestOptionsTLSConfig(t *testing.T) {
	cr := certregistry.NewCertRegistry()
	proxyTLS := &tls.Config{}

	cert, err := tls.LoadX509KeyPair("fixtures/test.crt", "fixtures/test.key")
	require.NoError(t, err)

	cert2, err := tls.LoadX509KeyPair("fixtures/test2.crt", "fixtures/test2.key")
	require.NoError(t, err)

	// empty without registry
	o := &Options{}
	c, err := o.tlsConfig(nil)
	require.NoError(t, err)
	require.Nil(t, c)

	// empty with registry
	o = &Options{}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.NotNil(t, c.GetCertificate)

	// proxy tls config
	o = &Options{ProxyTLS: proxyTLS}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Same(t, proxyTLS, c)

	// proxy tls config priority
	o = &Options{ProxyTLS: proxyTLS, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Same(t, proxyTLS, c)

	// cert key path
	o = &Options{TLSMinVersion: tls.VersionTLS12, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS12), c.MinVersion)
	require.Equal(t, []tls.Certificate{cert}, c.Certificates)

	// multiple cert key paths
	o = &Options{TLSMinVersion: tls.VersionTLS13, CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test.key,fixtures/test2.key"}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), c.MinVersion)
	require.Equal(t, []tls.Certificate{cert, cert2}, c.Certificates)

	// TLS Cipher Suites
	o = &Options{CipherSuites: []uint16{1}}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	assert.Equal(t, len(c.CipherSuites), 1)

}

func TestOptionsTLSConfigInvalidPaths(t *testing.T) {
	cr := certregistry.NewCertRegistry()

	for _, tt := range []struct {
		name    string
		options *Options
	}{
		{"missing cert path", &Options{KeyPathTLS: "fixtures/test.key"}},
		{"missing key path", &Options{CertPathTLS: "fixtures/test.crt"}},
		{"wrong cert path", &Options{CertPathTLS: "fixtures/notFound.crt", KeyPathTLS: "fixtures/test.key"}},
		{"wrong key path", &Options{CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/notFound.key"}},
		{"cert key mismatch", &Options{CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test2.key"}},
		{"multiple cert key count mismatch", &Options{CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test.key"}},
		{"multiple cert key mismatch", &Options{CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test2.key,fixtures/test.key"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.options.tlsConfig(cr)
			t.Logf("tlsConfig error: %v", err)
			require.Error(t, err)
		})
	}
}

// to run this test, set `-args listener` for the test command
func TestHTTPSServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	i, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{
		Address:         a,
		InsecureAddress: i,
		CertPathTLS:     "fixtures/test.crt",
		KeyPathTLS:      "fixtures/test.key",
	}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	go listenAndServe(proxy, &o)

	r, err := waitConnGet("https://" + o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}

	r, err = waitConnGet("http://" + o.InsecureAddress)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}

// to run this test, set `-args listener` for the test command
func TestHTTPServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	go listenAndServe(proxy, &o)
	r, err := waitConnGet("http://" + o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}

func TestServerShutdownHTTP(t *testing.T) {
	o := &Options{}
	testServerShutdown(t, o, "http")
}

func TestServerShutdownHTTPS(t *testing.T) {
	o := &Options{
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	testServerShutdown(t, o, "https")
}

type responseOrError struct {
	rsp *http.Response
	err error
}

func testServerShutdown(t *testing.T, o *Options, scheme string) {
	const shutdownDelay = 1 * time.Second

	address, err := findAddress()
	require.NoError(t, err)

	o.Address, o.WaitForHealthcheckInterval = address, shutdownDelay
	testUrl := scheme + "://" + address

	// simulate a backend that got a request and should be handled correctly
	dc, err := routestring.New(`r0: * -> latency("3s") -> inlineContent("OK") -> status(200) -> <shunt>`)
	require.NoError(t, err)

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()

	sigs := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		err := listenAndServeQuit(proxy, o, sigs, done, nil, nil)
		require.NoError(t, err)
	}()

	// initiate shutdown
	sigs <- syscall.SIGTERM

	time.Sleep(shutdownDelay / 2)

	t.Logf("Make request in parallel before shutdown started")

	roeCh := make(chan responseOrError)
	go func() {
		rsp, err := waitConnGet(testUrl)
		roeCh <- responseOrError{rsp, err}
	}()

	time.Sleep(shutdownDelay)

	t.Logf("We are 1.5x past the shutdown delay, so shutdown should have been started")

	select {
	case <-roeCh:
		t.Fatalf("Request should still be in progress after shutdown started")
	default:
		_, err = waitConnGet(testUrl)
		assert.ErrorContains(t, err, "connection refused", "Another request should fail after shutdown started")
	}

	roe := <-roeCh
	require.NoError(t, roe.err, "Request must succeed")
	defer roe.rsp.Body.Close()

	body, err := io.ReadAll(roe.rsp.Body)
	require.NoError(t, err)
	assert.Equal(t, "OK", string(body))

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Errorf("Shutdown takes too long after request is finished")
	}
}

type (
	customRatelimitSpec   struct{ registry *ratelimit.Registry }
	customRatelimitFilter struct{}
)

func (s *customRatelimitSpec) Name() string { return "customRatelimit" }
func (s *customRatelimitSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	log.Infof("Registry: %v", s.registry)
	return &customRatelimitFilter{}, nil
}
func (f *customRatelimitFilter) Request(ctx filters.FilterContext)  {}
func (f *customRatelimitFilter) Response(ctx filters.FilterContext) {}

func Example_ratelimitRegistryBinding() {
	s := &customRatelimitSpec{}

	o := Options{
		Address:            ":9090",
		InlineRoutes:       `* -> customRatelimit() -> <shunt>`,
		EnableRatelimiters: true,
		EnableSwarm:        true,
		SwarmRedisURLs:     []string{":6379"},
		CustomFilters:      []filters.Spec{s},
		SwarmRegistry: func(registry *ratelimit.Registry) {
			s.registry = registry
		},
	}

	log.Fatal(Run(o))
	// Example functions without output comments are compiled but not executed
}

func createFilterRegistry(specs ...filters.Spec) filters.Registry {
	fr := make(filters.Registry)
	for _, spec := range specs {
		fr.Register(spec)
	}
	return fr
}

func createRoutesFile(route string) (string, error) {
	fd, err := os.CreateTemp("/tmp", "test_data_clients_")
	if err != nil {
		return "", fmt.Errorf("Failed to create tempfile: %w", err)
	}
	_, err = fd.WriteString(route)
	if err != nil {
		return "", fmt.Errorf("Failed to write tempfile: %w", err)
	}

	filePath := fd.Name()
	err = fd.Close()

	return filePath, err
}

func TestDataClients(t *testing.T) {
	// routesfile
	routesFileStatus := 201
	routeStringFmt := `r%d: Path("/routes-file") -> status(%d) -> inlineContent("Got it") -> <shunt>;`
	filePath, err := createRoutesFile(fmt.Sprintf(routeStringFmt, routesFileStatus, routesFileStatus))
	if err != nil {
		t.Fatalf("Failed to create routes file: %v", err)
	}
	defer os.Remove(filePath)

	// application log
	fdApp, err := os.CreateTemp("/tmp", "app_log_")
	if err != nil {
		t.Fatalf("Failed to create tempfile: %v", err)
	}
	defer fdApp.Close()

	// access log
	fdAccess, err := os.CreateTemp("/tmp", "access_log_")
	if err != nil {
		t.Fatalf("Failed to create tempfile: %v", err)
	}
	defer fdAccess.Close()

	// run skipper proxy that we want to test
	o := Options{
		Address:                         ":8090",
		EnableRatelimiters:              true,
		SourcePollTimeout:               1500 * time.Millisecond,
		WaitFirstRouteLoad:              true,
		SuppressRouteUpdateLogs:         false,
		MetricsListener:                 ":8091",
		RoutesFile:                      filePath,
		InlineRoutes:                    `healthz: Path("/healthz") -> status(200) -> inlineContent("OK") -> <shunt>;`,
		ApplicationLogOutput:            fdApp.Name(),
		AccessLogOutput:                 fdAccess.Name(),
		AccessLogDisabled:               false,
		MaxTCPListenerConcurrency:       0,
		ExpectedBytesPerRequest:         1024,
		ReadHeaderTimeoutServer:         0,
		ReadTimeoutServer:               1 * time.Second,
		MetricsFlavours:                 []string{"codahale"},
		EnablePrometheusMetrics:         true,
		LoadBalancerHealthCheckInterval: 3 * time.Second,
		OAuthTokeninfoURL:               "http://127.0.0.1:12345",
		CredentialsPaths:                []string{"/does-not-exist"},
		CompressEncodings:               []string{"gzip"},
		IgnoreTrailingSlash:             true,
		EnableBreakers:                  true,
		DebugListener:                   ":8092",
		StatusChecks:                    []string{"http://127.0.0.1:8091/metrics", "http://127.0.0.1:8092"},
	}

	dcs, err := createDataClients(o, nil)
	if err != nil {
		t.Fatalf("Failed to createDataclients: %v", err)
	}

	fr := createFilterRegistry(
		fscheduler.NewFifo(),
		flog.NewEnableAccessLog(),
		builtin.NewStatus(),
		builtin.NewInlineContent(),
	)
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	defer endpointRegistry.Close()

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     dcs, //[]routing.DataClient{dc},
		PostProcessors: []routing.PostProcessor{
			loadbalancer.NewAlgorithmProvider(),
			endpointRegistry,
			reg,
		},
		SuppressLogs: true,
	}
	rt := routing.New(ro)
	defer rt.Close()
	<-rt.FirstLoad()
	tracer := tracingtest.NewTracer()
	pr := proxy.WithParams(proxy.Params{
		Routing:     rt,
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
	})
	defer pr.Close()
	lb := stdlibhttptest.NewServer(pr)
	defer lb.Close()

	sigs := make(chan os.Signal, 1)
	go run(o, sigs, nil)

	for i := 0; i < 30; i++ {
		t.Logf("Waiting for proxy being ready")

		rsp, _ := http.DefaultClient.Get("http://localhost:8090/healthz")
		if rsp != nil && rsp.StatusCode == 200 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	rsp, err := http.DefaultClient.Get("http://localhost:8090/routes-file")
	if err != nil {
		t.Fatalf("Failed to GET routes file route: %v", err)
	}

	if rsp.StatusCode != routesFileStatus {
		t.Fatalf("Failed to GET the status of routes file route: %d", rsp.StatusCode)
	}

	sigs <- syscall.SIGTERM
}
