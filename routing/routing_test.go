package routing_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
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

func checkRequest(rt *routing.Routing, req *http.Request) (*routing.Route, error) {
	if r, _ := rt.Route(req); r != nil {
		return r, nil
	}

	return nil, errors.New("requested route not found")
}

func checkGetRequest(rt *routing.Routing, url string) (*routing.Route, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return checkRequest(rt, req)
}

func waitForNRouteSettingsTO(tl *loggingtest.Logger, n int, to time.Duration) error {
	return tl.WaitForN("route settings applied", n, to)
}

func waitForNRouteSettings(tl *loggingtest.Logger, n int) error {
	return waitForNRouteSettingsTO(tl, n, 12*pollTimeout)
}

func waitForRouteSetting(tl *loggingtest.Logger) error {
	return waitForNRouteSettings(tl, 1)
}

func TestKeepsReceivingInitialRouteDataUntilSucceeds(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc.FailNext()
	dc.FailNext()
	dc.FailNext()

	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}
}

func TestReceivesInitial(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}
}

func TestReceivesFullOnFailedUpdate(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	tl.Reset()
	dc.FailNext()
	dc.Update([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil)

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}
}

func TestReceivesUpdate(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	tl.Reset()
	dc.Update([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil)

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}
}

func TestReceivesDelete(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{
		{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"},
		{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	tl.Reset()
	dc.Update(nil, []string{"route1"})

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err == nil {
		t.Error("failed to delete")
	}
}

func TestUpdateDoesNotChangeRouting(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	tl.Reset()
	dc.Update(nil, nil)

	if err := waitForNRouteSettingsTO(tl, 1, 3*pollTimeout); err != nil && err != loggingtest.ErrWaitTimeout {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}
}

func TestMergesMultipleSources(t *testing.T) {
	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc1, dc2, dc3},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForNRouteSettings(tl, 3); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/another"); err != nil {
		t.Error(err)
	}
}

func TestMergesUpdatesFromMultipleSources(t *testing.T) {
	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc1, dc2, dc3},
		PollTimeout:  pollTimeout,
		Log:          tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForNRouteSettings(tl, 3); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-path"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-other"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/another"); err != nil {
		t.Error(err)
	}

	tl.Reset()

	dc1.Update([]*eskip.Route{{Id: "route1", Path: "/some-changed-path", Backend: "https://www.example.org"}}, nil)
	dc2.Update([]*eskip.Route{{Id: "route2", Path: "/some-other-changed", Backend: "https://www.example.org"}}, nil)
	dc3.Update(nil, []string{"route3"})

	if err := waitForNRouteSettings(tl, 3); err != nil {
		t.Error(err)
		return
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-changed-path"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/some-other-changed"); err != nil {
		t.Error(err)
	}

	if _, err := checkGetRequest(rt, "https://www.example.com/another"); err == nil {
		t.Error(err)
	}
}

func TestIgnoresInvalidBackend(t *testing.T) {
	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "invalid backend"}})
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer: 0,
		DataClients:  []routing.DataClient{dc},
		PollTimeout:  pollTimeout})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForNRouteSettings(tl, 3); err != loggingtest.ErrWaitTimeout {
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
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		UpdateBuffer:   0,
		DataClients:    []routing.DataClient{dc},
		PollTimeout:    pollTimeout,
		FilterRegistry: fr,
		Log:            tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	if r, err := checkGetRequest(rt, "https://www.example.com/some-path"); r == nil || err != nil {
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

	cps := []routing.PredicateSpec{&predicate{}, &predicate{}}
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout,
		Predicates:  cps,
		Log:         tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	req, err := http.NewRequest("GET", "https://www.example.com", nil)
	if err != nil {
		t.Error(err)
		return
	}

	req.Header.Set(predicateHeader, "custom1")
	if r, err := checkRequest(rt, req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://route1.example.org" {
			t.Error("custom predicate matching failed, route1")
			return
		}
	}

	req.Header.Del(predicateHeader)
	if r, err := checkRequest(rt, req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://route.example.org" {
			t.Error("custom predicate matching failed, catch-all")
			return
		}
	}
}

// TestNonMatchedStaticRoute for bug #116: non-matched static route supress wild-carded route
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
	tl := loggingtest.New()
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout,
		Predicates:  cps,
		Log:         tl})
	defer func() {
		rt.Close()
		tl.Close()
	}()

	if err := waitForRouteSetting(tl); err != nil {
		t.Error(err)
		return
	}

	req, err := http.NewRequest("GET", "https://www.example.com/foo/bar", nil)
	if err != nil {
		t.Error(err)
		return
	}

	if r, err := checkRequest(rt, req); r == nil || err != nil {
		t.Error(err)
	} else {
		if r.Backend != "https://foo.org" {
			t.Error("non-matched static route supress wild-carded route")
		}
	}
}
