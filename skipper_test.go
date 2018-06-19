package skipper

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"os"
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

func testListener() bool {
	for _, a := range os.Args {
		if a == "listener" {
			return true
		}
	}

	return false
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

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()

	err = listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

func TestWithWrongKeyPathFails(t *testing.T) {
	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a,
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/notFound.key",
	}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	err = listenAndServe(proxy, &o)
	if err == nil {
		t.Fatal(err)
	}
}

// to run this test, set `-args listener` for the test command
func TestHTTPSServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{
		Address:     a,
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
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

// to run this test, set `-args listener` for the test command
func TestHTTPServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
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

func TestLoadPlugins(t *testing.T) {
	o := Options{
		PluginDirs:    []string{"./_test_plugins"},
		FilterPlugins: [][]string{{"filter_noop"}},
	}
	if err := o.findAndLoadPlugins(); err != nil {
		t.Fatalf("Failed to load plugins: %s", err)
	}
}

func TestLoadPluginsFail(t *testing.T) {
	o := Options{
		PluginDirs: []string{"./_test_plugins_fail"},
	}
	if err := o.findAndLoadPlugins(); err == nil {
		t.Fatalf("did not fail to load plugins: %s", err)
	}
}
