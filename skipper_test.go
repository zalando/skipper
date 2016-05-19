package skipper

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

const (
	listenDelay   = 15 * time.Millisecond
	listenTimeout = 9 * listenDelay
)

func initializeProxy() *http.Handler {
	filterRegistry := builtin.MakeRegistry()
	proxy := proxy.New(routing.New(routing.Options{
		FilterRegistry: filterRegistry,
		DataClients:    []routing.DataClient{}}), proxy.OptionsNone)
	return &proxy
}

func waitConn(req func() (*http.Response, error)) (*http.Response, error) {
	to := time.After(listenTimeout)
	for {
		rsp, err := req()
		if err == nil {
			return rsp, nil
		}

		select {
		case <-to:
			return nil, err
		default:
			time.Sleep(listenDelay)
		}
	}
}

func waitConnGet(url string) (*http.Response, error) {
	return waitConn(func() (*http.Response, error) {
		return (&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true}}}).Get(url)
	})
}

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})
	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
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
	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a,
		CertPathTLS: "fixtures/notFound.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	proxy := initializeProxy()

	err = listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

func TestWithWrongKeyPathFails(t *testing.T) {
	a, err := findAddress()
	if err != nil {

	}

	o := Options{Address: a,
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/notFound.key",
	}
	proxy := initializeProxy()
	err = listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

func TestHTTPSServer(t *testing.T) {
	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{
		Address:     a,
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	proxy := initializeProxy()
	go listenAndServe(proxy, &o)

	r, err := waitConnGet("https://" + o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}

func TestHTTPServer(t *testing.T) {
	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a}
	proxy := initializeProxy()
	go listenAndServe(proxy, &o)
	r, err := waitConnGet("http://" + o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}
