package etcd

import (
	"encoding/json"
	"eskip"
	"github.com/coreos/go-etcd/etcd"
	"log"
	"skipper/mock"
	"skipper/skipper"
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

func testBackend(t *testing.T, rd skipper.RawData, routeId, backend string) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		t.Error("error parsing document", err)
	}

	for _, r := range d {
		if r.Id == routeId {
			if r.Backend != backend {
				t.Error("backend does not match")
			}

			return
		}
	}

	t.Error("route not found")
}

func testInitial(t *testing.T, rd skipper.RawData) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		t.Error("error parsing document", err)
	}

	if len(d) != 1 {
		t.Error("wrong number of routes", len(d))
	}

	r := d[0]

	if r.Id != "pdp" {
		t.Error("wrong route id", r.Id)
	}

	if r.MatchExp != "PathRegexp(`.*\\.html`)" {
		t.Error("wrong match expression", r.MatchExp)
	}

	if len(r.Filters) != 2 {
		t.Error("wrong number of filters", len(r.Filters))
	}

	checkFilter := func(f *eskip.Filter, name string, args ...interface{}) {
		if f.Name != name {
			t.Error("wrong filter name", name, f.Name)
		}

		if len(f.Args) != len(args) {
			t.Error("wrong number of filter args", name, len(f.Args))
		}

		for i, a := range args {
			if f.Args[i] != a {
				t.Error("wrong filter argument", name, f.Args[i])
			}
		}
	}

	checkFilter(r.Filters[0], "customHeader", 3.14)
	checkFilter(r.Filters[1], "xSessionId", "v4")

	if r.Backend != "https://www.zalando.de" {
		t.Error("wrong backend")
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

func TestRecieveInitialAndUpdates(t *testing.T) {
	resetData(t)
	c := etcd.NewClient(mock.EtcdUrls)
	dc, _ := Make(mock.EtcdUrls, "/skippertest")

	select {
	case d := <-dc.Receive():
		testInitial(t, d)
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-1.html"`, 0)
	select {
	case d := <-dc.Receive():
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-1.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout 1")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-2.html"`, 0)
	select {
	case d := <-dc.Receive():
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-2.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout 2")
	}

	c.Set("/skippertest/routes/pdp", `Path("/pdp") -> "http://www.zalando.de/pdp-updated-3.html"`, 0)
	select {
	case d := <-dc.Receive():
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-3.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout 3")
	}
}
