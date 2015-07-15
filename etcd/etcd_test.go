package etcd

import (
	"encoding/json"
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
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

func checkBackend(t *testing.T, rd skipper.RawData, routeId, backend string) bool {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		t.Error("error parsing document", err)
		return false
	}

	for _, r := range d {
		if r.Id == routeId {
			return r.Backend == backend
		}
	}

	return false
}

func testBackend(t *testing.T, rd skipper.RawData, routeId, backend string) {
	if !checkBackend(t, rd, routeId, backend) {
		t.Error("backend does not match")
	}
}

func checkInitial(t *testing.T, rd skipper.RawData) (bool, string) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		return false, "error parsing document"
	}

	if len(d) != 1 {
		return false, "wrong number of routes"
	}

	r := d[0]

	if r.Id != "pdp" {
		return false, "wrong route id"
	}

	if r.MatchExp != "PathRegexp(`.*\\.html`)" {
		return false, "wrong match expression"
	}

	if len(r.Filters) != 2 {
		return false, "wrong number of filters"
	}

	checkFilter := func(f *eskip.Filter, name string, args ...interface{}) (bool, string) {
		if f.Name != name {
			return false, "wrong filter name"
		}

		if len(f.Args) != len(args) {
			return false, "wrong number of filter args"
		}

		for i, a := range args {
			if f.Args[i] != a {
				return false, "wrong filter argument"
			}
		}

		return true, ""
	}

	if ok, msg := checkFilter(r.Filters[0], "customHeader", 3.14); !ok {
		return false, msg
	}

	if ok, msg := checkFilter(r.Filters[1], "xSessionId", "v4"); !ok {
		return false, msg
	}

	if r.Backend != "https://www.zalando.de" {
		return false, "wrong backend"
	}

	return true, ""
}

func testInitial(t *testing.T, rd skipper.RawData) {
	if ok, msg := checkInitial(t, rd); !ok {
		t.Error(msg)
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
		testInitial(t, d)
	case <-time.After(15 * time.Millisecond):
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
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout")
	}
}

func waitForEtcd(dc skipper.DataClient, test func(skipper.RawData) bool) bool {
	for {
		select {
		case d := <-dc.Receive():
			if test(d) {
				return true
			}
		case <-time.After(15 * time.Millisecond):
			return false
		}
	}
}

func TestRecieveInitialAndUpdates(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	waitForEtcd(dc, func(d skipper.RawData) bool {
		ok, _ := checkInitial(t, d)
		return ok
	})

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-1.html"`, 0)
	waitForEtcd(dc, func(d skipper.RawData) bool {
		return checkBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-1.html")
	})

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-2.html"`, 0)
	waitForEtcd(dc, func(d skipper.RawData) bool {
		return checkBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-2.html")
	})

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-3.html"`, 0)
	waitForEtcd(dc, func(d skipper.RawData) bool {
		return checkBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-3.html")
	})
}

func TestReceiveInserts(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	waitForEtcd(dc, func(d skipper.RawData) bool {
		ok, _ := checkInitial(t, d)
		return ok
	})

	waitForInserts := func(done chan int) {
		var insert1, insert2, insert3 bool
		for {
			if insert1 && insert2 && insert3 {
				done <- 0
				return
			}

			d := <-dc.Receive()
			insert1 = checkBackend(t, d, "pdp1", "http://www.zalando.de/pdp-inserted-1.html")
			insert2 = checkBackend(t, d, "pdp2", "http://www.zalando.de/pdp-inserted-2.html")
			insert3 = checkBackend(t, d, "pdp3", "http://www.zalando.de/pdp-inserted-3.html")
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
