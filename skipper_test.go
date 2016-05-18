package skipper

import (
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func initializeProxy() *http.Handler {
	filterRegistry := builtin.MakeRegistry()
	proxy := proxy.New(routing.New(routing.Options{
		FilterRegistry: filterRegistry,
		DataClients:    []routing.DataClient{}}), proxy.OptionsNone)
	return &proxy
}

func TestOptionsDefaultsToHTTP(t *testing.T) {
	o := Options{}
	if o.isHTTPS() {
		t.FailNow()
	}
}

func TestOptionsWithCertUsesHTTPS(t *testing.T) {
	o := Options{CertPathTLS: "foo", KeyPathTLS: "bar"}
	if !o.isHTTPS() {
		t.FailNow()
	}
}

func TestWithWrongCertPathFails(t *testing.T) {
	o := Options{Address: ":9091",
		CertPathTLS: "fixtures/notFound.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	proxy := initializeProxy()

	err := listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

func TestWithWrongKeyPathFails(t *testing.T) {
	o := Options{Address: ":9091",
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/notFound.key",
	}
	proxy := initializeProxy()
	err := listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

func TestHTTPSServer(t *testing.T) {
	o := Options{
		Address:     ":9091",
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	proxy := initializeProxy()
	go listenAndServe(proxy, &o)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	r, err := client.Get("https://localhost:9091")
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	defer r.Body.Close()
	_, _ = ioutil.ReadAll(r.Body)
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}

}

func TestHTTPServer(t *testing.T) {
	o := Options{
		Address: ":9090",
	}
	proxy := initializeProxy()
	go listenAndServe(proxy, &o)
	r, err := http.Get("http://localhost:9090")
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	defer r.Body.Close()
	_, _ = ioutil.ReadAll(r.Body)
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
}
