package teeloopback

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/primitive"
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
	t       *testing.T
	header  http.Header
	body    string
	name    string
	closed  chan struct{}
	content string
	pending counter
	total   int
}

type counter chan int

func newCountdown(start int) counter {
	c := make(counter, 1)
	c <- start
	return c
}

func (c counter) dec() {
	v := <-c
	c <- v - 1
}

func (c counter) value() int {
	v := <-c
	c <- v
	return v
}

func (c counter) String() string {
	return fmt.Sprint(c.value())
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.pending.dec()
	pending := h.pending.value()
	h.t.Logf("%s total requests issued %d, pending %d", h.name, h.total, pending)
	if h.total == 0 {
		h.t.Error("handler is not expected to be called")
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.t.Error(err)
	}
	h.header = r.Header
	content := string(b)
	if h.content != "" {
		content = h.content
	}
	h.body = content
	_, _ = w.Write([]byte(content))

	if pending < 0 {
		h.t.Errorf("the test server %s received more requests than the %d expected", h.name, h.total)
	}
	if pending == 0 {
		close(h.closed)
	}
}

func newTestHandler(t *testing.T, content string, totalRequest int, name string) *testHandler {
	return &testHandler{
		t:       t,
		closed:  make(chan struct{}),
		content: content,
		pending: newCountdown(totalRequest),
		total:   totalRequest,
		name:    name,
	}
}

func newTestServer(t *testing.T, content string, rts int, name string) (*httptest.Server, *testHandler) {
	handler := newTestHandler(t, content, rts, name)
	server := httptest.NewServer(handler)
	return server, handler
}

func TestLoopbackAndMatchPredicate(t *testing.T) {
	// Test shadow and original server are called once when a request is tee'd
	const routeDoc = `
		original: Path("/") -> "%v";
	 	split: Path("/") && True() -> teeLoopback("A") ->"%v";
		shadow: Path("/") && True() && Tee("A") -> "%v";
	`
	ss, sh := newTestServer(t, "", 1, "shadow-server")
	defer ss.Close()

	os, oh := newTestServer(t, "", 1, "original-server")
	defer os.Close()

	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, os.URL, os.URL, ss.URL))
	registry := make(filters.Registry)
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routing.Options{
		Predicates: []routing.PredicateSpec{
			tee.New(),
			primitive.NewTrue(),
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
	<-oh.closed
	<-sh.closed
	rsp.Body.Close()
	if sh.body != testingStr || oh.body != testingStr {
		t.Error("bodies are not equal")
	}
}

func TestPreventInfiniteLoopback(t *testing.T) {
	// Loopback should stop if the teeLoopback call matches the same route.
	ss, _ := newTestServer(t, "shadow", 0, "shadow-server")
	defer ss.Close()

	os, _ := newTestServer(t, "original", 2, "original-server")
	defer os.Close()

	const routeDoc = `
	 	original: Path("/") -> teeLoopback("A") ->"%v";
		shadow: Path("/") && Tee("B") -> "%v";
	`
	routes, _ := eskip.Parse(fmt.Sprintf(routeDoc, os.URL, ss.URL))
	routingOptions := routing.Options{
		Predicates: []routing.PredicateSpec{
			tee.New(),
		},
	}
	registry := builtin.MakeRegistry()
	registry.Register(NewTeeLoopback())
	p := proxytest.WithRoutingOptions(registry, routingOptions, routes...)
	defer p.Close()

	res, err := http.Get(p.URL + "/")

	if err != nil {
		t.Error("request failed")
		return
	}
	defer res.Body.Close()
	content, err := ioutil.ReadAll(res.Body)
	c := string(content)

	if err != nil {
		t.Error("could not read the response body")
	}
	if c != "original" {
		t.Error("routes are not loaded from the main source")
	}
}
