package skipper

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters"
	flog "github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	fscheduler "github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routesrv"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
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
	o := &Options{
		CustomFilters: []filters.Spec{auth.NewBearerInjector(nil)},
	}
	fr := o.filterRegistry()

	assert.Contains(t, fr, filters.SetRequestHeaderName)
	assert.Contains(t, fr, filters.LuaName)
	assert.Contains(t, fr, filters.BearerInjectorName)

	o = &Options{
		CustomFilters:   []filters.Spec{auth.NewBearerInjector(nil)},
		DisabledFilters: []string{filters.LuaName, filters.BearerInjectorName},
	}
	fr = o.filterRegistry()

	assert.Contains(t, fr, filters.SetRequestHeaderName)
	assert.NotContains(t, fr, filters.LuaName)
	assert.NotContains(t, fr, filters.BearerInjectorName)
}

func TestOptionsTLSConfig(t *testing.T) {
	cr := certregistry.NewCertRegistry()

	cert, err := tls.LoadX509KeyPair("fixtures/test.crt", "fixtures/test.key")
	require.NoError(t, err)

	cert2, err := tls.LoadX509KeyPair("fixtures/test2.crt", "fixtures/test2.key")
	require.NoError(t, err)

	// empty
	o := &Options{}
	c, err := o.tlsConfig(cr)
	require.NoError(t, err)
	require.Nil(t, c)

	// enable kubernetes tls
	o = &Options{KubernetesEnableTLS: true}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.NotNil(t, c.GetCertificate)

	// proxy tls config
	o = &Options{ProxyTLS: &tls.Config{}}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Equal(t, &tls.Config{}, c)

	// proxy tls config priority
	o = &Options{ProxyTLS: &tls.Config{}, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}
	c, err = o.tlsConfig(cr)
	require.NoError(t, err)
	require.Equal(t, &tls.Config{}, c)

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

func TestHTTPServerShutdown(t *testing.T) {
	o := &Options{}
	testServerShutdown(t, o, "http")
}

func TestHTTPSServerShutdown(t *testing.T) {
	o := &Options{
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	testServerShutdown(t, o, "https")
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
	go func() {
		err := listenAndServeQuit(proxy, o, sigs, nil, nil, nil)
		require.NoError(t, err)
	}()

	// initiate shutdown
	sigs <- syscall.SIGTERM

	time.Sleep(shutdownDelay / 2)

	t.Logf("ongoing request passing in before shutdown")
	r, err := waitConnGet(testUrl)
	require.NoError(t, err)
	require.Equal(t, 200, r.StatusCode)

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.Equal(t, "OK", string(body))

	time.Sleep(shutdownDelay / 2)

	t.Logf("request after shutdown should fail")
	_, err = waitConnGet(testUrl)
	require.Error(t, err)
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

func TestConcurrentKubernetesClusterStateAccessWithRemoteRedis(t *testing.T) {
	redisPortFmt := `  - port: %s
    protocol: TCP
`
	redisEpSpecFmt := `
---
apiVersion: v1
kind: Endpoints
metadata:
  labels:
    application: skipper-ingress-redis
  name: redis
  namespace: skipper
subsets:
- addresses:
  - hostname: redis-%d.skipper.cluster.local
    ip: %s
  ports:
`

	kubeSpec := `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: target
spec:
  backends:
  - name: shunt
    type: shunt
  defaultBackends:
  - backendName: shunt
  routes:
  - pathSubtree: /test
    filters:
    - disableAccessLog()
    - clusterRatelimit("foo", 1, "1s")
    - status(200)
    - inlineContent("OK")
---
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: myapp
spec:
  hosts:
  - example.org
  backends:
  - name: myapp
    type: service
    serviceName: myapp
    servicePort: 80
  defaultBackends:
  - backendName: myapp
---
apiVersion: v1
kind: Service
metadata:
  name: myapp
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    application: myapp
  type: ClusterIP
---
apiVersion: v1
kind: Endpoints
metadata:
  name: myapp
subsets:
- addresses:
  - ip: 10.2.4.8
  - ip: 10.2.4.16
  ports:
  - port: 80
---
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-ingress-redis
  name: redis
  namespace: skipper
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-ingress-redis
  type: ClusterIP
`

	redis1, done1 := redistest.NewTestRedis(t)
	redis2, done2 := redistest.NewTestRedis(t)
	redis3, done3 := redistest.NewTestRedis(t)
	defer done1()
	defer done2()
	defer done3()
	host1, port1, err := net.SplitHostPort(redis1)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}
	_, port2, err := net.SplitHostPort(redis2)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}
	_, port3, err := net.SplitHostPort(redis3)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}

	// apiserver1
	specFmt := redisEpSpecFmt + redisPortFmt
	redisSpec1 := fmt.Sprintf(specFmt, 0, host1, port1)
	apiServer1, u1, err := createApiserver(kubeSpec + redisSpec1)
	if err != nil {
		t.Fatalf("Failed to start apiserver1: %v", err)
	}
	defer apiServer1.Close()

	// apiserver2
	specFmt += redisPortFmt
	redisSpec2 := fmt.Sprintf(specFmt, 0, host1, port1, port2)
	apiServer2, u2, err := createApiserver(kubeSpec + redisSpec2)
	if err != nil {
		t.Fatalf("Failed to start apiserver2: %v", err)
	}
	defer apiServer2.Close()

	// apiserver3
	specFmt += redisPortFmt
	redisSpec3 := fmt.Sprintf(specFmt, 0, host1, port1, port2, port3)
	apiServer3, u3, err := createApiserver(kubeSpec + redisSpec3)
	if err != nil {
		t.Fatalf("Failed to start apiserver3: %v", err)
	}
	defer apiServer3.Close()

	// create skipper as LB to kube-apiservers
	fr := createFilterRegistry(fscheduler.NewFifo(), flog.NewEnableAccessLog())
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	docFmt := `
r1: * -> enableAccessLog(4,5) -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;
r2: PathRegexp("/endpoints") -> enableAccessLog(2,4,5) -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;
`
	docApiserver := fmt.Sprintf(docFmt, u1.String(), u2.String(), u3.String(), u1.String(), u2.String(), u3.String())
	dc, err := testdataclient.NewDoc(docApiserver)
	if err != nil {
		t.Fatalf("Failed to create testdataclient: %v", err)
	}

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PostProcessors: []routing.PostProcessor{
			loadbalancer.NewAlgorithmProvider(),
			reg,
		},
		SuppressLogs: true,
	}
	rt := routing.New(ro)
	defer rt.Close()
	<-rt.FirstLoad()
	tracer := &tracingtest.Tracer{}
	pr := proxy.WithParams(proxy.Params{
		Routing:     rt,
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
	})
	defer pr.Close()
	lb := stdlibhttptest.NewServer(pr)
	defer lb.Close()

	rsvo := routesrv.Options{
		Address:                         ":8082",
		KubernetesURL:                   lb.URL,
		KubernetesRedisServiceNamespace: "skipper",
		KubernetesRedisServiceName:      "redis",
		KubernetesHealthcheck:           true,
		SourcePollTimeout:               1500 * time.Millisecond,
	}

	go routesrv.Run(rsvo)

	for {
		rsp, _ := http.DefaultClient.Head("http://localhost:8082/routes")
		if rsp != nil && rsp.StatusCode == 200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// run skipper proxy that we want to test
	o := Options{
		Address:                        ":9090",
		EnableRatelimiters:             true,
		EnableSwarm:                    true,
		Kubernetes:                     true,
		SwarmRedisEndpointsRemoteURL:   "http://localhost:8082/swarm/redis/shards",
		KubernetesURL:                  lb.URL,
		KubernetesHealthcheck:          true,
		SourcePollTimeout:              1500 * time.Millisecond,
		WaitFirstRouteLoad:             true,
		ClusterRatelimitMaxGroupShards: 2,
		SwarmRedisDialTimeout:          100 * time.Millisecond,
		SuppressRouteUpdateLogs:        false,
		SupportListener:                ":9091",
		testOptions: testOptions{
			redisUpdateInterval: time.Second,
		},
	}

	sigs := make(chan os.Signal, 1)
	go run(o, sigs, nil)

	for i := 0; i < 10; i++ {
		t.Logf("Waiting for proxy being ready")

		rsp, _ := http.DefaultClient.Get("http://localhost:9090/kube-system/healthz")
		if rsp != nil && rsp.StatusCode == 200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	rate := 10
	sec := 5
	va := httptest.NewVegetaAttacker("http://localhost:9090/test", rate, time.Second, time.Second)
	va.Attack(io.Discard, time.Duration(sec)*time.Second, "mytest")

	successRate := va.Success()
	t.Logf("Success [0..1]: %0.2f", successRate)

	epsilon := 0.2
	// sec * 1 because 1 is the number of requests allowed per second via clusterRatelimit("foo", 1, "1s")
	assert.InEpsilon(t, float64(sec*1)/float64(sec*rate), successRate, epsilon, fmt.Sprintf("Test should have a success rate between %0.2f < %0.2f < %0.2f", successRate-epsilon, successRate, successRate+epsilon))

	// reqCount should be between 49 & 51 since we run 10 per second for 5 seconds
	epsilon = 1
	reqCount := va.TotalRequests()
	t.Logf("Total requests: %d", reqCount)
	assert.InEpsilon(t, uint64(rate*sec), va.TotalRequests(), epsilon, fmt.Sprintf("Test should run %d requests between: %d and %d", uint64(rate*sec), reqCount-uint64(epsilon), reqCount+uint64(epsilon)))

	epsilon = 1
	countOK, _ := va.CountStatus(http.StatusOK)
	t.Logf("Number of succeeded requests: %d", countOK)
	assert.InEpsilon(t, 1*sec, countOK, epsilon, fmt.Sprintf("Test should have accepted requests between %d and %d", countOK-int(epsilon), countOK+int(epsilon)))

	countLimited, _ := va.CountStatus(http.StatusTooManyRequests)
	t.Logf("Number of limited requests: %d", countLimited)
	assert.InEpsilon(t, sec*rate-(1*sec), countLimited, epsilon, fmt.Sprintf("Test should have limited requests between %d and %d", countLimited-int(epsilon), countLimited+int(epsilon)))

	sigs <- syscall.SIGTERM
}

func TestConcurrentKubernetesClusterStateAccess(t *testing.T) {
	redisPortFmt := `  - port: %s
    protocol: TCP
`
	redisEpSpecFmt := `
---
apiVersion: v1
kind: Endpoints
metadata:
  labels:
    application: skipper-ingress-redis
  name: redis
  namespace: skipper
subsets:
- addresses:
  - hostname: redis-%d.skipper.cluster.local
    ip: %s
  ports:
`

	kubeSpec := `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: target
spec:
  backends:
  - name: shunt
    type: shunt
  defaultBackends:
  - backendName: shunt
  routes:
  - pathSubtree: /test
    filters:
    - disableAccessLog()
    - clusterRatelimit("foo", 1, "1s")
    - status(200)
    - inlineContent("OK")
---
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: myapp
spec:
  hosts:
  - example.org
  backends:
  - name: myapp
    type: service
    serviceName: myapp
    servicePort: 80
  defaultBackends:
  - backendName: myapp
---
apiVersion: v1
kind: Service
metadata:
  name: myapp
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    application: myapp
  type: ClusterIP
---
apiVersion: v1
kind: Endpoints
metadata:
  name: myapp
subsets:
- addresses:
  - ip: 10.2.4.8
  - ip: 10.2.4.16
  ports:
  - port: 80
---
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-ingress-redis
  name: redis
  namespace: skipper
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-ingress-redis
  type: ClusterIP
`

	redis1, done1 := redistest.NewTestRedis(t)
	redis2, done2 := redistest.NewTestRedis(t)
	redis3, done3 := redistest.NewTestRedis(t)
	defer done1()
	defer done2()
	defer done3()
	host1, port1, err := net.SplitHostPort(redis1)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}
	_, port2, err := net.SplitHostPort(redis2)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}
	_, port3, err := net.SplitHostPort(redis3)
	if err != nil {
		t.Fatalf("Failed to SplitHostPort: %v", err)
	}

	// apiserver1
	specFmt := redisEpSpecFmt + redisPortFmt
	redisSpec1 := fmt.Sprintf(specFmt, 0, host1, port1)
	apiServer1, u1, err := createApiserver(kubeSpec + redisSpec1)
	if err != nil {
		t.Fatalf("Failed to start apiserver1: %v", err)
	}
	defer apiServer1.Close()

	// apiserver2
	specFmt += redisPortFmt
	redisSpec2 := fmt.Sprintf(specFmt, 0, host1, port1, port2)
	apiServer2, u2, err := createApiserver(kubeSpec + redisSpec2)
	if err != nil {
		t.Fatalf("Failed to start apiserver2: %v", err)
	}
	defer apiServer2.Close()

	// apiserver3
	specFmt += redisPortFmt
	redisSpec3 := fmt.Sprintf(specFmt, 0, host1, port1, port2, port3)
	apiServer3, u3, err := createApiserver(kubeSpec + redisSpec3)
	if err != nil {
		t.Fatalf("Failed to start apiserver3: %v", err)
	}
	defer apiServer3.Close()

	// create skipper as LB to kube-apiservers
	fr := createFilterRegistry(fscheduler.NewFifo(), flog.NewEnableAccessLog())
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	docFmt := `
r1: * -> enableAccessLog(4,5) -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;
r2: PathRegexp("/endpoints") -> enableAccessLog(2,4,5) -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;
`
	docApiserver := fmt.Sprintf(docFmt, u1.String(), u2.String(), u3.String(), u1.String(), u2.String(), u3.String())
	dc, err := testdataclient.NewDoc(docApiserver)
	if err != nil {
		t.Fatalf("Failed to create testdataclient: %v", err)
	}

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PostProcessors: []routing.PostProcessor{
			loadbalancer.NewAlgorithmProvider(),
			reg,
		},
		SuppressLogs: true,
	}
	rt := routing.New(ro)
	defer rt.Close()
	<-rt.FirstLoad()
	tracer := &tracingtest.Tracer{}
	pr := proxy.WithParams(proxy.Params{
		Routing:     rt,
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
	})
	defer pr.Close()
	lb := stdlibhttptest.NewServer(pr)
	defer lb.Close()

	// run skipper proxy that we want to test
	o := Options{
		Address:                         ":9090",
		EnableRatelimiters:              true,
		EnableSwarm:                     true,
		Kubernetes:                      true,
		KubernetesURL:                   lb.URL,
		KubernetesRedisServiceNamespace: "skipper",
		KubernetesRedisServiceName:      "redis",
		KubernetesHealthcheck:           true,
		SourcePollTimeout:               1500 * time.Millisecond,
		WaitFirstRouteLoad:              true,
		ClusterRatelimitMaxGroupShards:  2,
		SwarmRedisDialTimeout:           100 * time.Millisecond,
		SuppressRouteUpdateLogs:         false,
		SupportListener:                 ":9091",
		testOptions: testOptions{
			redisUpdateInterval: time.Second,
		},
	}

	sigs := make(chan os.Signal, 1)
	go run(o, sigs, nil)

	for i := 0; i < 10; i++ {
		t.Logf("Waiting for proxy being ready")

		rsp, _ := http.DefaultClient.Get("http://localhost:9090/kube-system/healthz")
		if rsp != nil && rsp.StatusCode == 200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	rate := 10
	sec := 5
	va := httptest.NewVegetaAttacker("http://localhost:9090/test", rate, time.Second, time.Second)
	va.Attack(io.Discard, time.Duration(sec)*time.Second, "mytest")
	t.Logf("Success [0..1]: %0.2f", va.Success())

	if successRate := va.Success(); successRate < 0.1 || successRate > 0.5 {
		t.Fatalf("Test should have a success rate between %0.2f < %0.2f < %0.2f", 0.1, successRate, 0.5)
	}
	if reqCount := va.TotalRequests(); reqCount < uint64(rate*sec) {
		t.Fatalf("Test should run %d requests got: %d", uint64(rate*sec), reqCount)
	}
	countOK, ok := va.CountStatus(http.StatusOK)
	if countOK == 0 {
		t.Fatalf("Some requests should have passed: %d %v", countOK, ok)
	}

	countLimited, ok := va.CountStatus(http.StatusTooManyRequests)
	if !ok || countLimited < countOK {
		t.Fatalf("count TooMany should be higher than OKs: %d < %d: %v", countLimited, countOK, ok)
	}

	sigs <- syscall.SIGTERM
}

func createApiserver(spec string) (*stdlibhttptest.Server, *url.URL, error) {
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, bytes.NewBufferString(spec))
	if err != nil {
		return nil, nil, err
	}
	apiServer := stdlibhttptest.NewServer(api)
	u, err := url.Parse(apiServer.URL)
	return apiServer, u, err
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
		testOptions:                     testOptions{},
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

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     dcs, //[]routing.DataClient{dc},
		PostProcessors: []routing.PostProcessor{
			loadbalancer.NewAlgorithmProvider(),
			reg,
		},
		SuppressLogs: true,
	}
	rt := routing.New(ro)
	defer rt.Close()
	<-rt.FirstLoad()
	tracer := &tracingtest.Tracer{}
	pr := proxy.WithParams(proxy.Params{
		Routing:     rt,
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
	})
	defer pr.Close()
	lb := stdlibhttptest.NewServer(pr)
	defer lb.Close()

	sigs := make(chan os.Signal, 1)
	go run(o, sigs, nil)

	for i := 0; i < 10; i++ {
		t.Logf("Waiting for proxy being ready")

		rsp, _ := http.DefaultClient.Get("http://localhost:8090/healthz")
		if rsp != nil && rsp.StatusCode == 200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
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
