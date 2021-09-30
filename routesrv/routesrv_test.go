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

const (
	pollInterval = 3 * time.Second
	waitTimeout  = 5 * time.Second
)

var tl *loggingtest.Logger

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
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, specs...)
	if err != nil {
		t.Errorf("cannot initialize kubernetes api: %s", err)
	}

	return api
}

func newKubeServer(t *testing.T, specs ...io.Reader) (*httptest.Server, *muxHandler) {
	handler := &muxHandler{handler: newKubeAPI(t, specs...)}
	return httptest.NewUnstartedServer(handler), handler
}

func loadKubeYAML(t *testing.T, path string) io.Reader {
	y, err := os.ReadFile(path)
	if err != nil {
		t.Error("failed to open kubernetes resources fixture")
	}

	return bytes.NewBuffer(y)
}

func newRouteServer(t *testing.T, kubeServer *httptest.Server) *routesrv.RouteServer {
	rs, err := routesrv.New(routesrv.Options{SourcePollTimeout: pollInterval, KubernetesURL: kubeServer.URL})
	if err != nil {
		t.Errorf("cannot initialize server: %s", err)
	}

	return rs
}

func parseEskipFixture(t *testing.T, fileName string) []*eskip.Route {
	eskipBytes, err := os.ReadFile("testdata/lb-target-multi.eskip")
	if err != nil {
		t.Error("failed to open eskip fixture")
	}
	routes, err := eskip.Parse(string(eskipBytes))
	if err != nil {
		t.Error("eskip fixture is not valid eskip")
	}

	return routes
}

func getRoutes(rs *routesrv.RouteServer) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)

	return w
}

func assertHTTPStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	got := w.Code
	if got != expected {
		t.Errorf("http status code should be %d, but was %d", expected, got)
	}
}

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
	assertHTTPStatus(t, w, http.StatusNotFound)
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
	assertHTTPStatus(t, w, http.StatusNotFound)
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

	expected := parseEskipFixture(t, "testdata/lb-target-multi.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Errorf("served routes are not valid eskip: %s", w.Body)
	}
	if !eskip.EqLists(expected, got) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(expected, got))
	}
	assertHTTPStatus(t, w, http.StatusOK)
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
	assertHTTPStatus(t, w1, http.StatusOK)

	ks.Close()
	if err := tl.WaitFor(routesrv.LogRoutesFetchingFailed, waitTimeout); err != nil {
		t.Error("source failure not recognized")
	}
	w2 := getRoutes(rs)

	if !bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("served routes changed after source failure")
	}
	assertHTTPStatus(t, w2, http.StatusOK)
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
