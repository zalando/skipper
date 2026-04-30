package routesrv_test

import (
	"bytes"
	"compress/gzip"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routesrv"
	"github.com/zalando/skipper/routing"
)

type muxHandler struct {
	handler http.Handler
	mu      sync.RWMutex
}

func (m *muxHandler) set(handler http.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

func (m *muxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.handler.ServeHTTP(w, r)
}

func newKubeAPI(t *testing.T, specs ...io.Reader) http.Handler {
	t.Helper()
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, specs...)
	if err != nil {
		t.Fatalf("cannot initialize kubernetes api: %s", err)
	}
	return api
}

func newKubeServer(t *testing.T, specs ...io.Reader) (*httptest.Server, *muxHandler) {
	t.Helper()
	handler := &muxHandler{handler: newKubeAPI(t, specs...)}
	return httptest.NewUnstartedServer(handler), handler
}

func loadKubeYAML(t *testing.T, path string) io.Reader {
	t.Helper()
	y, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to open kubernetes resources fixture %s: %v", path, err)
	}
	return bytes.NewBuffer(y)
}

func newRouteServer(t *testing.T, kubeServer *httptest.Server) *routesrv.RouteServer {
	return newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     kubeServer.URL,
	})
}

func newRouteServerWithOptions(t *testing.T, o skipper.Options) *routesrv.RouteServer {
	t.Helper()
	rs, err := routesrv.New(o)
	if err != nil {
		t.Fatalf("cannot initialize server: %s", err)
	}
	return rs
}

func parseEskipFixture(t *testing.T, fileName string) []*eskip.Route {
	t.Helper()
	eskipBytes, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("failed to open eskip fixture %s: %v", fileName, err)
	}
	return eskip.MustParse(string(eskipBytes))
}

func parseIP(t *testing.T, fileName string) []byte {
	t.Helper()
	ipbytes, err := os.ReadFile(fileName)
	ipbytes = bytes.TrimSuffix(ipbytes, []byte("\n"))
	if err != nil {
		t.Fatalf("failed to open eskip fixture %s: %v", fileName, err)
	}
	return ipbytes
}

func getRoutes(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)

	return w
}

func getZoneAwareRoutes(rs *routesrv.RouteServer, zone string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes/"+zone, nil)
	rs.ServeHTTP(w, r)

	return w
}

func getZoneAwareRoutesGzip(rs *routesrv.RouteServer, zone string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes/"+zone, nil)
	r.Header.Set("Accept-Encoding", "gzip")
	rs.ServeHTTP(w, r)

	return w
}

func getZoneAwareRoutesWithHeaders(rs *routesrv.RouteServer, zone string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes/"+zone, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rs.ServeHTTP(w, r)
	return w
}

func decompressGzip(t *testing.T, b *bytes.Buffer) []byte {
	t.Helper()
	zr, err := gzip.NewReader(b)
	require.NoError(t, err)
	defer zr.Close()
	out, err := io.ReadAll(zr)
	require.NoError(t, err)
	return out
}

func getHealth(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	rs.ServeHTTP(w, r)

	return w
}

func getRedisURLs(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/swarm/redis/shards", nil)
	rs.ServeHTTP(w, r)

	return w
}

func getValkeyURLs(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/swarm/valkey/shards", nil)
	rs.ServeHTTP(w, r)

	return w
}

func headRoutes(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("HEAD", "/routes", nil)
	rs.ServeHTTP(w, r)

	return w
}

func getRoutesWithRequestHeadersSetting(rs *routesrv.RouteServer, header map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	for k, v := range header {
		r.Header.Add(k, v)
	}
	rs.ServeHTTP(w, r)

	return w
}

func wantHTTPCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	got := w.Code
	if got != want {
		t.Errorf("wrong http status; %d != %d", got, want)
	}
}

const (
	pollInterval = 100 * time.Millisecond
	waitTimeout  = 1 * time.Second
)

var tl *loggingtest.Logger

func TestMain(m *testing.M) {
	flag.Parse()
	tl = loggingtest.New()
	logrus.AddHook(tl)
	os.Exit(m.Run())
}

func TestWrongMethodToRouteSrv(t *testing.T) {
	ks, _ := newKubeServer(t)
	defer ks.Close()

	rs := newRouteServer(t, ks)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/routes", bytes.NewBufferString("foo"))
	rs.ServeHTTP(w, r)
	wantHTTPCode(t, w, http.StatusMethodNotAllowed)
}

func TestNotInitializedRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t)
	defer ks.Close()

	rs := newRouteServer(t, ks)

	w := getRoutes(rs)

	if len(w.Body.Bytes()) > 0 {
		t.Error("uninitialized routes were served")
	}
	wantHTTPCode(t, w, http.StatusNotFound)

	w = getHealth(rs)
	wantHTTPCode(t, w, http.StatusServiceUnavailable)
}

func TestEmptyRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t)
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesEmpty, waitTimeout); err != nil {
		t.Error("empty routes not received")
	}
	w := getRoutes(rs)

	if len(w.Body.Bytes()) > 0 {
		t.Error("empty routes were served")
	}
	wantHTTPCode(t, w, http.StatusNotFound)
}

func testFetchedRoutesAreServedInEskipFormat(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/lb-target-multi.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Errorf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)

	w = getHealth(rs)
	wantHTTPCode(t, w, http.StatusNoContent)
}

func TestFetchedRoutesAreServedInEskipFormat(t *testing.T) {
	testFetchedRoutesAreServedInEskipFormat(t)
}

func TestFetchedRoutesAreServedInEskipFormatDebug(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	errCH := make(chan error)
	go func() {
		err := tl.WaitFor("Inserted route 3 of 3", waitTimeout)
		errCH <- err
	}()

	testFetchedRoutesAreServedInEskipFormat(t)

	err := <-errCH
	if err != nil {
		t.Fatalf("Failed to get debug logs: %v", err)
	}
}

func TestRedisEndpointSlices(t *testing.T) {
	for _, f := range []string{
		"testdata/redis-endpointslice-single.yaml",
		"testdata/redis-endpointslice-multi.yaml",
	} {

		defer tl.Reset()
		ks, _ := newKubeServer(t, loadKubeYAML(t, f))
		ks.Start()
		defer ks.Close()
		rs := newRouteServerWithOptions(t, skipper.Options{
			SourcePollTimeout:               pollInterval,
			Kubernetes:                      true,
			KubernetesURL:                   ks.URL,
			KubernetesRedisServiceNamespace: "namespace1",
			KubernetesRedisServiceName:      "service1",
			KubernetesRedisServicePort:      6379,
			KubernetesEnableEndpointslices:  true,
		})

		w := getRedisURLs(rs)

		wantHTTPCode(t, w, http.StatusOK)

		want := parseIP(t, "testdata/redis-ip.json")
		assert.JSONEq(t, string(want), w.Body.String())
	}
}

func TestRedisEndpoints(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/redis.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:               pollInterval,
		Kubernetes:                      true,
		KubernetesURL:                   ks.URL,
		KubernetesRedisServiceNamespace: "namespace1",
		KubernetesRedisServiceName:      "service1",
		KubernetesRedisServicePort:      6379,
	})

	w := getRedisURLs(rs)

	wantHTTPCode(t, w, http.StatusOK)

	want := parseIP(t, "testdata/redis-ip.json")
	assert.JSONEq(t, string(want), w.Body.String())
}

func TestValkeyEndpointSlices(t *testing.T) {
	for _, f := range []string{
		"testdata/valkey-endpointslice-single.yaml",
		"testdata/valkey-endpointslice-multi.yaml",
	} {

		defer tl.Reset()
		ks, _ := newKubeServer(t, loadKubeYAML(t, f))
		ks.Start()
		defer ks.Close()
		rs := newRouteServerWithOptions(t, skipper.Options{
			SourcePollTimeout:                pollInterval,
			Kubernetes:                       true,
			KubernetesURL:                    ks.URL,
			KubernetesValkeyServiceNamespace: "namespace1",
			KubernetesValkeyServiceName:      "service1",
			KubernetesValkeyServicePort:      6379,
			KubernetesEnableEndpointslices:   true,
		})

		w := getValkeyURLs(rs)

		wantHTTPCode(t, w, http.StatusOK)

		want := parseIP(t, "testdata/valkey-ip.json")
		assert.JSONEq(t, string(want), w.Body.String())
	}
}

func TestValkeyEndpoints(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/valkey.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:                pollInterval,
		Kubernetes:                       true,
		KubernetesURL:                    ks.URL,
		KubernetesValkeyServiceNamespace: "namespace1",
		KubernetesValkeyServiceName:      "service1",
		KubernetesValkeyServicePort:      6379,
	})

	w := getValkeyURLs(rs)

	wantHTTPCode(t, w, http.StatusOK)

	want := parseIP(t, "testdata/valkey-ip.json")
	assert.JSONEq(t, string(want), w.Body.String())
}

func TestFetchedIngressRoutesAreServedInEskipFormat(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/ing-v1-lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/ing-v1-lb-target-multi.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)
}

func TestLastRoutesAreServedDespiteSourceFailure(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)
	wantHTTPCode(t, w1, http.StatusOK)

	ks.Close()
	if err := tl.WaitFor(routesrv.LogRoutesFetchingFailed, waitTimeout); err != nil {
		t.Fatalf("source failure not recognized: %v", err)
	}
	w2 := getRoutes(rs)

	if !bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("served routes changed after source failure")
	}
	wantHTTPCode(t, w2, http.StatusOK)
}

func TestRoutesAreUpdated(t *testing.T) {
	defer tl.Reset()
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)

	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-single.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout); err != nil {
		t.Fatalf("source failure not recognized: %v", err)
	}
	w2 := getRoutes(rs)

	if bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("route contents were not updated")
	}
}

func TestRoutesWithDefaultFilters(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
		DefaultFilters: &eskip.DefaultFilters{
			Prepend: []*eskip.Filter{
				{
					Name: "enableAccessLog",
					Args: []any{4, 5},
				},
			},
			Append: []*eskip.Filter{
				{
					Name: "status",
					Args: []any{200},
				},
			},
		},
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/lb-target-multi-with-default-filters.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)
}

func TestRoutesWithOAuth2Callback(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:     pollInterval,
		Kubernetes:            true,
		KubernetesURL:         ks.URL,
		EnableOAuth2GrantFlow: true,
		OAuth2CallbackPath:    "/.well-known/oauth2-callback",
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/lb-target-multi-with-oauth2-callback.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)
}

func TestRoutesWithEastWest(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/internal-host-explicit-route-predicate.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEastWestRangeDomains: []string{"ingress.cluster.local"},
		KubernetesEastWestRangePredicates: []*eskip.Predicate{
			{
				Name: "ClientIP",
				Args: []any{"10.2.0.0/15"},
			},
		},
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/internal-host-explicit-route-predicate.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)
}

func TestRoutesWithForwardBackend(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/forward-backend.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
		ForwardBackendURL: "http://forward.example",
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/forward-backend.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, w, http.StatusOK)
}

func TestESkipBytesHandlerWithCorrectEtag(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)

	etag := w1.Header().Get("Etag")
	header := map[string]string{"If-None-Match": etag}
	w2 := getRoutesWithRequestHeadersSetting(rs, header)

	if len(w2.Body.String()) > 0 {
		t.Errorf("expected empty routes list but got %s", w2.Body.String())
	}
	if w2.Code != http.StatusNotModified {
		t.Errorf("expected 304 status code but received incorrect status code: %d", w2.Code)
	}
}

func TestESkipBytesHandlerWithStaleEtag(t *testing.T) {
	defer tl.Reset()
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)
	etag := w1.Header().Get("Etag")
	header := map[string]string{"If-None-Match": etag}

	// update the routes, which also updates e.etag
	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-single.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout); err != nil {
		t.Fatalf("routes not updated: %v", err)
	}

	w2 := getRoutesWithRequestHeadersSetting(rs, header)

	if len(w2.Body.String()) == 0 {
		t.Errorf("expected non-empty routes list")
	}
	if w2.Code == http.StatusNotModified {
		t.Errorf("received incorrect 304 status code")
	}
}

func TestESkipBytesHandlerWithLastModified(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)

	lastModified := w1.Header().Get("Last-Modified")
	header := map[string]string{"If-Modified-Since": lastModified}
	w2 := getRoutesWithRequestHeadersSetting(rs, header)

	if len(w2.Body.String()) > 0 {
		t.Errorf("expected empty routes list but got %s", w2.Body.String())
	}
	if w2.Code != http.StatusNotModified {
		t.Errorf("expected 304 status code but received incorrect status code: %d", w2.Code)
	}
}

func TestESkipBytesHandlerWithOldLastModified(t *testing.T) {
	defer tl.Reset()
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)
	lastModified := w1.Header().Get("Last-Modified")
	header := map[string]string{"If-Modified-Since": lastModified}

	// Last-Modified has a second precision so we need to wait a bit for it to change.
	// Move clock forward instead of using time.Sleep that makes test flaky.
	routesrv.SetNow(rs, func() time.Time { return time.Now().Add(2 * time.Second) })

	// update the routes, which also updated the e.lastModified
	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-single.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout); err != nil {
		t.Fatalf("routes not updated: %v", err)
	}

	w2 := getRoutesWithRequestHeadersSetting(rs, header)

	if len(w2.Body.String()) == 0 {
		t.Errorf("expected non-empty routes list")
	}
	if w2.Code == http.StatusNotModified {
		t.Errorf("received incorrect 304 status code")
	}
}

func TestESkipBytesHandlerWithXCount(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := headRoutes(rs)
	if n := w1.Body.Len(); n != 0 {
		t.Fatalf("Failed to HEAD and get a response with body: %v", n)
	}
	countStr := w1.Header().Get(routing.RoutesCountName)
	count, err := strconv.Atoi(countStr)
	if err != nil {
		t.Fatalf("Failed to convert response header %s value '%v' to int: %v", routing.RoutesCountName, countStr, err)
	}

	N := 3
	if count != N {
		t.Errorf("Failed to get %d number of routes, got: %d", N, count)
	}
}

func TestRoutesWithEditRoute(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
		EditRoute: []*eskip.Editor{
			eskip.NewEditor(regexp.MustCompile("Host[(](.*)[)]"), "HostAny($1)"),
		},
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	responseRecorder := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/lb-target-multi-with-edit-route.eskip")
	got, err := eskip.Parse(responseRecorder.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", responseRecorder.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, responseRecorder, http.StatusOK)
}

func TestRoutesWithCloneRoute(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
		CloneRoute: []*eskip.Clone{
			eskip.NewClone(regexp.MustCompile("Host"), "HostAny"),
		},
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	responseRecorder := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/lb-target-multi-with-clone-route.eskip")
	got, err := eskip.Parse(responseRecorder.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", responseRecorder.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, responseRecorder, http.StatusOK)
}

func TestRoutesWithExplicitLBAlgorithm(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/ing-v1-lb-target-multi-explicit-lb-algo.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:                      pollInterval,
		Kubernetes:                             true,
		KubernetesURL:                          ks.URL,
		KubernetesDefaultLoadBalancerAlgorithm: "powerOfRandomNChoices",
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	responseRecorder := getRoutes(rs)

	want := parseEskipFixture(t, "testdata/ing-v1-lb-target-multi-explicit-lb-algo.eskip")
	got, err := eskip.Parse(responseRecorder.Body.String())
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %s", responseRecorder.Body)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
	wantHTTPCode(t, responseRecorder, http.StatusOK)
}

func TestESkipBytesHandlerGzip(t *testing.T) {
	defer tl.Reset()
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	testGzipResponse := func(t *testing.T, count int) {
		// Get plain response
		plainResponse := getRoutes(rs)
		plainEtag := plainResponse.Header().Get("Etag")
		plainContent := plainResponse.Body.Bytes()

		// Get gzip response
		gzipResponse := getRoutesWithRequestHeadersSetting(rs, map[string]string{"Accept-Encoding": "gzip"})
		assert.Equal(t, http.StatusOK, gzipResponse.Code)
		assert.Equal(t, "text/plain; charset=utf-8", gzipResponse.Header().Get("Content-Type"))
		assert.Equal(t, "gzip", gzipResponse.Header().Get("Content-Encoding"))
		assert.Equal(t, strconv.Itoa(count), gzipResponse.Header().Get("X-Count"))

		gzipEtag := gzipResponse.Header().Get("Etag")
		assert.NotEqual(t, plainEtag, gzipEtag, "gzip Etag should differ from plain Etag")

		zr, err := gzip.NewReader(gzipResponse.Body)
		require.NoError(t, err)
		defer zr.Close()

		gzipContent, err := io.ReadAll(zr)
		require.NoError(t, err)

		assert.Equal(t, plainContent, gzipContent, "gzip content should be equal to plain content")

		// Get gzip response using Etag
		gzipEtagResponse := getRoutesWithRequestHeadersSetting(rs, map[string]string{"If-None-Match": gzipEtag, "Accept-Encoding": "gzip"})

		assert.Equal(t, http.StatusNotModified, gzipEtagResponse.Code)
		// RFC 7232 section 4.1:
		assert.Empty(t, gzipEtagResponse.Header().Get("Content-Type"))
		assert.Empty(t, gzipEtagResponse.Header().Get("Content-Length"))
		assert.Empty(t, gzipEtagResponse.Header().Get("Content-Encoding"))
		assert.Equal(t, strconv.Itoa(count), gzipEtagResponse.Header().Get("X-Count"))
		assert.Empty(t, gzipEtagResponse.Body.String())
	}

	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))
	testGzipResponse(t, 3)

	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-single.yaml")))
	require.NoError(t, tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout))

	testGzipResponse(t, 2)
}

func TestESkipBytesHandlerGzipServedForDefaultClient(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()

	rs, err := routesrv.New(skipper.Options{
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
	})
	require.NoError(t, err)

	rs.StartUpdates()
	defer rs.StopUpdates()

	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	ts := httptest.NewServer(rs)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/routes")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, resp.Uncompressed, "expected uncompressed body")

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	routes, err := eskip.Parse(string(b))
	require.NoError(t, err)
	assert.Len(t, routes, 3)
}

func TestESkipBytesHandlerWithNoUpdate(t *testing.T) {
	defer tl.Reset()
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}
	w1 := getRoutes(rs)
	lastModified := w1.Header().Get("Last-Modified")
	header := map[string]string{"If-Modified-Since": lastModified}

	// Last-Modified has a second precision so we need to wait a bit for it to change.
	// Move clock forward instead of using time.Sleep that makes test flaky.
	routesrv.SetNow(rs, func() time.Time { return time.Now().Add(2 * time.Second) })

	// should not update the routes
	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesNoChange, 2, waitTimeout); err != nil {
		t.Fatalf("routes updated: %v", err)
	}

	w2 := getRoutesWithRequestHeadersSetting(rs, header)

	if len(w2.Body.String()) != 0 {
		t.Errorf("expected empty routes list")
	}
	if w2.Code != http.StatusNotModified {
		t.Errorf("expected 304 status code, got %d", w2.Code)
	}
}

func TestRoutesWithZone(t *testing.T) {

	for _, tc := range []struct {
		name  string
		zone  string
		ing   string
		eskip string
	}{
		{
			name:  "TwoAddrPerZone",
			zone:  "eu-central-1a",
			ing:   "testdata/zone-aware-traffic/all-zones-2-addr.yaml",
			eskip: "testdata/zone-aware-traffic/all-zones-2-addr.eskip",
		},
		{
			name:  "ThreeAddrPerZone",
			zone:  "eu-central-1a",
			ing:   "testdata/zone-aware-traffic/all-zones-3-addr.yaml",
			eskip: "testdata/zone-aware-traffic/all-zones-3-addr.eskip",
		},
		{
			name:  "AllZonesExceptZoneA",
			zone:  "eu-central-1a",
			ing:   "testdata/zone-aware-traffic/all-zones-except-zone-a.yaml",
			eskip: "testdata/zone-aware-traffic/all-zones-except-zone-a.eskip",
		},
		{
			name:  "AllZonesTopologySetToZoneB",
			zone:  "eu-central-1b",
			ing:   "testdata/zone-aware-traffic/all-zones-topology-zone-b.yaml",
			eskip: "testdata/zone-aware-traffic/all-zones-topology-zone-b.eskip",
		},
		{
			name:  "OnlyZoneA",
			zone:  "eu-central-1a",
			ing:   "testdata/zone-aware-traffic/only-zone-a.yaml",
			eskip: "testdata/zone-aware-traffic/only-zone-a.eskip",
		},
		{
			// Routes that do not meet the per-zone endpoint threshold must still
			// appear in the zone response with their original endpoints, not be dropped.
			name:  "MixedZoneThreshold",
			zone:  "eu-central-1a",
			ing:   "testdata/zone-aware-traffic/mixed-zone-threshold.yaml",
			eskip: "testdata/zone-aware-traffic/mixed-zone-threshold.eskip",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer tl.Reset()
			ks, _ := newKubeServer(t, loadKubeYAML(t, tc.ing))
			ks.Start()
			defer ks.Close()
			rs := newRouteServerWithOptions(t, skipper.Options{
				SourcePollTimeout:              pollInterval,
				Kubernetes:                     true,
				KubernetesURL:                  ks.URL,
				KubernetesEnableEndpointslices: true,
			})

			rs.StartUpdates()
			defer rs.StopUpdates()

			if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
				t.Fatalf("routes not initialized: %v", err)
			}
			w := getZoneAwareRoutes(rs, tc.zone)

			want := parseEskipFixture(t, tc.eskip)
			got, err := eskip.Parse(w.Body.String())
			if err != nil {
				t.Fatalf("served routes are not valid eskip: %s", w.Body)
			}
			if !eskip.EqLists(got, want) {
				t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
			}
			wantHTTPCode(t, w, http.StatusOK)
		})
	}
}

func TestEtagWithZoneAwareRoutingFallback(t *testing.T) {
	defer tl.Reset()

	// Initial state: 3 endpoints in zone-a, 3 in zone-b, 3 in zone-c
	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}

	w1 := getZoneAwareRoutes(rs, "eu-central-1a")
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	got1 := eskip.MustParse(w1.Body.String())
	want1 := parseEskipFixture(t, "testdata/zone-aware-traffic/all-zones-3-addr.eskip")
	if !eskip.EqLists(got1, want1) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got1, want1))
	}

	etag1 := w1.Header().Get("Etag")

	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr-updated.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout); err != nil {
		t.Fatalf("routes not updated: %v", err)
	}

	w2 := getZoneAwareRoutes(rs, "eu-central-1a")
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	got2 := eskip.MustParse(w2.Body.String())
	want2 := parseEskipFixture(t, "testdata/zone-aware-traffic/all-zones-3-addr-updated.eskip")
	if !eskip.EqLists(got2, want2) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got2, want2))
	}

	etag2 := w2.Header().Get("Etag")

	require.NotEqual(t, etag1, etag2, "Etag should change after routes update")
}

func TestZoneAwareRoutesGzip(t *testing.T) {
	defer tl.Reset()

	// 3 endpoints per zone — enough to populate zoneData
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	// plain request returns zone-filtered routes
	want := parseEskipFixture(t, "testdata/zone-aware-traffic/all-zones-3-addr.eskip")

	plainResponse := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, plainResponse.Code)
	gotPlain, err := eskip.Parse(plainResponse.Body.String())
	require.NoError(t, err)
	require.True(t, eskip.EqLists(gotPlain, want))

	// gzip request must also return zone-filtered routes, not the full set
	gzipResponse := getZoneAwareRoutesGzip(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, gzipResponse.Code)
	require.Equal(t, "gzip", gzipResponse.Header().Get("Content-Encoding"))

	zr, err := gzip.NewReader(gzipResponse.Body)
	require.NoError(t, err)
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	require.NoError(t, err)

	gotGzip, err := eskip.Parse(string(decompressed))
	require.NoError(t, err)

	require.True(t, eskip.EqLists(gotGzip, want))
}

// TestZoneAwareXCount verifies that X-Count reflects the number of routes
// in the zone-filtered response, not the full route set.
func TestZoneAwareXCount(t *testing.T) {
	defer tl.Reset()

	// all-zones-3-addr has 3 zones × 3 endpoints each.
	// Zone eu-central-1a produces 1 zone-filtered route (3 backends in that zone).
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})
	rs.StartUpdates()
	defer rs.StopUpdates()
	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	zoneWant := parseEskipFixture(t, "testdata/zone-aware-traffic/all-zones-3-addr.eskip")
	expectedCount := strconv.Itoa(len(zoneWant))

	plainResp := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, plainResp.Code)
	assert.Equal(t, expectedCount, plainResp.Header().Get(routing.RoutesCountName), "X-Count must equal the number of zone-filtered routes")

	gzipResp := getZoneAwareRoutesGzip(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, gzipResp.Code)
	assert.Equal(t, expectedCount, gzipResp.Header().Get(routing.RoutesCountName), "gzip X-Count must also reflect zone-filtered route count")

	gotPlain, err := eskip.Parse(plainResp.Body.String())
	require.NoError(t, err)
	assert.Equal(t, expectedCount, strconv.Itoa(len(gotPlain)), "X-Count must match the actual number of routes in the response body")
}

func TestZoneAwareEtag(t *testing.T) {
	defer tl.Reset()

	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})
	rs.StartUpdates()
	defer rs.StopUpdates()
	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	plainResp := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, plainResp.Code)
	plainEtag := plainResp.Header().Get("Etag")
	require.NotEmpty(t, plainEtag)

	gzipResp := getZoneAwareRoutesGzip(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, gzipResp.Code)
	gzipEtag := gzipResp.Header().Get("Etag")
	require.NotEmpty(t, gzipEtag)

	assert.NotEqual(t, plainEtag, gzipEtag, "plain and gzip ETags must differ")
	assert.Contains(t, gzipEtag, "+gzip", "gzip ETag must contain +gzip suffix")

	fullResp := getRoutes(rs)
	require.Equal(t, http.StatusOK, fullResp.Code)
	fullEtag := fullResp.Header().Get("Etag")
	assert.NotEqual(t, plainEtag, fullEtag, "zone ETag must differ from full-routes ETag")

	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr-updated.yaml")))
	require.NoError(t, tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout))

	updatedResp := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, updatedResp.Code)
	updatedEtag := updatedResp.Header().Get("Etag")
	assert.NotEqual(t, plainEtag, updatedEtag, "zone ETag must change after routes update")
}

func TestZoneAwareConditionalGetEtag(t *testing.T) {
	defer tl.Reset()

	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})
	rs.StartUpdates()
	defer rs.StopUpdates()
	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	w1 := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("Etag")
	require.NotEmpty(t, etag)

	// Should return 304 when it gets the right Etag
	w2 := getZoneAwareRoutesWithHeaders(rs, "eu-central-1a", map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Empty(t, w2.Body.String())

	// same with gzip ETag
	wg1 := getZoneAwareRoutesGzip(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, wg1.Code)
	gzipEtag := wg1.Header().Get("Etag")

	wg2 := getZoneAwareRoutesWithHeaders(rs, "eu-central-1a", map[string]string{
		"If-None-Match":   gzipEtag,
		"Accept-Encoding": "gzip",
	})
	assert.Equal(t, http.StatusNotModified, wg2.Code)
	assert.Empty(t, wg2.Body.String())

	// update routes -> new Etag
	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr-updated.yaml")))
	require.NoError(t, tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout))

	w3 := getZoneAwareRoutesWithHeaders(rs, "eu-central-1a", map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.NotEmpty(t, w3.Body.String())
}

func TestZoneAwareConditionalGetLastModified(t *testing.T) {
	defer tl.Reset()

	ks, handler := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})
	rs.StartUpdates()
	defer rs.StopUpdates()
	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	w1 := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, w1.Code)
	lastModified := w1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	w2 := getZoneAwareRoutesWithHeaders(rs, "eu-central-1a", map[string]string{"If-Modified-Since": lastModified})
	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Empty(t, w2.Body.String())

	// change Last-Modified
	routesrv.SetNow(rs, func() time.Time { return time.Now().Add(2 * time.Second) })
	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-3-addr-updated.yaml")))
	require.NoError(t, tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout))

	// old Last-Modified is now stale → 200 with new content
	w3 := getZoneAwareRoutesWithHeaders(rs, "eu-central-1a", map[string]string{"If-Modified-Since": lastModified})
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.NotEmpty(t, w3.Body.String())
}

func TestZoneAwareFallbackToAllRoutesWhenBelowThreshold(t *testing.T) {
	defer tl.Reset()

	// Serve full routing table
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/zone-aware-traffic/all-zones-2-addr.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, skipper.Options{
		SourcePollTimeout:              pollInterval,
		Kubernetes:                     true,
		KubernetesURL:                  ks.URL,
		KubernetesEnableEndpointslices: true,
	})
	rs.StartUpdates()
	defer rs.StopUpdates()
	require.NoError(t, tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout))

	expectedRoutes := parseEskipFixture(t, "testdata/zone-aware-traffic/all-zones-2-addr.eskip")

	plainResp := getZoneAwareRoutes(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, plainResp.Code)
	gotPlain, err := eskip.Parse(plainResp.Body.String())
	require.NoError(t, err)
	assert.True(t, eskip.EqLists(gotPlain, expectedRoutes), "expected full routes (fallback) when zone has <3 endpoints")

	// same for gzip
	gzipResp := getZoneAwareRoutesGzip(rs, "eu-central-1a")
	require.Equal(t, http.StatusOK, gzipResp.Code)
	require.Equal(t, "gzip", gzipResp.Header().Get("Content-Encoding"))
	gotGzip, err := eskip.Parse(string(decompressGzip(t, gzipResp.Body)))
	require.NoError(t, err)
	assert.True(t, eskip.EqLists(gotGzip, expectedRoutes), "gzip fallback must also serve full routes when zone has <3 endpoints")

	// X-Count for both must match the full route count
	fullCount := strconv.Itoa(len(expectedRoutes))
	assert.Equal(t, fullCount, plainResp.Header().Get(routing.RoutesCountName))
	assert.Equal(t, fullCount, gzipResp.Header().Get(routing.RoutesCountName))
}
