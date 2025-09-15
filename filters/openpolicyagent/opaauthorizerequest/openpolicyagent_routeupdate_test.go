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
	routeUpdatePollingTimeout = 10 * time.Millisecond
	startUpTimeOut            = 1 * time.Second
)

type testPhase struct {
	routes         string
	testPath       string
	expectedStatus int
	healthy        bool
	//bundleRespCode int           `default:"200"`
	//bundleDelay    time.Duration `default:"0"`
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

	opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL())

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
	bundleName := "recoverybundle"

	bundleServer := opatestutils.StartControllableBundleServer(bundleName, http.StatusTooManyRequests)
	defer bundleServer.Stop()

	opaRegistry, fr := setupOpaTestEnvironment(t, bundleServer.URL())

	initialRoutes := eskip.MustParse(`r1: Path("/recoverybundle") -> opaAuthorizeRequest("recoverybundle", "") -> status(204) -> <shunt>`)
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

	assertOpaInstanceHealth(t, opaRegistry, bundleName, false)

	rsp, err := makeHTTPRequest(proxy, "/recoverybundle")
	require.NoError(t, err)
	defer rsp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)

	bundleServer.SetRespCode(http.StatusOK)

	require.Eventually(t, func() bool {
		inst, err := opaRegistry.GetOrStartInstance(bundleName)
		return inst != nil && err == nil && inst.Healthy()
	}, startUpTimeOut, routeUpdatePollingTimeout)

	rsp, err = makeHTTPRequest(proxy, "/recoverybundle")
	require.NoError(t, err)
	defer rsp.Body.Close()
	assert.Equal(t, http.StatusNoContent, rsp.StatusCode)
}

func setupOpaTestEnvironment(t *testing.T, bundleServerURL string) (*openpolicyagent.OpenPolicyAgentRegistry, filters.Registry) {
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

	opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
		openpolicyagent.WithInstanceStartupTimeout(2*time.Second),
		openpolicyagent.WithCleanInterval(5*time.Minute),
		openpolicyagent.WithReuseDuration(5*time.Minute),
		openpolicyagent.WithEnableCustomControlLoop(true),
		openpolicyagent.WithControlLoopInterval(30*time.Millisecond),
		openpolicyagent.WithControlLoopMaxJitter(3*time.Millisecond),
	)
	require.NoError(t, err)

	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)
	fr.Register(ftSpec)
	fr.Register(builtin.NewStatus())

	return opaRegistry, fr
}
