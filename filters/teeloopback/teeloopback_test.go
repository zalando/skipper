package teeloopback

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testHandler struct {
	t      *testing.T
	name   string
	header http.Header
	body   string
	served chan struct{}
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.t.Error(err)
	}
	h.header = r.Header
	h.body = string(b)
	close(h.served)
}

func newTestHandler(t *testing.T, name string) *testHandler {
	return &testHandler{
		t:      t,
		name:   name,
		served: make(chan struct{}),
	}
}

func newTestServer(t *testing.T, name string) (*httptest.Server, *testHandler){
	handler := newTestHandler(t, name)
	server := httptest.NewServer(handler)
	return server, handler
}
func TestLoopbackAndMatchPredicate(t *testing.T) {
	const routeDoc = `
	 	original: Path("/") -> teeLoopback("A") ->"%v";
		shadow: Path("/") && Tee("A") -> "%v";
	`
	ss, sh := newTestServer(t, "shadow")
	defer ss.Close()

	os, oh := newTestServer(t, "original")
	defer os.Close()

	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, os.URL, ss.URL))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			tee.New(),
		},
	}, routes...)

	defer p.Close()

	testingStr := "TESTEST"
	req, err := http.NewRequest("GET", p.URL, strings.NewReader(testingStr))
	if err != nil {
		t.Error(err)
	}

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Error(err)
	}
	<-oh.served
	<-sh.served
	rsp.Body.Close()
	if sh.body != testingStr || oh.body != testingStr {
		t.Error("Bodies are not equal")
	}
}
func TestLoopbackDoesNotMatchItself(t *testing.T) {
	// TODO
}
func TestLoopbackAndDoesNotMatchPredicate(t *testing.T) {
	// TODO
}
