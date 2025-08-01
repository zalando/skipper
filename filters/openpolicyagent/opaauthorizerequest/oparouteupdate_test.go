package opaauthorizerequest

import (
	"fmt"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/tracing/tracingtest"
	"net/http"
	"testing"
	"time"
)

const (
	pollTimeout = 15 * time.Millisecond
)

func TestOPA_WithDynamicRoutesAndPreProcessor(t *testing.T) {
	bundleName := "bundle1"
	bundleServer := startBundleServer(bundleName)
	defer bundleServer.Stop()

	config := []byte(fmt.Sprintf(`{
		"services": {
			"test": {
				"url": %q
			}
		},
		"bundles": {
			"%s": {
				"resource": "/bundles/{{ .bundlename }}"
			}
		}
	}`, bundleServer.URL(), bundleName))

	envoyMetaData := []byte(`{
		"filter_metadata": {
			"envoy.filters.http.header_to_metadata": {
				"policy_type": "ingress"
			}
		}
	}`)

	opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(
			openpolicyagent.WithConfigTemplate(config),
			openpolicyagent.WithEnvoyMetadataBytes(envoyMetaData),
		),
	)
	require.NoError(t, err)

	fr := make(filters.Registry)
	fr.Register(NewOpaAuthorizeRequestSpec(opaRegistry))
	fr.Register(builtin.NewStatus())

	initialRoutes := []*eskip.Route{}
	dc := testdataclient.New(initialRoutes)
	defer dc.Close()

	updatedRoutes := eskip.MustParse(`
		r1: Path("/initial") -> opaAuthorizeRequest("bundle1", "") -> status(204) -> <shunt>
	`)

	opaRegistry.NewPreProcessor().Do(updatedRoutes)

	tr, err := newTestRouting(fr, dc)
	require.NoError(t, err)
	defer tr.close()

	dc.Update(updatedRoutes, nil)
	require.NoError(t, tr.waitForRouteSettings(2))

	route, err := tr.getRouteForURL("https://www.z-opa.org/initial")
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Len(t, route.Filters, 2, "should have opa and status filters")

	foundOPA := false
	for _, f := range route.Filters {
		if f.Name == "opaAuthorizeRequest" {
			foundOPA = true
			break
		}
	}
	assert.True(t, foundOPA, "opaAuthorizeRequest filter should be applied by preprocessor")
}

func startBundleServer(bundleName string) *opasdktest.Server {
	return opasdktest.MustNewServer(
		opasdktest.MockBundle(fmt.Sprintf("/bundles/%s", bundleName), map[string]string{
			"main.rego": `
						package envoy.authz
						import rego.v1

						default allow := false

						allow if {
							input.parsed_path == [ "initial" ]
						}
						`,
		}),
	)

}

type testRouting struct {
	log     *loggingtest.Logger
	routing *routing.Routing
}

func newTestRouting(fr filters.Registry, dc ...routing.DataClient) (*testRouting, error) {
	log := loggingtest.New()

	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		DataClients:    dc,
		PollTimeout:    pollTimeout,
		Log:            log,
	})

	tr := &testRouting{
		log:     log,
		routing: rt,
	}

	return tr, tr.waitForRouteSettings(len(dc))
}

func (tr *testRouting) close() {
	tr.log.Close()
	tr.routing.Close()
}

func (tr *testRouting) waitForRouteSettings(expected int) error {
	timeout := 12 * pollTimeout
	return tr.log.WaitForN("route settings applied", expected, timeout)
}

func (tr *testRouting) getRouteForURL(url string) (*routing.Route, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = req.URL.Host

	route, _ := tr.routing.Route(req)
	if route == nil {
		return nil, fmt.Errorf("requested route not found: %s", req.URL.Path)
	}
	return route, nil
}
