package opaauthorizerequest

import (
	"fmt"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/opatestutils"
	"net/http"
	"testing"
	"time"

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
	routes         string
	testPath       string
	expectedStatus int
	healthy        bool
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
				routes:         `r1: Path("/initial") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				testPath:       "/initial",
				expectedStatus: http.StatusNoContent,
				healthy:        true,
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
				routes:         `r1: Path("/initial") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				testPath:       "/initial",
				expectedStatus: http.StatusNoContent,
				healthy:        true,
			},
		},
		{
			msg:        "Mixed OPA and non-OPA routes",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes: `r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>;
							r2: Path("/public") -> status(200) -> <shunt>
							`,
				testPath:       "/public",
				expectedStatus: http.StatusOK,
				healthy:        true,
			},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+ti.bundleName, map[string]string{
					"main.rego": `
							package envoy.authz
							import rego.v1

							default allow := false

							allow if {
								input.parsed_path == ["initial"]
							}

							allow if {
								input.parsed_path == ["updated"]
							}

							allow if {
								input.parsed_path == ["secure"]
							}
							`,
				}),
			)
			defer opaControlPlane.Stop()

			fr := make(filters.Registry)

			config := []byte(fmt.Sprintf(`{
				"services": {"test": {"url": %q}},
				"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
				"plugins": {
					"envoy_ext_authz_grpc": {
						"path": "envoy/authz/allow",
						"dry-run": false
					}
				}
			}`, opaControlPlane.URL()))

			opts := make([]func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error, 0)
			opts = append(opts, openpolicyagent.WithConfigTemplate(config))

			opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
				openpolicyagent.WithTracer(tracingtest.NewTracer()),
				openpolicyagent.WithPreloadingEnabled(true),
				openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
				openpolicyagent.WithInstanceStartupTimeout(startUpTimeOut),
				openpolicyagent.WithCleanInterval(5*time.Second),
				openpolicyagent.WithReuseDuration(5*time.Second),
			)
			assert.NoError(t, err)

			ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
			fr.Register(ftSpec)
			fr.Register(builtin.NewStatus())

			var initialRoutes []*eskip.Route
			if ti.bootstrapPhase.routes != "" {
				initialRoutes = eskip.MustParse(ti.bootstrapPhase.routes)
			}

			dc := testdataclient.New(initialRoutes)
			defer dc.Close()

			preprocessor := opaRegistry.NewPreProcessor()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				FilterRegistry: fr,
				DataClients:    []routing.DataClient{dc},
				PreProcessors:  []routing.PreProcessor{preprocessor},
				PollTimeout:    routeUpdatePollingTimeout,
			})
			defer proxy.Close()

			// Check the initial state - following bootstrap test pattern
			if ti.bootstrapPhase.routes != "" {
				assertOpaInstanceHealth(t, opaRegistry, ti.bundleName, ti.bootstrapPhase.healthy)
				rsp, err := makeHTTPRequest(proxy, ti.bootstrapPhase.testPath)
				require.NoError(t, err)
				defer rsp.Body.Close()
				assert.Equal(t, ti.bootstrapPhase.expectedStatus, rsp.StatusCode)
			}

			if ti.targetPhase != nil && ti.targetPhase.routes != "" {
				updateRoutes := eskip.MustParse(ti.targetPhase.routes)
				dc.Update(updateRoutes, nil)

				require.Eventually(t, func() bool {
					inst, err := opaRegistry.GetOrStartInstance(ti.bundleName)
					return inst != nil && err == nil && inst.Healthy()
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

	bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusGatewayTimeout)
	bundleServer.SetDelay(5 * time.Second)
	defer bundleServer.Stop()

	// Bootstrap phase: start with a simple route
	initialRoutes := eskip.MustParse(`r1: Path("/initial") -> status(200) -> <shunt>`)
	dc := testdataclient.New(initialRoutes)
	defer dc.Close()

	opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), true)

	preprocessor := opaRegistry.NewPreProcessor()
	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		FilterRegistry: fr,
		DataClients:    []routing.DataClient{dc},
		PreProcessors:  []routing.PreProcessor{preprocessor},
		PollTimeout:    routeUpdatePollingTimeout,
	})
	defer proxy.Close()

	routesDef := `
		r2: Path("/slowbundle") -> opaAuthorizeRequest("slowbundle", "") -> status(204) -> <shunt>;
		r3: Path("/simple") -> status(200) -> <shunt>
	`
	updatedRoutes := eskip.MustParse(routesDef)
	dc.Update(updatedRoutes, nil)

	// Non-OPA route should work fine
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

			bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusTooManyRequests)
			defer bundleServer.Stop()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			initialRoutes := eskip.MustParse(`r1: Path("/recoverybundle") -> opaAuthorizeRequest("recoverybundle", "") -> status(204) -> <shunt>`)
			dc := testdataclient.New(initialRoutes)
			defer dc.Close()

			preprocessor := opaRegistry.NewPreProcessor()

			proxy := proxytest.WithRoutingOptionsWithWait(fr, routing.Options{
				FilterRegistry: fr,
				DataClients:    []routing.DataClient{dc},
				PreProcessors:  []routing.PreProcessor{preprocessor},
				PollTimeout:    routeUpdatePollingTimeout,
			},
				proxyWaitTime)

			defer proxy.Close()

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
				}, maxOpaPollingInterval, routeUpdatePollingTimeout) // When control loop is disabled, OPA inbuilt polling is used to download the bundle
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

			bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusGatewayTimeout)
			defer bundleServer.Stop()

			// Bootstrap phase: start with a simple route
			initialRoutes := eskip.MustParse(`r1: Path("/initial") -> status(200) -> <shunt>`)
			dc := testdataclient.New(initialRoutes)
			defer dc.Close()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			preprocessor := opaRegistry.NewPreProcessor()
			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				FilterRegistry: fr,
				DataClients:    []routing.DataClient{dc},
				PreProcessors:  []routing.PreProcessor{preprocessor},
				PollTimeout:    routeUpdatePollingTimeout,
			})
			defer proxy.Close()

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
				return inst != nil && err == nil && inst.Healthy()
			}, startUpTimeOut, routeUpdatePollingTimeout)

			rsp, err := makeHTTPRequest(proxy, "/recoverybundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode)
		})
	}
}

func setupOpaTestEnvironment(t *testing.T, bundleServerURL string, enableControlLoop bool) (*openpolicyagent.OpenPolicyAgentRegistry, filters.Registry) {

	fr := make(filters.Registry)
	config := []byte(fmt.Sprintf(`{
		  "services": {"test": {"url": %q}},
          "bundles": {"test": {"resource": "/bundles/{{ .bundlename }}", "polling": {"min_delay_seconds": 1, "max_delay_seconds": 2}}},
		  "plugins": {
		   "envoy_ext_authz_grpc": {
			"path": "test1/allow",
			"dry-run": false
		   }
		  }
		 }`, bundleServerURL))

	opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}

	var opaRegistry *openpolicyagent.OpenPolicyAgentRegistry
	var err error

	if enableControlLoop {
		opaRegistry, err = openpolicyagent.NewOpenPolicyAgentRegistry(
			openpolicyagent.WithTracer(tracingtest.NewTracer()),
			openpolicyagent.WithPreloadingEnabled(true),
			openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
			openpolicyagent.WithInstanceStartupTimeout(startUpTimeOut),
			openpolicyagent.WithCleanInterval(cleanInterval),
			openpolicyagent.WithReuseDuration(reuseDuration),
			openpolicyagent.WithEnableCustomControlLoop(true),
			openpolicyagent.WithControlLoopInterval(controlLoopInterval),
			openpolicyagent.WithControlLoopMaxJitter(controlLoopMaxJitter),
		)
	} else {
		opaRegistry, err = openpolicyagent.NewOpenPolicyAgentRegistry(
			openpolicyagent.WithTracer(tracingtest.NewTracer()),
			openpolicyagent.WithPreloadingEnabled(true),
			openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
			openpolicyagent.WithInstanceStartupTimeout(startupTimeoutWithoutControlLoop),
			openpolicyagent.WithCleanInterval(cleanInterval),
			openpolicyagent.WithReuseDuration(reuseDuration),
		)
	}
	require.NoError(t, err)

	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
	fr.Register(ftSpec)
	fr.Register(builtin.NewStatus())

	return opaRegistry, fr
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

			bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusOK)
			defer bundleServer.Stop()

			opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL(), tc.enableControlLoop)

			initialRoutes := eskip.MustParse(`r1: Path("/failoverbundle") -> opaAuthorizeRequest("failoverbundle", "") -> status(204) -> <shunt>`)
			dc := testdataclient.New(initialRoutes)
			defer dc.Close()

			preprocessor := opaRegistry.NewPreProcessor()
			proxy := proxytest.WithRoutingOptionsWithWait(fr, routing.Options{
				FilterRegistry: fr,
				DataClients:    []routing.DataClient{dc},
				PreProcessors:  []routing.PreProcessor{preprocessor},
				PollTimeout:    routeUpdatePollingTimeout,
			}, proxyWaitTime)
			defer proxy.Close()

			// Phase 1: Establish healthy state
			require.Eventually(t, func() bool {
				inst, err := opaRegistry.GetOrStartInstance(bundleName)
				return inst != nil && err == nil && inst.Healthy()
			}, startUpTimeOut, routeUpdatePollingTimeout)

			rsp, err := makeHTTPRequest(proxy, "/failoverbundle")
			require.NoError(t, err)
			defer rsp.Body.Close()
			assert.Equal(t, http.StatusNoContent, rsp.StatusCode)

			bundleServer.SetRespCode(http.StatusServiceUnavailable)

			time.Sleep(2 * startupTimeoutWithoutControlLoop) // Give time take effects

			// Should still work with the cached bundle
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

	bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusOK)
	defer bundleServer.Stop()

	fr := make(filters.Registry)
	config := []byte(fmt.Sprintf(`{
		  "services": {"test": {"url": %q}},
          "bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
		  "plugins": {
		   "envoy_ext_authz_grpc": {
			"path": "test1/allow",
			"dry-run": false
		   }
		  }
		 }`, bundleServer.URL()))

	opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}

	var opaRegistry *openpolicyagent.OpenPolicyAgentRegistry
	var err error

	opaRegistry, err = openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
		openpolicyagent.WithInstanceStartupTimeout(startupTimeoutWithoutControlLoop),
		openpolicyagent.WithCleanInterval(3*time.Second),
		openpolicyagent.WithReuseDuration(1*time.Second),
	)

	require.NoError(t, err)

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

	time.Sleep(6 * time.Second) // Wait for two cleanup cycles time
	inst, err := opaRegistry.GetOrStartInstance(bundleName)
	assert.Nil(t, inst, "OPA instance should be cleaned up after route removal and reuse duration")
}
