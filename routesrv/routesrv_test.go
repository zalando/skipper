package routesrv_test

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routesrv"
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
		t.Errorf("cannot initialize kubernetes api: %s", err)
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
	return newRouteServerWithOptions(t, routesrv.Options{
		SourcePollTimeout: pollInterval,
		KubernetesURL:     kubeServer.URL,
	})
}

func newRouteServerWithOptions(t *testing.T, o routesrv.Options) *routesrv.RouteServer {
	t.Helper()
	rs, err := routesrv.New(o)
	if err != nil {
		t.Errorf("cannot initialize server: %s", err)
	}

	return rs
}

func parseEskipFixture(t *testing.T, fileName string) []*eskip.Route {
	t.Helper()
	eskipBytes, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("failed to open eskip fixture %s: %v", fileName, err)
	}
	routes, err := eskip.Parse(string(eskipBytes))
	if err != nil {
		t.Fatalf("eskip fixture is not valid eskip %s: %v", fileName, err)
	}

	return routes
}

func getRoutes(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)

	return w
}

func wantHTTPCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	got := w.Code
	if got != want {
		t.Errorf("wrong http status; %d != %d", got, want)
	}
}

const (
	pollInterval = 3 * time.Second
	waitTimeout  = 5 * time.Second
)

var tl *loggingtest.Logger

func TestMain(m *testing.M) {
	flag.Parse()
	tl = loggingtest.New()
	logrus.AddHook(tl)
	os.Exit(m.Run())
}

func TestNotInitializedRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t)
	rs := newRouteServer(t, ks)

	w := getRoutes(rs)

	if len(w.Body.Bytes()) > 0 {
		t.Error("uninitialized routes were served")
	}
	wantHTTPCode(t, w, http.StatusNotFound)
}

func TestEmptyRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t)
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesEmpty, waitTimeout); err != nil {
		t.Error("empty routes not received")
	}
	w := getRoutes(rs)

	if len(w.Body.Bytes()) > 0 {
		t.Error("empty routes were served")
	}
	wantHTTPCode(t, w, http.StatusNotFound)
}

func TestFetchedRoutesAreServedInEskipFormat(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Error("routes not initialized")
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
}

func TestFetchedV1IngressRoutesAreServedInEskipFormat(t *testing.T) {
	defer tl.Reset()
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/ing-v1-lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServerWithOptions(t, routesrv.Options{
		SourcePollTimeout:   pollInterval,
		KubernetesURL:       ks.URL,
		KubernetesIngressV1: true,
	})

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatal("routes not initialized")
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
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Error("routes not initialized")
	}
	w1 := getRoutes(rs)
	wantHTTPCode(t, w1, http.StatusOK)

	ks.Close()
	if err := tl.WaitFor(routesrv.LogRoutesFetchingFailed, waitTimeout); err != nil {
		t.Error("source failure not recognized")
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
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Error("routes not initialized")
	}
	w1 := getRoutes(rs)

	handler.set(newKubeAPI(t, loadKubeYAML(t, "testdata/lb-target-single.yaml")))
	if err := tl.WaitForN(routesrv.LogRoutesUpdated, 2, waitTimeout*2); err != nil {
		t.Error("routes not updated")
	}
	w2 := getRoutes(rs)

	if bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("route contents were not updated")
	}
}
