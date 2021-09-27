package routesrv_test

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routesrv"
)

var tl *loggingtest.Logger

func TestMain(m *testing.M) {
	flag.Parse()
	tl = loggingtest.New()
	logrus.AddHook(tl)
	os.Exit(m.Run())
}

func newKubeAPI(t *testing.T, specs ...io.Reader) http.Handler {
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, specs...)
	if err != nil {
		t.Errorf("cannot initialize kubernetes api: %s", err)
	}

	return api
}

func newKubeServer(t *testing.T, specs ...io.Reader) *httptest.Server {
	return httptest.NewUnstartedServer(newKubeAPI(t, specs...))
}

func loadKubeYAML(t *testing.T, path string) io.Reader {
	y, err := os.ReadFile("testdata/lb-target-multi.yaml")
	if err != nil {
		t.Error("failed to open kubernetes resources fixture")
	}

	return bytes.NewBuffer(y)
}

func newRouteServer(t *testing.T, kubeServer *httptest.Server) *routesrv.RouteServer {
	rs, err := routesrv.New(routesrv.Options{SourcePollTimeout: 3 * time.Second, KubernetesURL: kubeServer.URL})
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

func TestNotInitializedRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks := newKubeServer(t)
	rs := newRouteServer(t, ks)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)

	if len(w.Body.Bytes()) > 0 {
		t.Error("uninitialized routes were served")
	}
	if w.Code != http.StatusNotFound {
		t.Error("wrong http status")
	}
}

func TestEmptyRoutesAreNotServed(t *testing.T) {
	defer tl.Reset()
	ks := newKubeServer(t)
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesEmpty, 5*time.Second); err != nil {
		t.Error("empty routes not received")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)
	if len(w.Body.Bytes()) > 0 {
		t.Error("uninitialized routes were served")
	}
	if w.Code != http.StatusNotFound {
		t.Error("wrong http status")
	}
}

func TestFetchedRoutesAreServedInEskipFormat(t *testing.T) {
	defer tl.Reset()
	ks := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, 5*time.Second); err != nil {
		t.Error("routes not initialized")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	rs.ServeHTTP(w, r)
	expected := parseEskipFixture(t, "testdata/lb-target-multi.eskip")
	got, err := eskip.Parse(w.Body.String())
	if err != nil {
		t.Error("served routes are not valid eskip")
	}
	if !eskip.EqLists(expected, got) {
		t.Errorf("output different than expected: %s", cmp.Diff(expected, got))
	}
	if w.Code != http.StatusOK {
		t.Error("wrong http status")
	}
}

func TestLastRoutesAreServedDespiteSourceFailure(t *testing.T) {
	defer tl.Reset()
	ks := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	rs := newRouteServer(t, ks)

	rs.StartUpdates()
	if err := tl.WaitFor(routesrv.LogRoutesInitialized, 5*time.Second); err != nil {
		t.Error("routes not initialized")
	}

	r1 := httptest.NewRequest("GET", "/routes", nil)
	w1 := httptest.NewRecorder()
	rs.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("wrong http status: %d", w1.Code)
	}

	ks.Close()
	if err := tl.WaitFor(routesrv.LogRoutesFetchingFailed, 5*time.Second); err != nil {
		t.Error("source failure not recognized")
	}

	r2 := httptest.NewRequest("GET", "/routes", nil)
	w2 := httptest.NewRecorder()
	rs.ServeHTTP(w2, r2)

	if !bytes.Equal(w1.Body.Bytes(), w2.Body.Bytes()) {
		t.Error("served routes changed after source failure")
	}
	if w2.Code != http.StatusOK {
		t.Errorf("wrong http status: %d", w2.Code)
	}
}
