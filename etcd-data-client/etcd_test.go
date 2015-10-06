package etcd

import (
    "time"
    "testing"
    "log"
	"github.com/coreos/go-etcd/etcd"
    "github.com/zalando/skipper/eskip"
)

const receiveInitialTimeout = 1200 * time.Millisecond

const (
	testRoute = `
        PathRegexp(".*\\.html") ->
        customHeader(3.14) ->
        xSessionId("v4") ->
        "https://www.example.org"
    `

	testDoc = "pdp:" + testRoute
)

func init() {
	err := Etcd()
	if err != nil {
		log.Fatal(err)
	}
}

func checkInitial(d []*eskip.Route) bool {
	if len(d) != 1 {
		return false
	}

	r := d[0]

	if r.Id != "pdp" {
		return false
	}

	if len(r.PathRegexps) != 1 || r.PathRegexps[0] != ".*\\.html" {
		return false
	}

	if len(r.Filters) != 2 {
		return false
	}

	checkFilter := func(f *eskip.Filter, name string, args ...interface{}) bool {
		if f.Name != name {
			return false
		}

		if len(f.Args) != len(args) {
			return false
		}

		for i, a := range args {
			if f.Args[i] != a {
				return false
			}
		}

		return true
	}

	if !checkFilter(r.Filters[0], "customHeader", 3.14) {
		return false
	}

	if !checkFilter(r.Filters[1], "xSessionId", "v4") {
		return false
	}

	if r.Backend != "https://www.example.org" {
		return false
	}

	return true
}

func checkBackend(d []*eskip.Route, routeId, backend string) bool {
	for _, r := range d {
		if r.Id == routeId {
			return r.Backend == backend
		}
	}

	return false
}

func checkDeleted(ids []string, routeId string) bool {
	for _, id := range ids {
		if id == routeId {
			return true
		}
	}

	return false
}

func setAll(c *etcd.Client, dir string, data map[string]string) error {
	for name, item := range data {
		_, err := c.Set(dir+name, item, 0)
		if err != nil {
			return err
		}
	}

	return nil
}

func resetData(t *testing.T) {
	c := etcd.NewClient(EtcdUrls)

	// for the tests, considering errors as not-found
	c.Delete("/skippertest", true)

	err := setAll(c, "/skippertest/routes/", map[string]string{"pdp": testRoute})
	if err != nil {
		t.Error(err)
		return
	}
}

func TestReceivesEmptyBeforeTimeout(t *testing.T) {
    c := New(EtcdUrls, "/skippertest-invalid")

    done := make(chan int)
    go func() {
        rs, _ := c.Receive()
        if len(rs) != 0 {
            t.Error("not empty")
        }

        done <- 0
    }()

    select {
    case <-done:
    case <-time.After(2 * receiveInitialTimeout):
        t.Error("failed to receive empty on time")
    }
}

func TestReceivesInitialBeforeTimeout(t *testing.T) {
    resetData(t)
    c := New(EtcdUrls, "/skippertest")

    done := make(chan int)
    go func() {
        rs, _ := c.Receive()
        if !checkInitial(rs) {
            t.Error("invalid doc")
        }

        done <- 0
    }()

	select {
	case <-done:
	case <-time.After(30 * time.Millisecond):
		t.Error("receive timeout")
	}
}

func TestReceivesUpdates(t *testing.T) {
    resetData(t)
    c := New(EtcdUrls, "/skippertest")
    _, u := c.Receive()
	e := etcd.NewClient(EtcdUrls)
	e.Set("/skippertest/routes/pdp", `Path("/pdp") -> "https://updated.example.org"`, 0)
    ud := <-u
    if !checkBackend(ud.UpsertedRoutes, "pdp", "https://updated.example.org") {
        t.Error("failed to receive the right backend")
    }
}

func TestReceiveInsert(t *testing.T) {
    resetData(t)
    c := New(EtcdUrls, "/skippertest")
    _, u := c.Receive()
	e := etcd.NewClient(EtcdUrls)
	e.Set("/skippertest/routes/catalog", `Path("/pdp") -> "https://catalog.example.org"`, 0)
    ud := <-u
    if !checkBackend(ud.UpsertedRoutes, "catalog", "https://catalog.example.org") {
        t.Error("failed to receive the right backend")
    }
}

func TestReceiveDelete(t *testing.T) {
    resetData(t)
    c := New(EtcdUrls, "/skippertest")
    _, u := c.Receive()
	e := etcd.NewClient(EtcdUrls)
	e.Delete("/skippertest/routes/pdp", false)
    ud := <-u
    if !checkDeleted(ud.DeletedIds, "pdp") {
        t.Error("failed to receive the right deleted id")
    }
}
