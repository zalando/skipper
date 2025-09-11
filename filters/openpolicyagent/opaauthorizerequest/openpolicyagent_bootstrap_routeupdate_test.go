package opaauthorizerequest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/opatestutils"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/tracing/tracingtest"
)

const (
	pollTimeout     = 12 * time.Millisecond
	instanceTimeout = 30 * time.Second
	bundleName      = "bundle"
)

type testSetup struct {
	bundleServer   *opatestutils.ControllableBundleServer
	opaRegistry    *openpolicyagent.OpenPolicyAgentRegistry
	filterRegistry filters.Registry
	preprocessor   routing.PreProcessor
	dataClient     *testdataclient.Client
	routing        *testRouting
}

func (ts *testSetup) close() {
	if ts.bundleServer != nil {
		ts.bundleServer.Stop()
	}
	if ts.dataClient != nil {
		ts.dataClient.Close()
	}
	if ts.routing != nil {
		ts.routing.close()
	}
}

func setupTest(t *testing.T, bundleURL string) *testSetup {
	t.Helper()

	ts := &testSetup{}
	ts.opaRegistry = createOPARegistry(t, bundleURL, bundleName)
	ts.filterRegistry = setupFilterRegistry(ts.opaRegistry)
	ts.preprocessor = ts.opaRegistry.NewPreProcessor()
	ts.dataClient = testdataclient.New([]*eskip.Route{})
	ts.routing = setupTestRouting(t, ts.filterRegistry, ts.dataClient)

	return ts
}

func setupTestWithBundle(t *testing.T) *testSetup {
	t.Helper()

	bundleServer := opatestutils.StartControllableBundleServer(bundleName)
	bundleServer.SetAvailable(true)
	ts := setupTest(t, bundleServer.URL())
	ts.bundleServer = bundleServer

	return ts
}

func setupTestWithBootstrapRoutes(t *testing.T, routes []*eskip.Route) *testSetup {
	t.Helper()

	bundleServer := opatestutils.StartControllableBundleServer(bundleName)
	bundleServer.SetAvailable(true)

	ts := &testSetup{}
	ts.bundleServer = bundleServer
	ts.opaRegistry = createOPARegistry(t, bundleServer.URL(), bundleName)
	ts.filterRegistry = setupFilterRegistry(ts.opaRegistry)
	ts.preprocessor = ts.opaRegistry.NewPreProcessor()

	// Bootstrap the preprocessor with routes
	ts.preprocessor.Do(routes)

	// Create data client with the routes
	ts.dataClient = testdataclient.New(routes)
	ts.routing = setupTestRouting(t, ts.filterRegistry, ts.dataClient)

	return ts
}

func (ts *testSetup) bootstrap(routes []*eskip.Route) {
	ts.preprocessor.Do(routes)
}

func (ts *testSetup) updateRoutes(routes []*eskip.Route) error {
	ts.preprocessor.Do(routes)
	ts.dataClient.Update(routes, nil)
	return ts.routing.waitForRouteSettings(2)
}

func (ts *testSetup) waitForInstanceLoad() error {
	return waitEventually(func() bool {
		return ts.opaRegistry.GetReadyInstanceCount() > 0
	}, instanceTimeout, 100*time.Millisecond)
}

func opaRouteTemplate(path, bundle string) string {
	return fmt.Sprintf(`Path("%s") -> opaAuthorizeRequest("%s", "") -> status(204) -> <shunt>`, path, bundle)
}

func TestOPA_PreProcessor_AtBootstrap(t *testing.T) {
	initialRoutes := eskip.MustParse(fmt.Sprintf("r1: %s", opaRouteTemplate("/initial", bundleName)))
	ts := setupTestWithBootstrapRoutes(t, initialRoutes)
	defer ts.close()

	assert.Equal(t, 1, ts.opaRegistry.GetReadyInstanceCount())
	route := ts.routing.requireRoute(t, "https://opa.test/initial")
	assert.True(t, hasFilter(route, "opaAuthorizeRequest"))
	assert.True(t, hasFilter(route, "status"))
}

func TestOPA_BootstrapEmpty_UpdateWithValidBundle(t *testing.T) {
	ts := setupTestWithBundle(t)
	defer ts.close()

	ts.bootstrap([]*eskip.Route{})
	assert.Equal(t, 0, ts.opaRegistry.GetInstanceCount())

	updatedRoutes := eskip.MustParse(fmt.Sprintf("r1: %s", opaRouteTemplate("/initial", bundleName)))

	ts.preprocessor.Do(updatedRoutes)
	require.NoError(t, ts.waitForInstanceLoad())

	ts.dataClient.Update(updatedRoutes, nil)
	require.NoError(t, ts.routing.waitForRouteSettings(2))

	route := ts.routing.requireRoute(t, "https://opa.test/initial")
	assert.True(t, hasFilter(route, "opaAuthorizeRequest"))
	assert.True(t, hasFilter(route, "status"))
}

func TestOPA_BootstrapWithUnavailableBundle_UpdateWhenAvailable(t *testing.T) {
	ts := setupTestWithBundle(t)
	ts.bundleServer.SetAvailable(false)
	defer ts.close()

	initialRoutes := eskip.MustParse(fmt.Sprintf("r1: %s", opaRouteTemplate("/initial", bundleName)))
	ts.bootstrap(initialRoutes)
	ts.routing.requireMissingRoute(t, "/initial")

	ts.bundleServer.SetAvailable(true)
	updatedRoutes := eskip.MustParse(fmt.Sprintf("r1: %s", opaRouteTemplate("/initial", bundleName)))

	require.NoError(t, ts.waitForInstanceLoad())
	require.NoError(t, ts.updateRoutes(updatedRoutes))

	route := ts.routing.requireRoute(t, "https://opa,test/initial")
	assert.True(t, hasFilter(route, "opaAuthorizeRequest"))
	assert.True(t, hasFilter(route, "status"))
}

func TestOPA_BootstrapEmpty_UpdateWithMissingBundleAndValidRoute(t *testing.T) {
	ts := setupTest(t, "http://invalid-url")
	defer ts.close()

	ts.bootstrap([]*eskip.Route{})
	assert.Equal(t, 0, ts.opaRegistry.GetReadyInstanceCount())

	updatedRoutes := eskip.MustParse(`
  r1: Path("/fail") -> opaAuthorizeRequest("nonexistent-bundle", "") -> status(204) -> <shunt>;
  r2: Path("/ok") -> status(200) -> <shunt>;
 `)

	require.NoError(t, ts.updateRoutes(updatedRoutes))

	r1 := ts.routing.requireRoute(t, "https://opa.test/fail")
	assert.True(t, hasFilter(r1, "opaAuthorizeRequest"))

	r2 := ts.routing.requireRoute(t, "https://opa.test/ok")
	assert.True(t, hasFilter(r2, "status"))
}

func TestOPA_BootstrapEmpty_UpdateWithManyInvalidBundles(t *testing.T) {
	const numInvalidRoutes = 101

	ts := setupTest(t, "http://invalid-url")
	defer ts.close()

	ts.bootstrap([]*eskip.Route{})

	routes := generateFailingRoutes(numInvalidRoutes) + `ok: Path("/ok") -> status(200) -> <shunt>`
	eskipRoutes := eskip.MustParse(routes)

	require.NoError(t, ts.updateRoutes(eskipRoutes))

	for i := 1; i <= numInvalidRoutes; i++ {
		ts.routing.requireRoute(t, fmt.Sprintf("https://opa.test/fail/%d", i))
	}

	okRoute := ts.routing.requireRoute(t, "https://opa.test/ok")
	assert.True(t, hasFilter(okRoute, "status"))
	assert.Equal(t, 0, ts.opaRegistry.GetReadyInstanceCount())
	assert.Equal(t, 0, ts.opaRegistry.GetFailedInstanceCount())
	assert.Equal(t, 101, ts.opaRegistry.GetLoadingInstanceCount())
}

// Helper functions
func createOPARegistry(t *testing.T, url, bundleName string) *openpolicyagent.OpenPolicyAgentRegistry {
	t.Helper()

	config := []byte(fmt.Sprintf(`{
  "services": {"test": {"url": %q}},
  "bundles": {"%s": {"resource": "/bundles/{{ .bundlename }}"}}
 }`, url, bundleName))

	registry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(
			openpolicyagent.WithConfigTemplate(config),
		),
		openpolicyagent.WithInstanceStartupTimeout(time.Second),
		openpolicyagent.WithCleanInterval(time.Second),
		openpolicyagent.WithReuseDuration(time.Second),
	)
	require.NoError(t, err)
	return registry
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

func hasFilter(route *routing.Route, name string) bool {
	for _, f := range route.Filters {
		if f.Name == name {
			return true
		}
	}
	return false
}

func waitEventually(condition func() bool, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("condition not met within timeout")
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

func (tr *testRouting) requireRoute(t *testing.T, url string) *routing.Route {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = req.URL.Host
	route, _ := tr.routing.Route(req)
	require.NotNil(t, route, "route should exist for URL: %s", url)
	return route
}

func (tr *testRouting) requireMissingRoute(t *testing.T, path string) {
	t.Helper()
	url := "https://opa.test" + path
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = req.URL.Host
	route, _ := tr.routing.Route(req)
	require.Nil(t, route, "route should not exist for path: %s", path)
}
