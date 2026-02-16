package skipper_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/zalando/skipper"
	flog "github.com/zalando/skipper/filters/accesslog"
	fscheduler "github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/net/valkeytest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routesrv"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentKubernetesClusterStateAccessWithRemoteValkey(t *testing.T) {
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
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-ingress-valkey
  name: valkey
  namespace: skipper
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-ingress-valkey
  type: ClusterIP
`

	valkey1, done1 := valkeytest.NewTestValkey(t)
	valkey2, done2 := valkeytest.NewTestValkey(t)
	valkey3, done3 := valkeytest.NewTestValkey(t)

	defer done1()
	defer done2()
	defer done3()

	// apiserver1
	valkeySpec1 := createValkeyEndpointsSpec(t, valkey1)
	apiServer1 := createApiserver(t, kubeSpec+valkeySpec1)

	// apiserver2
	valkeySpec2 := createValkeyEndpointsSpec(t, valkey1, valkey2)
	apiServer2 := createApiserver(t, kubeSpec+valkeySpec2)

	// apiserver3
	valkeySpec3 := createValkeyEndpointsSpec(t, valkey1, valkey2, valkey3)
	apiServer3 := createApiserver(t, kubeSpec+valkeySpec3)

	// create skipper as LB to kube-apiservers
	fr := createFilterRegistry(fscheduler.NewFifo(), flog.NewDisableAccessLog())
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	docApiserver := fmt.Sprintf(`r1: * -> disableAccessLog() -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;`,
		apiServer1.URL, apiServer2.URL, apiServer3.URL)

	dc, err := testdataclient.NewDoc(docApiserver)
	require.NoError(t, err)
	defer dc.Close()

	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	defer endpointRegistry.Close()

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
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

	skipper.MuFindAddress.Lock()
	rsvo := skipper.Options{
		Address:                          skipper.FindAddress(t),
		Kubernetes:                       true,
		KubernetesURL:                    lb.URL,
		KubernetesValkeyServiceNamespace: "skipper",
		KubernetesValkeyServiceName:      "valkey",
		KubernetesValkeyServicePort:      6379,
		KubernetesHealthcheck:            true,
		SourcePollTimeout:                1500 * time.Millisecond,
	}
	go routesrv.Run(rsvo)
	waitForOK(t, "http://"+rsvo.Address+"/routes", 1*time.Second)

	// run skipper proxy that we want to test
	o := skipper.Options{
		Address:                        skipper.FindAddress(t),
		EnableRatelimiters:             true,
		EnableSwarm:                    true,
		Kubernetes:                     true,
		SwarmValkeyEndpointsRemoteURL:  "http://" + rsvo.Address + "/swarm/valkey/shards",
		KubernetesURL:                  lb.URL,
		KubernetesHealthcheck:          true,
		SourcePollTimeout:              1500 * time.Millisecond,
		WaitFirstRouteLoad:             true,
		ClusterRatelimitMaxGroupShards: 2,
		SwarmValkeyConnWriteTimeout:    100 * time.Millisecond,
		SuppressRouteUpdateLogs:        false,
		SupportListener:                skipper.FindAddress(t),
		SwarmValkeyUpdateInterval:      time.Second,
		SwarmValkeyConnLifetime:        time.Second,
	}

	runResult := make(chan error)
	sigs := make(chan os.Signal, 1)
	go func() { runResult <- skipper.RunWithShutdown(o, sigs, nil) }()

	waitForOK(t, "http://"+o.Address+"/kube-system/healthz", 1*time.Second)
	skipper.MuFindAddress.Unlock()

	rate := 10
	sec := 5
	va := httptest.NewVegetaAttacker("http://"+o.Address+"/test", rate, time.Second, time.Second)
	va.Attack(io.Discard, time.Duration(sec)*time.Second, t.Name())

	successRate := va.Success()
	t.Logf("Success [0..1]: %0.2f", successRate)
	t.Logf("Want: %0.2f", float64(sec*1)/float64(sec*rate))

	epsilon := 0.2
	// sec * 1 because 1 is the number of requests allowed per second via clusterRatelimit("foo", 1, "1s")
	expected := float64(sec*1) / float64(sec*rate)
	// https://github.com/stretchr/testify/issues/1839
	// if !assert.InEpsilon(t, expected, successRate, epsilon, fmt.Sprintf("Test should have a success rate between %0.2f < %0.2f < %0.2f", expected-epsilon, successRate, expected+epsilon)) {
	if expected-epsilon >= successRate && successRate >= expected+epsilon {
		t.Fatalf("Test should have a success rate between %0.2f < %0.2f < %0.2f", expected-epsilon, successRate, expected+epsilon)
	}

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
	assert.NoError(t, <-runResult)
}

func TestConcurrentKubernetesClusterStateAccessValkey(t *testing.T) {
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
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-ingress-valkey
  name: valkey
  namespace: skipper
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-ingress-valkey
  type: ClusterIP
`

	valkey1, done1 := valkeytest.NewTestValkey(t)
	valkey2, done2 := valkeytest.NewTestValkey(t)
	valkey3, done3 := valkeytest.NewTestValkey(t)

	defer done1()
	defer done2()
	defer done3()

	// apiserver1
	valkeySpec1 := createValkeyEndpointsSpec(t, valkey1)
	apiServer1 := createApiserver(t, kubeSpec+valkeySpec1)

	// apiserver2
	valkeySpec2 := createValkeyEndpointsSpec(t, valkey1, valkey2)
	apiServer2 := createApiserver(t, kubeSpec+valkeySpec2)

	// apiserver3
	valkeySpec3 := createValkeyEndpointsSpec(t, valkey1, valkey2, valkey3)
	apiServer3 := createApiserver(t, kubeSpec+valkeySpec3)

	// create skipper as LB to kube-apiservers
	fr := createFilterRegistry(fscheduler.NewFifo(), flog.NewDisableAccessLog())
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	docApiserver := fmt.Sprintf(`r1: * -> disableAccessLog() -> fifo(100,100,"3s") -> <roundRobin, "%s", "%s", "%s">;`,
		apiServer1.URL, apiServer2.URL, apiServer3.URL)

	dc, err := testdataclient.NewDoc(docApiserver)
	require.NoError(t, err)
	defer dc.Close()

	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	defer endpointRegistry.Close()

	// create LB in front of apiservers to be able to switch the data served by apiserver
	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
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

	// run skipper proxy that we want to test
	skipper.MuFindAddress.Lock()
	o := skipper.Options{
		Address:                          skipper.FindAddress(t),
		EnableRatelimiters:               true,
		EnableSwarm:                      true,
		Kubernetes:                       true,
		KubernetesURL:                    lb.URL,
		KubernetesValkeyServiceNamespace: "skipper",
		KubernetesValkeyServiceName:      "valkey",
		KubernetesValkeyServicePort:      6379,
		KubernetesHealthcheck:            true,
		SourcePollTimeout:                1500 * time.Millisecond,
		WaitFirstRouteLoad:               true,
		ClusterRatelimitMaxGroupShards:   2,
		SwarmValkeyConnWriteTimeout:      100 * time.Millisecond,
		SuppressRouteUpdateLogs:          false,
		SupportListener:                  skipper.FindAddress(t),
		SwarmValkeyUpdateInterval:        time.Second,
		SwarmValkeyConnLifetime:          time.Second,
	}

	runResult := make(chan error)
	sigs := make(chan os.Signal, 1)
	go func() { runResult <- skipper.RunWithShutdown(o, sigs, nil) }()

	waitForOK(t, "http://"+o.Address+"/kube-system/healthz", 1*time.Second)
	skipper.MuFindAddress.Unlock()

	rate := 10
	sec := 5
	va := httptest.NewVegetaAttacker("http://"+o.Address+"/test", rate, time.Second, time.Second)
	va.Attack(io.Discard, time.Duration(sec)*time.Second, t.Name())
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
		t.Fatalf("count TooMany should be more than OKs: %d < %d: %v", countLimited, countOK, ok)
	}

	sigs <- syscall.SIGTERM
	assert.NoError(t, <-runResult)
}

func TestValkeyAddrUpdater(t *testing.T) {
	dm := metrics.Default
	t.Cleanup(func() { metrics.Default = dm })

	const valkeyUpdateInterval = 10 * time.Millisecond
	const kubeSpec = `
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
    - inlineContent("OK")
---
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-ingress-valkey
  name: valkey
  namespace: skipper
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-ingress-valkey
  type: ClusterIP
`

	t.Run("without kubernetes dataclient", func(t *testing.T) {
		valkey1, done1 := valkeytest.NewTestValkey(t)
		valkey2, done2 := valkeytest.NewTestValkey(t)
		valkey3, done3 := valkeytest.NewTestValkey(t)
		defer done1()
		defer done2()
		defer done3()

		spec := kubeSpec + createValkeyEndpointsSpec(t, valkey1, valkey2, valkey3)
		apiServer := createApiserver(t, spec)

		metrics := &metricstest.MockMetrics{}

		skipper.MuFindAddress.Lock()
		o := skipper.Options{
			Address:                          skipper.FindAddress(t),
			EnableRatelimiters:               true,
			EnableSwarm:                      true,
			Kubernetes:                       false, // do not enable kubernetes dataclient
			KubernetesURL:                    apiServer.URL,
			KubernetesValkeyServiceNamespace: "skipper",
			KubernetesValkeyServiceName:      "valkey",
			KubernetesValkeyServicePort:      6379,
			SwarmValkeyUpdateInterval:        valkeyUpdateInterval,
			InlineRoutes:                     `Path("/ready") -> inlineContent("OK") -> <shunt>`,
			MetricsBackend:                   metrics,
		}

		runResult := make(chan error)
		sigs := make(chan os.Signal, 1)
		go func() { runResult <- skipper.RunWithShutdown(o, sigs, nil) }()

		waitForOK(t, "http://"+o.Address+"/ready", 1*time.Second)
		skipper.MuFindAddress.Unlock()
		time.Sleep(2 * valkeyUpdateInterval)

		metrics.WithGauges(func(g map[string]float64) {
			t.Logf("gauges: %v", g)

			assert.Equal(t, 1.0, g["routes.total"], "expected only the /ready route")
			assert.Equal(t, 3.0, g["swarm.valkey.shards"])
		})

		sigs <- syscall.SIGTERM
		assert.NoError(t, <-runResult)
	})

	t.Run("kubernetes dataclient", func(t *testing.T) {
		valkey1, done1 := valkeytest.NewTestValkey(t)
		valkey2, done2 := valkeytest.NewTestValkey(t)
		valkey3, done3 := valkeytest.NewTestValkey(t)
		valkey4, done4 := valkeytest.NewTestValkey(t)
		defer done1()
		defer done2()
		defer done3()
		defer done4()

		spec := kubeSpec + createValkeyEndpointsSpec(t, valkey1, valkey2, valkey3, valkey4)
		apiServer := createApiserver(t, spec)

		metrics := &metricstest.MockMetrics{}

		skipper.MuFindAddress.Lock()
		o := skipper.Options{
			Address:                          skipper.FindAddress(t),
			EnableRatelimiters:               true,
			EnableSwarm:                      true,
			Kubernetes:                       true, // enable kubernetes dataclient
			KubernetesURL:                    apiServer.URL,
			KubernetesValkeyServiceNamespace: "skipper",
			KubernetesValkeyServiceName:      "valkey",
			KubernetesValkeyServicePort:      6379,
			SwarmValkeyUpdateInterval:        valkeyUpdateInterval,
			MetricsBackend:                   metrics,
		}

		runResult := make(chan error)
		sigs := make(chan os.Signal, 1)
		go func() { runResult <- skipper.RunWithShutdown(o, sigs, nil) }()

		waitForOK(t, "http://"+o.Address+"/test", 1*time.Second)
		skipper.MuFindAddress.Unlock()
		time.Sleep(2 * valkeyUpdateInterval)

		metrics.WithGauges(func(g map[string]float64) {
			t.Logf("gauges: %v", g)

			assert.Equal(t, 1.0, g["routes.total"], "expected only the /test route")
			assert.Equal(t, 4.0, g["swarm.valkey.shards"])
		})

		sigs <- syscall.SIGTERM
		assert.NoError(t, <-runResult)
	})

	t.Run("remote url", func(t *testing.T) {
		valkey1, done1 := valkeytest.NewTestValkey(t)
		valkey2, done2 := valkeytest.NewTestValkey(t)
		valkey3, done3 := valkeytest.NewTestValkey(t)
		defer done1()
		defer done2()
		defer done3()

		eps := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{
				"endpoints": [
					{"address": "%s"}, {"address": "%s"},
					{"address": "%s"}
				]
			}`, valkey1, valkey2, valkey3)
		}))
		defer eps.Close()

		metrics := &metricstest.MockMetrics{}

		skipper.MuFindAddress.Lock()
		o := skipper.Options{
			Address:                       skipper.FindAddress(t),
			EnableRatelimiters:            true,
			EnableSwarm:                   true,
			SwarmValkeyEndpointsRemoteURL: eps.URL,
			SwarmValkeyUpdateInterval:     valkeyUpdateInterval,
			InlineRoutes:                  `Path("/ready") -> inlineContent("OK") -> <shunt>`,
			MetricsBackend:                metrics,
		}

		runResult := make(chan error)
		sigs := make(chan os.Signal, 1)
		go func() { runResult <- skipper.RunWithShutdown(o, sigs, nil) }()

		waitForOK(t, "http://"+o.Address+"/ready", 10*time.Second)
		skipper.MuFindAddress.Unlock()
		time.Sleep(1 * valkeyUpdateInterval)

		metrics.WithGauges(func(g map[string]float64) {
			t.Logf("gauges: %v", g)

			assert.Equal(t, 1.0, g["routes.total"], "expected only the /ready route")
			assert.Equal(t, 3.0, g["swarm.valkey.shards"])
		})

		sigs <- syscall.SIGTERM
		assert.NoError(t, <-runResult)
	})
}

func createValkeyEndpointsSpec(t *testing.T, addrs ...string) string {
	t.Helper()
	var addresses []map[string]any
	for _, addr := range addrs {
		host, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		require.Equal(t, "6379", port)

		addresses = append(addresses, map[string]any{"ip": host})
	}

	ep := map[string]any{
		"apiVersion": "v1",
		"kind":       "Endpoints",
		"metadata": map[string]any{
			"name":      "valkey",
			"namespace": "skipper",
		},
		"subsets": []any{
			map[string]any{
				"addresses": addresses,
				"ports": []map[string]any{{
					"port":     6379,
					"protocol": "TCP",
				}},
			},
		},
	}

	// JSON is a valid YAML
	b, err := json.Marshal(ep)
	require.NoError(t, err)

	return fmt.Sprintf("---\n%s\n", b)
}
