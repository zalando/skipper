package etcd

import (
	"encoding/json"
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/mock"
	"log"
	"testing"
	"time"
)

func init() {
	err := mock.Etcd()
	if err != nil {
		log.Fatal(err)
	}
}

const (
	testRoute = `

        PathRegexp(".*\\.html") ->
        customHeader(3.14) ->
        xSessionId("v4") ->
        "https://www.zalando.de"
    `

	testDoc = "pdp:" + testRoute
)

func marshalAndIgnore(d interface{}) []byte {
	b, _ := json.Marshal(d)
	return b
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
	c := etcd.NewClient(mock.EtcdUrls)

	// for the tests, considering errors as not-found
	c.Delete("/skippertest", true)

	err := setAll(c, "/skippertest/routes/", map[string]string{"pdp": testRoute})
	if err != nil {
		t.Error(err)
		return
	}
}

func checkBackend(rawData string, routeId, backend string) bool {
	d, err := eskip.Parse(rawData)
	if err != nil {
		return false
	}

	for _, r := range d {
		if r.Id == routeId {
			return r.Backend == backend
		}
	}

	return false
}

func checkInitial(rawData string) bool {
	d, err := eskip.Parse(rawData)
	if err != nil {
		return false
	}

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

	if r.Backend != "https://www.zalando.de" {
		return false
	}

	return true
}

func waitForEtcd(dc *Client, test func(string) bool) bool {
	for {
		select {
		case d := <-dc.Receive():
			if test(d) {
				return true
			}
		case <-time.After(45 * time.Millisecond):
			return false
		}
	}
}

func TestReceivesInitialSettings(t *testing.T) {
	resetData(t)
	dc, err := Make(mock.EtcdUrls, "/skippertest")
	if err != nil {
		t.Error(err)
	}

	select {
	case d := <-dc.Receive():
		if !checkInitial(d) {
			t.Error("failed to receive data")
		}

		// not sure how much to invest here to do this more properly,
		// normally 30ms should be enough and this dumbeddown approach
		// works, but already happened once that it wasn't enough
	case <-time.After(30 * time.Millisecond):
		t.Error("receive timeout")
	}
}

func TestReceivesUpdatedSettings(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated.html"`, 0)

	dc, _ := Make(mock.EtcdUrls, "/skippertest")
	select {
	case d := <-dc.Receive():
		if !checkBackend(d, "pdp", "http://www.zalando.de/pdp-updated.html") {
			t.Error("failed to receive the right backend")
		}
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout")
	}
}

func TestRecieveInitialAndUpdates(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	if !waitForEtcd(dc, checkInitial) {
		t.Error("failed to get initial set of data")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-1.html"`, 0)
	if !waitForEtcd(dc, func(d string) bool {
		return checkBackend(d, "pdp", "http://www.zalando.de/pdp-updated-1.html")
	}) {
		t.Error("failed to get updated backend")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-2.html"`, 0)
	if !waitForEtcd(dc, func(d string) bool {
		return checkBackend(d, "pdp", "http://www.zalando.de/pdp-updated-2.html")
	}) {
		t.Error("failed to get updated backend")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-3.html"`, 0)
	if !waitForEtcd(dc, func(d string) bool {
		return checkBackend(d, "pdp", "http://www.zalando.de/pdp-updated-3.html")
	}) {
		t.Error("failed to get updated backend")
	}
}

func TestReceiveInserts(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	if !waitForEtcd(dc, checkInitial) {
		t.Error("failed to get initial data")
	}

	waitForInserts := func(done chan int) {
		var insert1, insert2, insert3 bool
		for {
			if insert1 && insert2 && insert3 {
				done <- 0
				return
			}

			d := <-dc.Receive()
			insert1 = checkBackend(d, "pdp1", "http://www.zalando.de/pdp-inserted-1.html")
			insert2 = checkBackend(d, "pdp2", "http://www.zalando.de/pdp-inserted-2.html")
			insert3 = checkBackend(d, "pdp3", "http://www.zalando.de/pdp-inserted-3.html")
		}
	}

	c.Set("/skippertest/routes/pdp1", `Path("/pdp1") -> "http://www.zalando.de/pdp-inserted-1.html"`, 0)
	c.Set("/skippertest/routes/pdp2", `Path("/pdp2") -> "http://www.zalando.de/pdp-inserted-2.html"`, 0)
	c.Set("/skippertest/routes/pdp3", `Path("/pdp3") -> "http://www.zalando.de/pdp-inserted-3.html"`, 0)

	done := make(chan int)
	go waitForInserts(done)
	select {
	case <-time.After(3 * time.Second):
		t.Error("failed to receive all inserts")
	case <-done:
	}
}

func TestDeleteRoute(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	if !waitForEtcd(dc, checkInitial) {
		t.Error("failed to get initial data")
	}

	_, err := c.Delete("/skippertest/routes/pdp", false)
	if err != nil {
		t.Error("failed to delete route")
	}

	if !waitForEtcd(dc, func(rawData string) bool {
		d, err := eskip.Parse(rawData)
		if err != nil {
			return false
		}

		return len(d) == 0
	}) {
		t.Error("failed to delete route")
	}
}

func TestInsertUpdateDelete(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	if !waitForEtcd(dc, checkInitial) {
		t.Error("faield to get initial data")
	}

	c.Set("/skippertest/routes/pdp1", `Path("/pdp1") -> "http://www.zalando.de/pdp-inserted-1.html"`, 0)
	c.Set("/skippertest/routes/pdp2", `Path("/pdp2") -> "http://www.zalando.de/pdp-inserted-2.html"`, 0)
	c.Delete("/skippertest/routes/pdp1", false)
	c.Set("/skippertest/routes/pdp2", `Path("/pdp2") -> "http://www.zalando.de/pdp-mod-2.html"`, 0)

	if !waitForEtcd(dc, func(rawData string) bool {
		d, err := eskip.Parse(rawData)
		if err != nil {
			return false
		}

		if len(d) != 2 {
			return false
		}

		var originalOk, modOk bool
		for _, r := range d {
			if r.Id == "pdp" && r.Backend == "https://www.zalando.de" {
				originalOk = true
			}

			if r.Id == "pdp2" && r.Backend == "http://www.zalando.de/pdp-mod-2.html" {
				modOk = true
			}
		}

		return originalOk && modOk
	}) {
		t.Error("failed to delete route")
	}
}
