package routesrv_test

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
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

func newTestServer(t *testing.T) *routesrv.RouteServer {
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{})
	if err != nil {
		t.Errorf("cannot initialize kubernetes api: %s", err)
	}
	apiServer := httptest.NewServer(api)

	rs, err := routesrv.New(routesrv.Options{SourcePollTimeout: 3 * time.Second, KubernetesURL: apiServer.URL})
	if err != nil {
		t.Errorf("cannot initialize server: %s", err)
	}

	return rs
}

func TestNotInitializedRoutesAreNotServed(t *testing.T) {
	rs := newTestServer(t)

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
	rs := newTestServer(t)
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
