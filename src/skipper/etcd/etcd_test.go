package etcd

import (
	"encoding/json"
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

var (
	testBackends map[string]interface{} = map[string]interface{}{
		"pdp": "https://www.zalando.de/pdp.html"}

	testFrontends map[string]interface{} = map[string]interface{}{
		"pdp": map[string]interface{}{
			"route":      "PathRegexp(`.*\\.html`)",
			"backend-id": "pdp",
			"filters": []interface{}{
				"pdp-custom-headers",
				"x-session-id"}}}

	testFilterSpecs map[string]interface{} = map[string]interface{}{
		"pdp-custom-headers": map[string]interface{}{
			"middleware-name": "custom-headers",
			"config": map[string]interface{}{
				"free-data": 3.14}},
		"x-session-id": map[string]interface{}{
			"middleware-name": "x-session-id",
			"config": map[string]interface{}{
				"generator": "v4"}}}
)

func marshalAndIgnore(d interface{}) []byte {
	b, _ := json.Marshal(d)
	return b
}

func setAll(c *etcd.Client, dir string, data map[string]interface{}) error {
	for name, item := range data {
		_, err := c.Set(dir+name, string(marshalAndIgnore(item)), 0)
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

	err := setAll(c, "/skippertest/backends/", testBackends)
	if err != nil {
		t.Error(err)
		return
	}

	err = setAll(c, "/skippertest/frontends/", testFrontends)
	if err != nil {
		t.Error(err)
		return
	}

	err = setAll(c, "/skippertest/filter-specs/", testFilterSpecs)
	if err != nil {
		t.Error(err)
		return
	}
}

func testBackend(t *testing.T, rd skipper.RawData, key, value string) {
	if rd.Get()["backends"].(map[string]interface{})[key].(string) != value {
		t.Error("backend pdp does not match")
	}
}

func testInitial(t *testing.T, rd skipper.RawData) {
	testBackend(t, rd, "pdp", "https://www.zalando.de/pdp.html")

	d := rd.Get()

	pdpFrontend := d["frontends"].(map[string]interface{})["pdp"].(map[string]interface{})

	if pdpFrontend["route"].(string) != "PathRegexp(`.*\\.html`)" {
		t.Error("frontend route does not match")
	}

	if pdpFrontend["backend-id"].(string) != "pdp" {
		t.Error("frontend backend id does not match")
	}

	filters := pdpFrontend["filters"].([]interface{})
	if filters[0] != "pdp-custom-headers" || filters[1] != "x-session-id" {
		t.Error("frontend filters do not match")
	}

	customHeader := d["filter-specs"].(map[string]interface{})["pdp-custom-headers"].(map[string]interface{})

	if customHeader["middleware-name"].(string) != "custom-headers" {
		t.Error("custom header middleware does not match")
	}

	if _, ok := customHeader["config"].(map[string]interface{})["free-data"].(float64); !ok {
		t.Error("custom header config does not match")
	}

	xSessionId := d["filter-specs"].(map[string]interface{})["x-session-id"].(map[string]interface{})

	if xSessionId["middleware-name"].(string) != "x-session-id" {
		t.Error("custom header middleware does not match")
	}

	if xSessionId["config"].(map[string]interface{})["generator"].(string) != "v4" {
		t.Error("custom header config does not match")
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
	c.Set("/skippertest/backends/pdp", string(marshalAndIgnore("http://www.zalando.de/pdp-updated.html")), 0)

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

	c.Set("/skippertest/backends/pdp", string(marshalAndIgnore("http://www.zalando.de/pdp-updated-1.html")), 0)
	time.Sleep(3 * time.Millisecond)
	select {
	case d := <-dc.Receive():
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-1.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout")
	}

	c.Set("/skippertest/backends/pdp", string(marshalAndIgnore("http://www.zalando.de/pdp-updated-2.html")), 0)
	time.Sleep(3 * time.Millisecond)
	select {
	case d := <-dc.Receive():
		testBackend(t, d, "pdp", "http://www.zalando.de/pdp-updated-2.html")
	case <-time.After(15 * time.Millisecond):
		t.Error("receive timeout")
	}
}
