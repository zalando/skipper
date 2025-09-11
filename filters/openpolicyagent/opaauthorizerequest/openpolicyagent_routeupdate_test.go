package opaauthorizerequest

import (
	"fmt"
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

const routeUpdatePollingTimeout = 12 * time.Millisecond
const startUpTimeOut = 1 * time.Second

type testPhase struct {
	routes            string
	expectedInstances int
	testPath          string
	expectedStatus    int
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
				routes:            `r1: Path("/initial") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				expectedInstances: 1,
				testPath:          "/initial",
				expectedStatus:    http.StatusNoContent,
			},
		},
		{
			msg:        "Bootstrap empty then update with OPA route",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes:            "",
				expectedInstances: 0,
				testPath:          "",
				expectedStatus:    0,
			},
			targetPhase: &testPhase{
				routes:            `r1: Path("/initial") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>`,
				expectedInstances: 1,
				testPath:          "/initial",
				expectedStatus:    http.StatusNoContent,
			},
		},
		{
			msg:        "Mixed OPA and non-OPA routes",
			bundleName: "somebundle",
			bootstrapPhase: testPhase{
				routes: `r1: Path("/secure") -> opaAuthorizeRequest("somebundle", "") -> status(204) -> <shunt>;
							r2: Path("/public") -> status(200) -> <shunt>
							`,
				testPath:          "/public",
				expectedInstances: 1,
				expectedStatus:    http.StatusOK,
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
				assert.Equal(t, ti.bootstrapPhase.expectedInstances, opaRegistry.GetInstanceCount())
			}

			if ti.targetPhase != nil && ti.targetPhase.routes != "" {
				updateRoutes := eskip.MustParse(ti.targetPhase.routes)
				dc.Update(updateRoutes, nil)

				require.Eventually(t, func() bool {
					inst, err := opaRegistry.GetOrStartInstance(ti.bundleName)
					return inst != nil && err == nil
				}, startUpTimeOut, routeUpdatePollingTimeout)

				require.Eventually(t, func() bool {
					req, _ := http.NewRequest("GET", proxy.URL+ti.targetPhase.testPath, nil)
					rsp, err := proxy.Client().Do(req)
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
