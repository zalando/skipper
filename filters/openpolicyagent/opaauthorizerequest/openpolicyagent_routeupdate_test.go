package opaauthorizerequest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/openpolicyagent/internal/opatestutils"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/tracing/tracingtest"
)

const (
	routeUpdatePollingTimeout        = 10 * time.Millisecond
	startUpTimeOut                   = 1 * time.Second
	startupTimeoutWithoutControlLoop = 3 * time.Second
	cleanInterval                    = 5 * time.Minute
	reuseDuration                    = 5 * time.Minute
	controlLoopInterval              = 30 * time.Millisecond
	controlLoopMaxJitter             = 3 * time.Millisecond
	proxyWaitTime                    = 5 * time.Second
)

type testPhase struct {
	routes          string
	testPath        string
	expectedStatus  int
	expectedHealthy bool
}

func TestOpaRoutesAtRouteUpdate(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		bundleName     string
		bootstrapPhase testPhase
		targetPhase    *testPhase
	}{
		{
			msg:        "Bootstrap with OPA route",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes:          `r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				testPath:        "/secure",
				expectedStatus:  http.StatusNoContent,
				expectedHealthy: true,
			},
		},
		{
			msg:        "Bootstrap empty then update with OPA route",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes:         "",
				testPath:       "",
				expectedStatus: http.StatusNotFound,
			},
			targetPhase: &testPhase{
				routes:          `r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				testPath:        "/secure",
				expectedStatus:  http.StatusNoContent,
				expectedHealthy: true,
			},
		},
		{
			msg:        "Mixed OPA and non-OPA routes",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes: `	r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>;
							r2: Path("/public") -> status(200) -> <shunt>
						`,
				testPath:        "/public",
				expectedStatus:  http.StatusOK,
				expectedHealthy: true,
			},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+ti.bundleName, map[string]string{
					"main.rego": `
							package test1
							import rego.v1
							
							default allow := false
							
							allow if {
								input.parsed_path == ["secure"]
							}
					`,
				}),
			)
			defer opaControlPlane.Stop()

			opaRegistry, fr := setupOpaTestEnvironment(t, opaControlPlane.URL(), false)

			var initialRoutes []*eskip.Route
			if ti.bootstrapPhase.routes != "" {
				initialRoutes = eskip.MustParse(ti.bootstrapPhase.routes)
			}

			proxy, dc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout)
			defer cleanupProxy(proxy, dc)

			// Bootstrap phase: Verify the initial state
			if ti.bootstrapPhase.routes != "" {
				assertOpaInstanceHealth(t, opaRegistry, ti.bundleName, ti.bootstrapPhase.expectedHealthy)
				rsp, err := makeHTTPRequest(proxy, ti.bootstrapPhase.testPath)
				require.NoError(t, err)
				defer rsp.Body.Close()
				assert.Equal(t, ti.bootstrapPhase.expectedStatus, rsp.StatusCode)
			}

			// Target phase: Update routes and verify the target state
			if ti.targetPhase != nil && ti.targetPhase.routes != "" {
				updateRoutes := eskip.MustParse(ti.targetPhase.routes)
				dc.Update(updateRoutes, nil)

				require.Eventually(t, func() bool {
					inst, err := opaRegistry.GetOrStartInstance(ti.bundleName)
					return err == nil && inst.Healthy() && inst.Started()
				}, startUpTimeOut, routeUpdatePollingTimeout)

				require.Eventually(t, func() bool {
					rsp, err := makeHTTPRequest(proxy, ti.targetPhase.testPath)
					if err != nil {
						return false
					}
					defer rsp.Body.Close()
					return rsp.StatusCode == ti.targetPhase.expectedStatus
				}, startUpTimeOut, routeUpdatePollingTimeout)
			}
		})
	}
}

func makeHTTPRequest(proxy *proxytest.TestProxy, path string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", proxy.URL+path, nil)
	return proxy.Client().Do(req)
}

func assertOpaInstanceHealth(t *testing.T, registry *openpolicyagent.OpenPolicyAgentRegistry, bundleName string, expectedHealth bool) {
	inst, err := registry.GetOrStartInstance(bundleName)
	require.NoError(t, err)
	assert.NotNil(t, inst)
	assert.Equal(t, expectedHealth, inst.Healthy())
}

// TestOpaRoutesWithSlowBundleServer simulates a slow OPA bundle server responding with a timeout failing the OPA filter
// and returning 503. Non-OPA routes should be successfully added to the route table and work fine.
func TestOpaRoutesWithSlowBundleServerNotBlockingOtherRouteUpdates(t *testing.T) {
	bundleName := "slowbundle"

	bundleServer := opatestutils.StartControllableBundleServer(opatestutils.BundleServerConfig{BundleName: bundleName, RespCode: http.StatusGatewayTimeout})
	bundleServer.SetDelay(5 * time.Second)
	defer bundleServer.Stop()

	// Bootstrap phase: start with a simple route
	initialRoutes := eskip.MustParse(`r1: Path("/initial") -> status(200) -> <shunt>`)
	dc := testdataclient.New(initialRoutes)
	defer dc.Close()

	opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), true)
	proxy, dc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout)
	defer cleanupProxy(proxy, dc)

	// Update phase: add an OPA route with a slow bundle server and a non-OPA route
	routesDef := `
			r2: Path("/slowbundle") -> opaAuthorizeRequest("slowbundle", "") -> status(204) -> <shunt>;
			r3: Path("/simple") -> status(200) -> <shunt>
			`
	updatedRoutes := eskip.MustParse(routesDef)
	dc.Update(updatedRoutes, nil)

	require.Eventually(t, func() bool {
		rsp, err := makeHTTPRequest(proxy, "/simple")
		if err != nil {
			return false
		}
		defer rsp.Body.Close()
		return http.StatusOK == rsp.StatusCode
	}, 2*routeUpdatePollingTimeout, routeUpdatePollingTimeout) // Give twice the polling time for the change to be applied

	assertOpaInstanceHealth(t, opaRegistry, bundleName, false)

	rsp, err := makeHTTPRequest(proxy, "/slowbundle")
	require.NoError(t, err)
	defer rsp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)
}

// TestOpaRoutesWithBundleServerRecoveryBootstrap simulates a scenario where the OPA bundle server is unavailable at
// Skipper bootstrap and recovers later. The OPA instance should become healthy and the OPA route should start working.
func TestOpaRoutesWithBundleServerRecoveryBootstrap(t *testing.T) {
	testCases := []struct {
		msg               string
		enableControlLoop bool
	}{
		{
			msg:               "Bootstrap recovery - control loop disabled",
			enableControlLoop: false,
		},
		{
			msg:               "Bootstrap recovery - control loop enabled",
			enableControlLoop: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			bundleName := "recoverybundle"

			bundleServer := opatestutils.StartControllableBundleServer(opatestutils.BundleServerConfig{BundleName: bundleName, RespCode: http.StatusTooManyRequests})
			defer bundleServer.Stop()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			initialRoutes := eskip.MustParse(`r1: Path("/recoverybundle") -> opaAuthorizeRequest("recoverybundle", "") -> status(204) -> <shunt>`)
			proxy, tdc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout, proxyWaitTime)
			defer cleanupProxy(proxy, tdc)

			assertOpaInstanceHealth(t, opaRegistry, bundleName, false)

			rsp, err := makeHTTPRequest(proxy, "/recoverybundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)

			maxOpaPollingInterval := 2 * time.Second
			if !tc.enableControlLoop {
				time.Sleep(startupTimeoutWithoutControlLoop) //Let the startup timeout pass as the plugin picks up the recovered bundle within this time
				bundleServer.SetRespCode(http.StatusOK)

				require.Eventually(t, func() bool {
					inst, err := opaRegistry.GetOrStartInstance(bundleName)
					return inst != nil && err == nil && inst.Healthy()
				}, 2*maxOpaPollingInterval, routeUpdatePollingTimeout) // When control loop is disabled, OPA inbuilt polling is used to download the bundle
			} else {
				bundleServer.SetRespCode(http.StatusOK)

				require.Eventually(t, func() bool {
					inst, err := opaRegistry.GetOrStartInstance(bundleName)
					return inst != nil && err == nil && inst.Healthy()
				}, startUpTimeOut, routeUpdatePollingTimeout)
			}

			rsp, err = makeHTTPRequest(proxy, "/recoverybundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode)
		})
	}
}

// TestOpaRoutesWithBundleServerRecoveryRouteUpdates simulates a route update adding an OPA route with an unavailable
// bundle server, which recovers later.
func TestOpaRoutesWithBundleServerRecoveryRouteUpdates(t *testing.T) {
	testCases := []struct {
		msg               string
		enableControlLoop bool
	}{
		{
			msg:               "Route update recovery - control loop disabled",
			enableControlLoop: false,
		},
		{
			msg:               "Route update recovery - control loop enabled",
			enableControlLoop: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			bundleName := "recoverybundle"

			bundleServer := opatestutils.StartControllableBundleServer(opatestutils.BundleServerConfig{BundleName: bundleName, RespCode: http.StatusGatewayTimeout})
			defer bundleServer.Stop()

			// Bootstrap phase: start with a simple route
			initialRoutes := eskip.MustParse(`r1: Path("/initial") -> status(200) -> <shunt>`)
			dc := testdataclient.New(initialRoutes)
			defer dc.Close()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			proxy, dc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout)
			defer cleanupProxy(proxy, dc)

			// Update phase: add an OPA route with an unavailable bundle server that later recovers
			routesDef := `
					r2: Path("/recoverybundle") -> opaAuthorizeRequest("recoverybundle", "") -> status(204) -> <shunt>;
				`
			updatedRoutes := eskip.MustParse(routesDef)
			dc.Update(updatedRoutes, nil)

			_, err := opaRegistry.GetOrStartInstance(bundleName)
			require.ErrorContains(t, err, "open policy agent instance for bundle 'recoverybundle' is not ready yet")

			bundleServer.SetRespCode(http.StatusOK)

			require.Eventually(t, func() bool {
				inst, err := opaRegistry.GetOrStartInstance(bundleName)
				return err == nil && inst.Healthy() && inst.Started()
			}, startUpTimeOut, routeUpdatePollingTimeout)

			rsp, err := makeHTTPRequest(proxy, "/recoverybundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode)
		})
	}
}

// TestOpaRoutesWithBundleServerFailover simulates a scenario where the OPA bundle server becomes unavailable after
// successfully serving bundles. The OPA instance should remain healthy and continue serving requests using the cached bundle.
func TestOpaRoutesWithBundleServerFailover(t *testing.T) {
	testCases := []struct {
		msg               string
		enableControlLoop bool
	}{
		{
			msg:               "Bundle server failover - control loop disabled",
			enableControlLoop: false,
		},
		{
			msg:               "Bundle server failover - control loop enabled",
			enableControlLoop: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.msg, func(t *testing.T) {
			bundleName := "failoverbundle"

			bundleServer := opatestutils.StartControllableBundleServer(opatestutils.BundleServerConfig{BundleName: bundleName, RespCode: http.StatusOK})
			defer bundleServer.Stop()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			initialRoutes := eskip.MustParse(`
				r1: Path("/failoverbundle") -> opaAuthorizeRequest("failoverbundle", "") -> status(204) -> <shunt>
			`)
			proxy, tdc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout, proxyWaitTime)
			defer cleanupProxy(proxy, tdc)

			// Bootstrap phase: Establish healthy state
			require.Eventually(t, func() bool {
				inst, err := opaRegistry.GetOrStartInstance(bundleName)
				return inst != nil && err == nil && inst.Healthy()
			}, startUpTimeOut, routeUpdatePollingTimeout)

			rsp, err := makeHTTPRequest(proxy, "/failoverbundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode)

			// Failover phase: Simulate bundle server failure
			bundleServer.SetRespCode(http.StatusServiceUnavailable)

			time.Sleep(2 * startupTimeoutWithoutControlLoop)

			inst, err := opaRegistry.GetOrStartInstance(bundleName)
			assert.NoError(t, err)
			assert.NotNil(t, inst)
			assert.True(t, inst.Healthy(), "Healthy instance should remain healthy despite bundle server failure")

			rsp, err = makeHTTPRequest(proxy, "/failoverbundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode,
				"Should continue serving requests with cached bundle after server failure")
		})
	}
}

// TestOpaRouteRemovalAndCleanup simulates a scenario where an OPA route is removed from the route table. OPA instance
// should be cleaned up after the reuse duration if no other route requires it.
func TestOpaRouteRemovalAndCleanup(t *testing.T) {
	bundleName := "bundle"

	bundleServer := opatestutils.StartControllableBundleServer(opatestutils.BundleServerConfig{BundleName: bundleName, RespCode: http.StatusOK})
	defer bundleServer.Stop()

	config := OpaRegistryConfig{
		BundleServerURL:   bundleServer.URL(),
		EnableControlLoop: false,
		StartupTimeout:    startUpTimeOut,
		CleanInterval:     2 * time.Second,
		ReuseDuration:     2 * time.Second,
	}

	opaRegistry := createOpaRegistry(t, config)
	fr := make(filters.Registry)
	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
	fr.Register(ftSpec)
	fr.Register(builtin.NewStatus())

	routesDef := `
			r1: Path("/bundle") -> opaAuthorizeRequest("bundle", "") -> status(204) -> <shunt>;
			r2: Path("/simple") -> status(200) -> <shunt>
		`
	initialRoutes := eskip.MustParse(routesDef)
	dc := testdataclient.New(initialRoutes)
	defer dc.Close()

	preprocessor := opaRegistry.NewPreProcessor()

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		FilterRegistry: fr,
		DataClients:    []routing.DataClient{dc},
		PreProcessors:  []routing.PreProcessor{preprocessor},
		PostProcessors: []routing.PostProcessor{opaRegistry},
		PollTimeout:    routeUpdatePollingTimeout,
	})

	defer proxy.Close()

	assertOpaInstanceHealth(t, opaRegistry, bundleName, true)

	require.Eventually(t, func() bool {
		inst, err := opaRegistry.GetOrStartInstance(bundleName)
		return inst != nil && err == nil && inst.Healthy()
	}, startUpTimeOut, routeUpdatePollingTimeout)

	rsp, err := makeHTTPRequest(proxy, "/bundle")
	require.NoError(t, err)
	defer rsp.Body.Close()
	assert.Equal(t, http.StatusNoContent, rsp.StatusCode)

	updatedRoutes := eskip.MustParse(`r2: Path("/simple") -> status(200) -> <shunt>`)
	dc.Update(updatedRoutes, []string{"r1"})

	require.Eventually(t, func() bool {
		rsp, err = makeHTTPRequest(proxy, "/bundle")
		require.NoError(t, err)
		defer rsp.Body.Close()
		return http.StatusNotFound == rsp.StatusCode
	}, 2*routeUpdatePollingTimeout, routeUpdatePollingTimeout)

	time.Sleep(4 * time.Second) // Wait for two cleanup cycles time
	inst, err := opaRegistry.GetOrStartInstance(bundleName)
	assert.Nil(t, inst, "OPA instance should be cleaned up after route removal and reuse duration")
}

// TestExceedingBackgroundTaskBuffer tests multiple bundles being handled in the background task exceeding the buffer size.
func TestExceedingBackgroundTaskBuffer(t *testing.T) {
	// Use multiple bundles to trigger multiple background tasks
	bundleConfigs := []opatestutils.BundleServerConfig{{
		BundleName: "bundle1", RespCode: http.StatusOK,
	}, {
		BundleName: "bundle2", RespCode: http.StatusOK,
	}, {
		BundleName: "bundle3", RespCode: http.StatusOK,
	}, {
		BundleName: "bundle4", RespCode: http.StatusOK,
	}}

	bundleProxy, bundleServers := opatestutils.StartMultiBundleProxyServer(bundleConfigs)
	defer bundleProxy.Close()
	for _, srv := range bundleServers {
		defer srv.Stop()
	}

	// Bootstrap phase
	initialRoutes := []*eskip.Route{}
	dc := testdataclient.New(initialRoutes)
	defer dc.Close()

	config := OpaRegistryConfig{
		BundleServerURL:          bundleProxy.URL,
		EnableControlLoop:        false,
		StartupTimeout:           startUpTimeOut,
		CleanInterval:            2 * time.Second,
		ReuseDuration:            2 * time.Second,
		BackgroundTaskBufferSize: 1, // Small buffer to easily exhaust the size
	}

	opaRegistry := createOpaRegistry(t, config)

	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
	fr := make(filters.Registry)
	fr.Register(ftSpec)
	fr.Register(builtin.NewStatus())
	proxy, dc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout)
	defer cleanupProxy(proxy, dc)

	//Update phase to exceed the buffer size
	routesDef := `
			r1: Path("/bundle1") -> opaAuthorizeRequest("bundle1", "") -> status(204) -> <shunt>;
			r2: Path("/bundle2") -> opaAuthorizeRequest("bundle2", "") -> status(204) -> <shunt>;
			r3: Path("/bundle3") -> opaAuthorizeRequest("bundle3", "") -> status(204) -> <shunt>;
			r4: Path("/bundle4") -> opaAuthorizeRequest("bundle4", "") -> status(204) -> <shunt>
		`
	updatedRoutes := eskip.MustParse(routesDef)
	dc.Update(updatedRoutes, nil)
	time.Sleep(routeUpdatePollingTimeout) // Tasks spill over from the buffer need another route update to retry it

	require.Eventually(t, func() bool {
		dc.Update(updatedRoutes, nil) // if the queue has not been freed, it need multiple route updates to get processed

		inst1, _ := opaRegistry.GetOrStartInstance("bundle1")
		inst2, _ := opaRegistry.GetOrStartInstance("bundle2")
		inst3, _ := opaRegistry.GetOrStartInstance("bundle3")
		inst4, _ := opaRegistry.GetOrStartInstance("bundle4")
		return inst1.Started() && inst1.Healthy() && inst2.Started() && inst2.Healthy() && inst3.Started() && inst3.Healthy() && inst4.Started() && inst4.Healthy()
	}, 3*startUpTimeOut, routeUpdatePollingTimeout)
}

// TestMalformedBundleResponse simulates a scenario where the OPA bundle server returns a malformed bundle response.
// The non-OPA routes should still be added to the route table and work fine, while the OPA route should continue to return 503
func TestMalformedBundleResponse(t *testing.T) {
	bundleName := "somebundle"
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/"+bundleName, map[string]string{
			"main.rego": `
					package envoy.authz
					import rego.v1
					
					default allow := false
					
					allow if {
						input.parsed_path == ["secure"]
					//syntax error 
				`,
		}),
	)
	defer opaControlPlane.Stop()

	opaRegistry, fr := setupOpaTestEnvironment(t, opaControlPlane.URL(), false)

	initialRoutes := eskip.MustParse(`
			r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>;
			r2: Path("/public") -> status(200) -> <shunt>
			`)

	proxy, dc := startSkipper(fr, opaRegistry, initialRoutes, routeUpdatePollingTimeout)
	defer cleanupProxy(proxy, dc)

	assertOpaInstanceHealth(t, opaRegistry, bundleName, false)
	rsp, err := makeHTTPRequest(proxy, "/secure")
	require.NoError(t, err)
	defer rsp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)

	rspSimple, err := makeHTTPRequest(proxy, "/public")
	require.NoError(t, err)
	defer rspSimple.Body.Close()
	assert.Equal(t, http.StatusOK, rspSimple.StatusCode)

}

func startSkipper(fr filters.Registry, opaRegistry *openpolicyagent.OpenPolicyAgentRegistry, initialRoutes []*eskip.Route, pollTimeout time.Duration, waitTime ...time.Duration) (*proxytest.TestProxy, *testdataclient.Client) {
	dc := testdataclient.New(initialRoutes)
	preprocessor := opaRegistry.NewPreProcessor()
	actualWaitTime := proxyWaitTime
	if len(waitTime) > 0 {
		actualWaitTime = waitTime[0]
	}
	proxy := proxytest.WithRoutingOptionsWithWait(fr, routing.Options{
		FilterRegistry: fr,
		DataClients:    []routing.DataClient{dc},
		PreProcessors:  []routing.PreProcessor{preprocessor},
		PostProcessors: []routing.PostProcessor{opaRegistry},
		PollTimeout:    pollTimeout,
	}, actualWaitTime)
	return proxy, dc
}

func cleanupProxy(proxy *proxytest.TestProxy, dc *testdataclient.Client) {
	dc.Close()
	proxy.Close()
}

// OpaRegistryConfig holds configuration for creating OPA registry instances
type OpaRegistryConfig struct {
	BundleServerURL          string
	EnableControlLoop        bool
	StartupTimeout           time.Duration
	CleanInterval            time.Duration
	ReuseDuration            time.Duration
	ConfigPath               string // allows customizing the plugin path
	BackgroundTaskBufferSize int
}

// createOpaRegistry creates a new OpenPolicyAgentRegistry with the given configuration
func createOpaRegistry(t *testing.T, config OpaRegistryConfig) *openpolicyagent.OpenPolicyAgentRegistry {
	// Default config path if not specified
	configPath := config.ConfigPath
	if configPath == "" {
		configPath = "test1/allow"
	}

	configTemplate := []byte(fmt.Sprintf(`{
		"services": {"test": {"url": %q}},
		"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}", "polling": {"min_delay_seconds": 1, "max_delay_seconds": 2}}},
		"plugins": {
			"envoy_ext_authz_grpc": {
				"path": "%s",
				"dry-run": false
			}
		}
	}`, config.BundleServerURL, configPath))

	opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(configTemplate),
	}

	registryOpts := []func(*openpolicyagent.OpenPolicyAgentRegistry) error{
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
		openpolicyagent.WithInstanceStartupTimeout(config.StartupTimeout),
		openpolicyagent.WithCleanInterval(config.CleanInterval),
		openpolicyagent.WithReuseDuration(config.ReuseDuration),
	}

	if config.EnableControlLoop {
		registryOpts = append(registryOpts,
			openpolicyagent.WithEnableCustomControlLoop(true),
			openpolicyagent.WithControlLoopInterval(controlLoopInterval),
			openpolicyagent.WithControlLoopMaxJitter(controlLoopMaxJitter),
		)
	}

	if config.BackgroundTaskBufferSize != 0 {
		registryOpts = append(registryOpts, openpolicyagent.WithBackgroundTaskBufferSize(config.BackgroundTaskBufferSize))
	}

	registry, err := openpolicyagent.NewOpenPolicyAgentRegistry(registryOpts...)
	require.NoError(t, err)
	return registry
}

// setupOpaTestEnvironment sets up the OPA test environment with the given bundle server URL and control loop setting.
func setupOpaTestEnvironment(t *testing.T, bundleServerURL string, enableControlLoop bool) (*openpolicyagent.OpenPolicyAgentRegistry, filters.Registry) {
	fr := make(filters.Registry)

	startupTimeout := startUpTimeOut
	if !enableControlLoop {
		startupTimeout = startupTimeoutWithoutControlLoop
	}

	config := OpaRegistryConfig{
		BundleServerURL:   bundleServerURL,
		EnableControlLoop: enableControlLoop,
		StartupTimeout:    startupTimeout,
		CleanInterval:     cleanInterval,
		ReuseDuration:     reuseDuration,
	}

	opaRegistry := createOpaRegistry(t, config)

	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
	fr.Register(ftSpec)
	fr.Register(builtin.NewStatus())

	return opaRegistry, fr
}
