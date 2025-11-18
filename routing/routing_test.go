package routing_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"net/http/httptest"

	"encoding/json"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

const (
	pollTimeout     = 15 * time.Millisecond
	predicateHeader = "X-Custom-Predicate"
)

type predicate struct {
	matchVal string
}

type testRouting struct {
	log     *loggingtest.Logger
	routing *routing.Routing
}

func (cp *predicate) Name() string { return "CustomPredicate" }

func (cp *predicate) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, errors.New("invalid number of args")
	}

	if matchVal, ok := args[0].(string); ok {
		cp.matchVal = matchVal
		return &predicate{matchVal}, nil
	} else {
		return nil, errors.New("invalid arg")
	}
}

func (cp *predicate) Match(r *http.Request) bool {
	return r.Header.Get(predicateHeader) == cp.matchVal
}

func newTestRoutingWithFiltersPredicates(fr filters.Registry, cps []routing.PredicateSpec, dc ...routing.DataClient) (*testRouting, error) {
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		FilterRegistry: fr,
		Predicates:     cps,
		DataClients:    dc,
		PollTimeout:    pollTimeout,
		Log:            tl})
	tr := &testRouting{tl, rt}
	return tr, tr.waitForNRouteSettings(len(dc))
}

func newTestRoutingWithFilters(fr filters.Registry, dc ...routing.DataClient) (*testRouting, error) {
	return newTestRoutingWithFiltersPredicates(fr, nil, dc...)
}

func newTestRoutingWithPredicates(cps []routing.PredicateSpec, dc ...routing.DataClient) (*testRouting, error) {
	return newTestRoutingWithFiltersPredicates(builtin.MakeRegistry(), cps, dc...)
}

func newTestRouting(dc ...routing.DataClient) (*testRouting, error) {
	return newTestRoutingWithFiltersPredicates(builtin.MakeRegistry(), nil, dc...)
}

func (tr *testRouting) waitForNRouteSettingsTO(n int, to time.Duration) error {
	return tr.log.WaitForN("route settings applied", n, to)
}

func (tr *testRouting) waitForNRouteSettings(n int) error {
	return tr.waitForNRouteSettingsTO(n, 12*pollTimeout)
}

func (tr *testRouting) waitForRouteSetting() error {
	return tr.waitForNRouteSettings(1)
}

func (tr *testRouting) checkRequest(req *http.Request) (*routing.Route, error) {
	if r, _ := tr.routing.Route(req); r != nil {
		return r, nil
	}

	return nil, errors.New("requested route not found")
}

func (tr *testRouting) checkGetRequest(url string) (*routing.Route, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return tr.checkRequest(req)
}

func (tr *testRouting) close() {
	tr.log.Close()
	tr.routing.Close()
}

func stringsAreSame(xs, ys []string) bool {
	if len(xs) != len(ys) {
		return false
	}
	diff := make(map[string]int, len(xs))
	for _, x := range xs {
		diff[x]++
	}
	for _, y := range ys {
		if _, ok := diff[y]; !ok {
			return false
		}
		diff[y]--
		if diff[y] == 0 {
			delete(diff, y)
		}
	}
	return len(diff) == 0
}

func TestKeepsReceivingInitialRouteDataUntilSucceeds(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc.Close()

	dc.FailNext()
	dc.FailNext()
	dc.FailNext()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}
}

func TestReceivesInitial(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc.Close()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
	}

	defer tr.close()

	if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}
}

func TestReceivesFullOnFailedUpdate(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc.Close()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	tr.log.Reset()
	dc.FailNext()
	dc.Update([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil)

	if err := tr.waitForRouteSetting(); err != nil {
		t.Error(err)
		return
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}
}

func TestReceivesUpdate(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc.Close()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	tr.log.Reset()
	dc.Update([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil)

	if err := tr.waitForRouteSetting(); err != nil {
		t.Error(err)
		return
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}
}

func TestReceivesDelete(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{
		{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"},
		{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	defer dc.Close()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	tr.log.Reset()
	dc.Update(nil, []string{"route1"})

	if err := tr.waitForRouteSetting(); err != nil {
		t.Error(err)
		return
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err == nil {
		t.Error("failed to delete")
	}
}

func TestUpdateDoesNotChangeRouting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
		defer dc.Close()

		tr, err := newTestRouting(dc)
		if err != nil {
			t.Error(err)
			return
		}
		defer tr.close()

		tr.log.Reset()
		dc.Update(nil, nil)

		if err := tr.waitForNRouteSettingsTO(1, 3*pollTimeout); err != nil && err != loggingtest.ErrWaitTimeout {
			t.Error(err)
			return
		}

		if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err != nil {
			t.Error(err)
		}
	})
}

func TestMergesMultipleSources(t *testing.T) {
	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc1.Close()

	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	defer dc2.Close()

	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	defer dc3.Close()

	tr, err := newTestRouting(dc1, dc2, dc3)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/another"); err != nil {
		t.Error(err)
	}
}

func TestMergesUpdatesFromMultipleSources(t *testing.T) {
	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	defer dc1.Close()

	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	defer dc2.Close()

	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	defer dc3.Close()

	tr, err := newTestRouting(dc1, dc2, dc3)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	if _, err := tr.checkGetRequest("https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/another"); err != nil {
		t.Error(err)
	}

	tr.log.Reset()

	dc1.Update([]*eskip.Route{{Id: "route1", Path: "/some-changed-path", Backend: "https://www.example.org"}}, nil)
	dc2.Update([]*eskip.Route{{Id: "route2", Path: "/some-other-changed", Backend: "https://www.example.org"}}, nil)
	dc3.Update(nil, []string{"route3"})

	if err := tr.waitForNRouteSettings(3); err != nil {
		t.Error(err)
		return
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-changed-path"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/some-other-changed"); err != nil {
		t.Error(err)
	}

	if _, err := tr.checkGetRequest("https://www.example.com/another"); err == nil {
		t.Error(err)
	}
}

func TestIgnoresInvalidBackend(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "invalid backend"}})
	defer dc.Close()

	tr, err := newTestRouting(dc)
	if err != nil {
		t.Error(err)
	}

	defer tr.close()

	if err = tr.log.WaitFor("invalid backend", time.Second); err != nil {
		t.Error(err)
	}
}

func TestProcessesFilterDefinitions(t *testing.T) {
	fr := make(filters.Registry)
	fs := &filtertest.Filter{FilterName: "filter1"}
	fr.Register(fs)

	dc := testdataclient.New([]*eskip.Route{{
		Id:      "route1",
		Path:    "/some-path",
		Filters: []*eskip.Filter{{Name: "filter1", Args: []interface{}{3.14, "Hello, world!"}}},
		Backend: "https://www.example.org"}})
	defer dc.Close()

	tr, err := newTestRoutingWithFilters(fr, dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	if r, err := tr.checkGetRequest("https://www.example.com/some-path"); r == nil || err != nil {
		t.Error(err)
	} else {
		if len(r.Filters) != 1 {
			t.Error("failed to process filters")
			return
		}

		if f, ok := r.Filters[0].Filter.(*filtertest.Filter); !ok ||
			f.FilterName != fs.Name() || len(f.Args) != 2 ||
			f.Args[0] != float64(3.14) || f.Args[1] != "Hello, world!" {
			t.Error("failed to process filters")
		}
	}
}

func TestProcessesPredicates(t *testing.T) {
	dc, err := testdataclient.NewDoc(`
        route1: CustomPredicate("custom1") -> "https://route1.example.org";
        route2: CustomPredicate("custom2") -> "https://route2.example.org";
        catchAll: * -> "https://route.example.org"`)
	if err != nil {
		t.Error(err)
		return
	}
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}

	tr, err := newTestRoutingWithPredicates(cps, dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	req, err := http.NewRequest("GET", "https://www.example.com", nil)
	if err != nil {
		t.Error(err)
		return
	}

	req.Header.Set(predicateHeader, "custom1")
	if r, err := tr.checkRequest(req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://route1.example.org" {
			t.Error("custom predicate matching failed, route1")
			return
		}
	}

	req.Header.Del(predicateHeader)
	if r, err := tr.checkRequest(req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://route.example.org" {
			t.Error("custom predicate matching failed, catch-all")
			return
		}
	}
}

// TestNonMatchedStaticRoute for bug #116: non-matched static route suppress wild-carded route
func TestNonMatchedStaticRoute(t *testing.T) {
	dc, err := testdataclient.NewDoc(`
		a: Path("/foo/*_") -> "https://foo.org";
		b: Path("/foo/bar") && CustomPredicate("custom1") -> "https://bar.org";
		z: * -> "https://catch.all"`)
	if err != nil {
		t.Error(err)
		return
	}

	cps := []routing.PredicateSpec{&predicate{}}

	tr, err := newTestRoutingWithPredicates(cps, dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	req, err := http.NewRequest("GET", "https://www.example.com/foo/bar", nil)
	if err != nil {
		t.Error(err)
		return
	}

	if r, err := tr.checkRequest(req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://foo.org" {
			t.Error("non-matched static route suppress wild-carded route")
		}
	}
}

func TestRoutingHandlerParameterChecking(t *testing.T) {
	rt := routing.New(routing.Options{})
	defer rt.Close()

	mux := http.NewServeMux()
	mux.Handle("/", rt)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "?offset=-1")
	if err != nil {
		t.Errorf("failed making get request: %v", err)
		return
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}

	resp, err = http.Get(server.URL + "?limit=-1")
	if err != nil {
		t.Errorf("failed making get request: %v", err)
		return
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}

	resp, err = http.Get(server.URL + "?offset=foo")
	if err != nil {
		t.Errorf("failed making get request: %v", err)
		return
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}

	resp, err = http.Get(server.URL + "?offset=10&limit=100")
	if err != nil {
		t.Errorf("failed making get request: %v", err)
		return
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}
}

func TestRoutingHandlerEskipResponse(t *testing.T) {
	dc, err := testdataclient.NewDoc(`
        route1: CustomPredicate("custom1") -> "https://route1.example.org";
        route2: CustomPredicate("custom2") -> "https://route2.example.org";
        catchAll: * -> "https://route.example.org"`)
	if err != nil {
		t.Error(err)
		return
	}
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}

	tr, err := newTestRoutingWithPredicates(cps, dc)
	if err != nil {
		t.Error(err)
		return
	}
	defer tr.close()

	mux := http.NewServeMux()
	mux.Handle("/", tr.routing)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}
	body := string(b)

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}

	if got, want := resp.Header.Get("content-type"), "text/plain"; got != want {
		t.Errorf("content type = %v, want %v", got, want)
	}

	routes, err := eskip.Parse(body)
	if err != nil {
		t.Error(err)
		return
	}
	if got, want := len(routes), 3; got != want {
		t.Errorf("number of routes = %v, want %v", got, want)
	}

	var routeIds []string
	for _, r := range routes {
		routeIds = append(routeIds, r.Id)
	}
	expectedRouteIds := []string{"route1", "catchAll", "route2"}
	if !stringsAreSame(routeIds, expectedRouteIds) {
		t.Errorf("routes = %v, want %v", routeIds, expectedRouteIds)
	}
}

func TestRoutingHandlerJsonResponse(t *testing.T) {
	dc, _ := testdataclient.NewDoc(`
        route1: CustomPredicate("custom1") -> "https://route1.example.org";
        route2: CustomPredicate("custom2") -> "https://route2.example.org";
        catchAll: * -> "https://route.example.org"`)
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}
	tr, _ := newTestRoutingWithPredicates(cps, dc)
	defer tr.close()

	mux := http.NewServeMux()
	mux.Handle("/", tr.routing)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("unexpected server error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status code = %v, want %v", got, want)
	}

	if got, want := resp.Header.Get("content-type"), "application/json"; got != want {
		t.Errorf("content type = %v, want %v", got, want)
	}

	var routes []*eskip.Route
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		t.Errorf("failed to encode the response body: %v", err)
	}

	if got, want := len(routes), 3; got != want {
		t.Errorf("number of routes = %v, want %v", got, want)
	}
}

func TestRoutingHandlerFilterInvalidRoutes(t *testing.T) {
	dc, _ := testdataclient.NewDoc(`
        route1: CustomPredicate("custom1") -> "https://route1.example.org";
        route2: FooBar("custom2") -> "https://route2.example.org";
        catchAll: * -> "https://route.example.org"`)
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}
	tr, _ := newTestRoutingWithPredicates(cps, dc)
	defer tr.close()

	mux := http.NewServeMux()
	mux.Handle("/", tr.routing)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("accept", "application/json")
	resp, _ := http.DefaultClient.Do(req)

	var routes []*eskip.Route
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		t.Errorf("failed to encode the response body: %v", err)
	}

	if got, want := len(routes), 2; got != want {
		t.Errorf("number of routes = %v, want %v", got, want)
	}

	routeIds := []string{}
	for _, r := range routes {
		routeIds = append(routeIds, r.Id)
	}
	expectedRouteIds := []string{"route1", "catchAll"}
	if !stringsAreSame(routeIds, expectedRouteIds) {
		t.Errorf("routes = %v, want %v", routeIds, expectedRouteIds)
	}
}

func TestRoutingHandlerPagination(t *testing.T) {
	dc, _ := testdataclient.NewDoc(`
		route1: CustomPredicate("custom1") -> "https://route1.example.org";
		route2: CustomPredicate("custom2") -> "https://route2.example.org";
		catchAll: * -> "https://route.example.org"
	`)
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}
	tr, _ := newTestRoutingWithPredicates(cps, dc)
	defer tr.close()

	mux := http.NewServeMux()
	mux.Handle("/", tr.routing)
	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		offset  int
		limit   int
		nroutes int
	}{
		{0, 0, 0},
		{0, 1, 1},
		{10, 10, 0},
		{0, 10, 3},
		{0, 3, 3},
		{1, 3, 2},
	}

	for _, ti := range tests {
		u := fmt.Sprintf("%s?offset=%d&limit=%d", server.URL, ti.offset, ti.limit)
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("accept", "application/json")
		resp, _ := http.DefaultClient.Do(req)

		if resp.Header.Get(routing.RoutesCountName) != "3" {
			t.Error("invalid or missing route count header")
		}

		var routes []*eskip.Route
		if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
			t.Errorf("failed to encode the response body: %v", err)
		}

		if got, want := len(routes), ti.nroutes; got != want {
			t.Errorf("number of routes = %v, want %v", got, want)
		}
	}
}

func TestRoutingHandlerHEAD(t *testing.T) {
	dc, _ := testdataclient.NewDoc(`
		route1: CustomPredicate("custom1") -> "https://route1.example.org";
		route2: CustomPredicate("custom2") -> "https://route2.example.org";
		catchAll: * -> "https://route.example.org"
	`)
	defer dc.Close()

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}
	tr, err := newTestRoutingWithPredicates(cps, dc)
	if err != nil {
		t.Error(err)
		return
	}

	defer tr.close()

	mux := http.NewServeMux()
	mux.Handle("/", tr.routing)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, err := http.NewRequest("HEAD", server.URL+"/routes", nil)
	if err != nil {
		t.Error(err)
		return
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
		return
	}

	defer rsp.Body.Close()

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	if len(b) != 0 {
		t.Error("unexpected payload in the response to a HEAD request")
		return
	}

	if rsp.Header.Get(routing.RoutesCountName) != "3" {
		t.Error("invalid count header")
	}
}

func TestUpdateFailsRecovers(t *testing.T) {
	l := loggingtest.New()
	defer l.Close()

	dc, err := testdataclient.NewDoc(`
		foo: Path("/foo") -> setPath("/") -> "https://foo.example.org";
		bar: Path("/bar") -> setPath("/") -> "https://bar.example.org";
		baz: Path("/baz") -> setPath("/") -> "https://baz.example.org";
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
		PollTimeout:    12 * time.Millisecond,
		Log:            l,
	})
	defer rt.Close()

	check := func(id, path, backend string, match bool) {
		if t.Failed() {
			return
		}

		if r, _ := rt.Route(&http.Request{URL: &url.URL{Path: path}}); r == nil && match {
			t.Error("route not found:", id)
		} else if r == nil && !match {
			return
		} else if !match {
			t.Error("unexpected route found:", id, path)
		} else if r.Id != id || r.Backend != backend {
			t.Error("invalid route matched")
			t.Log("got:     ", r.Id, r.Backend)
			t.Log("expected:", id, backend)
		}
	}

	if err := l.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	check("foo", "/foo", "https://foo.example.org", true)
	check("bar", "/bar", "https://bar.example.org", true)
	check("baz", "/baz", "https://baz.example.org", true)

	l.Reset()
	dc.FailNext()

	if err := dc.UpdateDoc(`
		foo: Path("/foo") -> setPath("/") -> "https://foo.example.org";
		baz: Path("/baz") -> setPath("/") -> "https://baz-new.example.org";
	`, []string{"bar"}); err != nil {
		t.Fatal(err)
	}

	if err := l.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	check("foo", "/foo", "https://foo.example.org", true)
	check("bar", "", "", false)
	check("baz", "/baz", "https://baz-new.example.org", true)
}

type valueDataClient struct{}

var _ routing.DataClient = valueDataClient{}

func (v valueDataClient) LoadAll() ([]*eskip.Route, error)              { return nil, nil }
func (v valueDataClient) LoadUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }

func TestSignalFirstLoad(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dc := testdataclient.New([]*eskip.Route{{}})
			defer dc.Close()

			l := loggingtest.New()
			defer l.Close()

			rt := routing.New(routing.Options{
				FilterRegistry: builtin.MakeRegistry(),
				DataClients:    []routing.DataClient{dc},
				PollTimeout:    1 * time.Second,
				Log:            l,
			})
			defer rt.Close()

			select {
			case <-rt.FirstLoad():
			default:
				t.Error("the first load signal was blocking")
			}

			if err := l.WaitFor("route settings applied", 1*time.Second); err != nil {
				t.Error("failed to receive route settings", err)
			}
		})
	})

	t.Run("enabled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dc := testdataclient.New([]*eskip.Route{{}})
			defer dc.Close()

			l := loggingtest.New()
			defer l.Close()

			rt := routing.New(routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  builtin.MakeRegistry(),
				DataClients:     []routing.DataClient{dc},
				PollTimeout:     1 * time.Second,
				Log:             l,
			})
			defer rt.Close()

			select {
			case <-rt.FirstLoad():
				t.Error("the first load signal was not blocking")
			default:
			}

			if err := l.WaitFor("route settings applied", 1*time.Second); err != nil {
				t.Error("failed to receive route settings", err)
			}

			select {
			case <-rt.FirstLoad():
			default:
				t.Error("the first load signal was blocking")
			}
		})
	})

	t.Run("enabled, empty", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dc := testdataclient.New(nil)
			defer dc.Close()

			l := loggingtest.New()
			defer l.Close()

			rt := routing.New(routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  builtin.MakeRegistry(),
				DataClients:     []routing.DataClient{dc},
				PollTimeout:     1 * time.Second,
				Log:             l,
			})
			defer rt.Close()

			select {
			case <-rt.FirstLoad():
				t.Error("the first load signal was not blocking")
			default:
			}

			if err := l.WaitFor("route settings applied", 1*time.Second); err != nil {
				t.Error("failed to receive route settings", err)
			}

			select {
			case <-rt.FirstLoad():
			default:
				t.Error("the first load signal was blocking")
			}
		})
	})

	t.Run("multiple data clients", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const pollTimeout = 1 * time.Second

			dc1 := testdataclient.New([]*eskip.Route{{Id: "r1", Backend: "https://foo.example.org"}})
			defer dc1.Close()

			dc2 := testdataclient.New([]*eskip.Route{{Id: "r2", Backend: "https://bar.example.org"}})
			defer dc2.Close()

			// duplicate data clients, will be removed
			dc3 := dc1
			dc4 := dc2

			// Schedule r1 update right away and delay r2 update
			go func() {
				dc1.Update([]*eskip.Route{{Id: "r1", Backend: "https://baz.example.org"}}, nil)
			}()
			dc2.WithLoadAllDelay(10 * pollTimeout)

			l := loggingtest.New()
			defer l.Close()

			rt := routing.New(routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  builtin.MakeRegistry(),
				DataClients:     []routing.DataClient{dc1, dc2, dc3, dc4},
				PollTimeout:     pollTimeout,
				Log:             l,
			})
			defer rt.Close()

			<-rt.FirstLoad()

			if validRoutes := rt.Get().ValidRoutes(); len(validRoutes) != 2 {
				t.Errorf("expected 2 valid routes, got: %v", validRoutes)
			}

			if clients := rt.Get().DataClients(); len(clients) != 2 {
				t.Errorf("expected 2 dataclients (excluding duplicates), got: %v", clients)
			}
		})
	})
}

func TestDuplicateDataClients(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := loggingtest.New()
		defer l.Close()

		dc1 := testdataclient.New([]*eskip.Route{{Id: "r1", Backend: "https://foo.example.org"}})
		defer dc1.Close()

		dc2 := dc1 // duplicate, will be removed
		dc3 := valueDataClient{}
		dc4 := valueDataClient{} // duplicate, will be removed

		rt := routing.New(routing.Options{
			SignalFirstLoad: true,
			FilterRegistry:  builtin.MakeRegistry(),
			DataClients:     []routing.DataClient{dc1, dc2, dc3, dc4},
			PollTimeout:     pollTimeout,
			Log:             l,
		})
		defer rt.Close()

		<-rt.FirstLoad()

		l.WaitFor("Removed 2 duplicate data clients", 10*pollTimeout)

		if clients := rt.Get().DataClients(); len(clients) != 2 {
			t.Errorf("expected 2 dataclients, got: %v", clients)
		}
	})
}
