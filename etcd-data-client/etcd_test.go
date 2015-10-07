package etcd

import (
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/eskip"
	"log"
	"testing"
	"time"
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

func TestReceivesError(t *testing.T) {
	c := New(EtcdUrls, "/skippertest-invalid")
	_, err := c.GetInitial()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestReceivesInitial(t *testing.T) {
	resetData(t)

	c := New(EtcdUrls, "/skippertest")
	rs, err := c.GetInitial()

	if err != nil {
		t.Error(err)
	}

	if !checkInitial(rs) {
		t.Error("failed to receive the right docs")
	}
}

func TestReceivesUpdates(t *testing.T) {
	resetData(t)

	c := New(EtcdUrls, "/skippertest")
	c.GetInitial()

	e := etcd.NewClient(EtcdUrls)
	e.Set("/skippertest/routes/pdp", `Path("/pdp") -> "https://updated.example.org"`, 0)

	rs, ds, err := c.GetUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkBackend(rs, "pdp", "https://updated.example.org") {
		t.Error("failed to receive the right backend")
	}

	if len(ds) != 0 {
		t.Error("unexpected delete")
	}
}

func TestReceiveInsert(t *testing.T) {
	resetData(t)

	c := New(EtcdUrls, "/skippertest")
	_, err := c.GetInitial()
	if err != nil {
		t.Error(err)
	}

	e := etcd.NewClient(EtcdUrls)
	e.Set("/skippertest/routes/catalog", `Path("/pdp") -> "https://catalog.example.org"`, 0)

	rs, ds, err := c.GetUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkBackend(rs, "catalog", "https://catalog.example.org") {
		t.Error("failed to receive the right backend")
	}

	if len(ds) != 0 {
		t.Error("unexpected delete")
	}
}

func TestReceiveDelete(t *testing.T) {
	resetData(t)

	c := New(EtcdUrls, "/skippertest")
	c.GetInitial()

	e := etcd.NewClient(EtcdUrls)
	e.Delete("/skippertest/routes/pdp", false)

	rs, ds, err := c.GetUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkDeleted(ds, "pdp") {
		t.Error("failed to receive the right deleted id")
	}

	if len(rs) != 0 {
		t.Error("unexpected upsert")
	}
}
