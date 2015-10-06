package routing_test

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"net/http"
	"testing"
	"time"
)

func waitRoute(rt *routing.Routing, req *http.Request) <-chan *routing.Route {
	done := make(chan *routing.Route)
	go func() {
		for {
			r, _ := rt.Route(req)
			if r != nil {
				done <- r
				return
			}
		}
	}()

	return done
}

func waitUpdate(dc *testdataclient.C, upsert []*eskip.Route, deletedIds []string, fail bool) <-chan int {
	done := make(chan int)
	go func() {
		if fail {
			dc.FailNext()
		}

		dc.Update(upsert, deletedIds)
		done <- 42
	}()

	return done
}

func waitDone(to time.Duration, done ...<-chan *routing.Route) bool {
	allDone := make(chan *routing.Route)

	count := len(done)
	for _, c := range done {
		go func(c <-chan *routing.Route) {
			<-c
			count--
			if count == 0 {
				allDone <- nil
			}
		}(c)
	}

	select {
	case <-allDone:
		return true
	case <-time.After(to):
		return false
	}
}

func TestKeepsReceivingInitialRouteDataUntilSucceeds(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc.FailNext()
	dc.FailNext()
	dc.FailNext()

	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(6*pollTimeout, waitRoute(rt, req)) {
		t.Error("timeout")
	}
}

func TestReceivesInitial(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(2*pollTimeout, waitRoute(rt, req)) {
		t.Error("test timeout")
	}
}

func TestReceivesFullOnFailedUpdate(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	<-waitRoute(rt, req)
	<-waitUpdate(dc, []*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil, true)

	req, err = http.NewRequest("GET", "https://www.example.com/some-other", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(2*pollTimeout, waitRoute(rt, req)) {
		t.Error("test timeout")
	}
}

func TestReceivesUpdate(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	<-waitRoute(rt, req)
	<-waitUpdate(dc, []*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}}, nil, false)

	req, err = http.NewRequest("GET", "https://www.example.com/some-other", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(2*pollTimeout, waitRoute(rt, req)) {
		t.Error("test timeout")
	}
}

func TestReceivesDelete(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{
		{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"},
		{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	<-waitRoute(rt, req)
	<-waitUpdate(dc, nil, []string{"route1"}, false)
	time.Sleep(2 * pollTimeout)

	req, err = http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	if waitDone(2*pollTimeout, waitRoute(rt, req)) {
		t.Error("should not have found route")
	}
}

func TestMergesMultipleSources(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc1, dc2, dc3},
		PollTimeout: pollTimeout})

	req1, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	req2, err := http.NewRequest("GET", "https://www.example.com/some-other", nil)
	if err != nil {
		t.Error(err)
	}

	req3, err := http.NewRequest("GET", "https://www.example.com/another", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(2*pollTimeout,
		waitRoute(rt, req1),
		waitRoute(rt, req2),
		waitRoute(rt, req3)) {
		t.Error("test timeout")
	}
}

func TestMergesUpdatesFromMultipleSources(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc1 := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "https://www.example.org"}})
	dc2 := testdataclient.New([]*eskip.Route{{Id: "route2", Path: "/some-other", Backend: "https://other.example.org"}})
	dc3 := testdataclient.New([]*eskip.Route{{Id: "route3", Path: "/another", Backend: "https://another.example.org"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc1, dc2, dc3},
		PollTimeout: pollTimeout})

	req1, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	req2, err := http.NewRequest("GET", "https://www.example.com/some-other", nil)
	if err != nil {
		t.Error(err)
	}

	req3, err := http.NewRequest("GET", "https://www.example.com/another", nil)
	if err != nil {
		t.Error(err)
	}

	waitRoute(rt, req1)
	waitRoute(rt, req2)
	waitRoute(rt, req3)

	<-waitUpdate(dc1, []*eskip.Route{{Id: "route1", Path: "/some-changed-path", Backend: "https://www.example.org"}}, nil, false)
	<-waitUpdate(dc2, []*eskip.Route{{Id: "route2", Path: "/some-other-changed", Backend: "https://www.example.org"}}, nil, false)
	<-waitUpdate(dc3, nil, []string{"route3"}, false)

	req1, err = http.NewRequest("GET", "https://www.example.com/some-changed-path", nil)
	if err != nil {
		t.Error(err)
	}

	req2, err = http.NewRequest("GET", "https://www.example.com/some-other-changed", nil)
	if err != nil {
		t.Error(err)
	}

	req3, err = http.NewRequest("GET", "https://www.example.com/another", nil)
	if err != nil {
		t.Error(err)
	}

	if !waitDone(2*pollTimeout,
		waitRoute(rt, req1),
		waitRoute(rt, req2)) {
		t.Error("test timeout")
	}

	if waitDone(2*pollTimeout, waitRoute(rt, req3)) {
		t.Error("should not have found route")
	}
}

func TestIgnoresInvalidBackend(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	dc := testdataclient.New([]*eskip.Route{{Id: "route1", Path: "/some-path", Backend: "invalid backend"}})
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		PollTimeout: pollTimeout})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	if waitDone(2*pollTimeout, waitRoute(rt, req)) {
		t.Error("should not found route")
	}
}

func TestProcessesFilterDefinitions(t *testing.T) {
	const pollTimeout = 15 * time.Millisecond

	fr := make(filters.Registry)
	fs := &filtertest.Filter{FilterName: "filter1"}
	fr[fs.Name()] = fs

	dc := testdataclient.New([]*eskip.Route{{
		Id:      "route1",
		Path:    "/some-path",
		Filters: []*eskip.Filter{{Name: "filter1", Args: []interface{}{3.14, "roger"}}},
		Backend: "https://www.example.org"}})
	rt := routing.New(routing.Options{
		DataClients:    []routing.DataClient{dc},
		PollTimeout:    pollTimeout,
		FilterRegistry: fr})

	req, err := http.NewRequest("GET", "https://www.example.com/some-path", nil)
	if err != nil {
		t.Error(err)
	}

	select {
	case r := <-waitRoute(rt, req):
		if len(r.Filters) != 1 {
			t.Error("failed to process filters")
			return
		}

		if f, ok := r.Filters[0].(*filtertest.Filter); !ok ||
			f.FilterName != fs.Name() || len(f.Args) != 2 ||
			f.Args[0] != float64(3.14) || f.Args[1] != "roger" {
			t.Error("failed to process filters")
		}
	case <-time.After(2 * pollTimeout):
		t.Error("test timeout")
	}
}
