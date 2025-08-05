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

const pollTimeout = 12 * time.Millisecond

func TestOPA_WithDynamicRoutesAndPreProcessor(t *testing.T) {
	bundleName := "bundle"
	bundleServer := startBundleServer(bundleName)
	defer bundleServer.Stop()

	opaRegistry := createOPARegistry(t, bundleServer.URL(), bundleName)
	fr := setupFilterRegistry(opaRegistry)

	opaPreprocessor := opaRegistry.NewPreProcessor()
	initialRoutes := []*eskip.Route{}
	dc := testdataclient.New(initialRoutes)
	//opaPreprocessor.Do(initialRoutes)        //ToDo investigate why fail with this
	defer dc.Close()

	updatedRoutes := eskip.MustParse(fmt.Sprintf(`
		r1: Path("/initial") -> opaAuthorizeRequest("%s", "") -> status(204) -> <shunt>
	`, bundleName))
	opaPreprocessor.Do(updatedRoutes)

	tr := setupTestRouting(t, fr, dc)
	defer tr.close()

	dc.Update(updatedRoutes, nil)
	require.NoError(t, tr.waitForRouteSettings(2))

	route := tr.requireRoute(t, "https://www.z-opa.org/initial")
	require.Len(t, route.Filters, 2)
	assert.True(t, hasFilter(route, "opaAuthorizeRequest"))
	assert.True(t, hasFilter(route, "status"))
}

func TestOPA_WithMissingBundle_WithAnotherRoute(t *testing.T) {
	missingBundle := "nonexistent-bundle"
	opaRegistry := createOPARegistry(t, "http://invalid-url", missingBundle)
	fr := setupFilterRegistry(opaRegistry)

	opaPreprocessor := opaRegistry.NewPreProcessor()
	initialRoutes := []*eskip.Route{}
	dc := testdataclient.New(initialRoutes)
	opaPreprocessor.Do(initialRoutes)
	defer dc.Close()

	updatedRoutes := eskip.MustParse(`
		r1: Path("/fail") -> opaAuthorizeRequest("nonexistent-bundle", "") -> status(204) -> <shunt>;
		r2: Path("/ok") -> status(200) -> <shunt>;
	`)
	opaPreprocessor.Do(updatedRoutes)

	tr := setupTestRouting(t, fr, dc)
	defer tr.close()

	dc.Update(updatedRoutes, nil)
	require.NoError(t, tr.waitForRouteSettings(2))

	tr.requireMissingRoute(t, "/fail")
	route := tr.requireRoute(t, "https://www.z-opa.org/ok")
	assert.True(t, hasFilter(route, "status"))
}

func TestOPA_WithMultipleMissingBundles_AndOneWorkingRoute(t *testing.T) {
	const (
		numInvalidRoutes = 101 //Just exceeding the backgroundTaskChan size
		baseURL          = "http://invalid-url"
	)

	opaRegistry := createOPARegistry(t, baseURL, "{{ .bundlename }}")
	fr := setupFilterRegistry(opaRegistry)
	opaPreprocessor := opaRegistry.NewPreProcessor()

	initialRoutes := []*eskip.Route{}
	dc := testdataclient.New(initialRoutes)
	opaPreprocessor.Do(initialRoutes)
	defer dc.Close()

	// Generate invalid routes + 1 valid
	routes := generateFailingRoutes(numInvalidRoutes)
	routes += `ok: Path("/ok") -> status(200) -> <shunt>`
	eskipRoutes := eskip.MustParse(routes)

	opaPreprocessor.Do(eskipRoutes)

	tr := setupTestRouting(t, fr, dc)
	defer tr.close()

	dc.Update(eskipRoutes, nil)
	require.NoError(t, tr.waitForRouteSettings(2))

	for i := 1; i <= numInvalidRoutes; i++ {
		tr.requireMissingRoute(t, fmt.Sprintf("/fail/%d", i))
	}
	okRoute := tr.requireRoute(t, "https://www.z-opa.org/ok")
	assert.True(t, hasFilter(okRoute, "status"))
}

// --- Helpers ---
func createOPARegistry(t *testing.T, url, bundleName string) *openpolicyagent.OpenPolicyAgentRegistry {
	t.Helper()
	config := []byte(fmt.Sprintf(`{
		"services": {
			"test": { "url": %q }
		},
		"bundles": {
			"%s": { "resource": "/bundles/{{ .bundlename }}" }
		}
	}`, url, bundleName))

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
	return opaRegistry
}

func setupFilterRegistry(opaRegistry *openpolicyagent.OpenPolicyAgentRegistry) filters.Registry {
	fr := make(filters.Registry)
	fr.Register(NewOpaAuthorizeRequestSpec(opaRegistry))
	fr.Register(builtin.NewStatus())
	return fr
}

func generateFailingRoutes(n int) string {
	var routes string
	for i := 1; i <= n; i++ {
		routes += fmt.Sprintf("fail%d: Path(\"/fail/%d\") -> opaAuthorizeRequest(\"invalid-bundle-%d\", \"\") -> status(403) -> <shunt>;", i, i, i)
	}
	return routes
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

func hasFilter(route *routing.Route, name string) bool {
	for _, f := range route.Filters {
		if f.Name == name {
			return true
		}
	}
	return false
}

type testRouting struct {
	log     *loggingtest.Logger
	routing *routing.Routing
}

func setupTestRouting(t *testing.T, fr filters.Registry, dc routing.DataClient) *testRouting {
	t.Helper()
	log := loggingtest.New()
	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		DataClients:    []routing.DataClient{dc},
		PollTimeout:    pollTimeout,
		Log:            log,
	})
	tr := &testRouting{log: log, routing: rt}
	require.NoError(t, tr.waitForRouteSettings(1))
	return tr
}

func (tr *testRouting) close() {
	tr.log.Close()
	tr.routing.Close()
}

func (tr *testRouting) waitForRouteSettings(expected int) error {
	return tr.log.WaitForN("route settings applied", expected, 12*pollTimeout)
}

func (tr *testRouting) getRouteForURL(url string) (*routing.Route, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = req.URL.Host
	route, _ := tr.routing.Route(req)
	if route == nil {
		return nil, fmt.Errorf("requested route not found: %s", req.URL.Path)
	}
	return route, nil
}

func (tr *testRouting) requireRoute(t *testing.T, url string) *routing.Route {
	t.Helper()
	route, err := tr.getRouteForURL(url)
	require.NoError(t, err)
	require.NotNil(t, route)
	return route
}

func (tr *testRouting) requireMissingRoute(t *testing.T, path string) {
	t.Helper()
	url := "https://www.z-opa.org" + path
	route, err := tr.getRouteForURL(url)
	require.EqualError(t, err, fmt.Sprintf("requested route not found: %s", path))
	require.Nil(t, route)
}
