package skipper

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zalando/skipper/dataclients/routestring"
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

func TestHTTPServerShutdown(t *testing.T) {
	d := 1 * time.Second

	o := Options{
		Address:                    ":19999",
		WaitForHealthcheckInterval: d,
	}

	// simulate a backend that got a request and should be handled correctly
	dc, err := routestring.New(`r0: * -> latency("3s") -> inlineContent("OK") -> status(200) -> <shunt>`)
	if err != nil {
		t.Errorf("Failed to create dataclient: %v", err)
	}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients: []routing.DataClient{
			dc,
		},
	})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	go func() {
		if errLas := listenAndServe(proxy, &o); errLas != nil {
			t.Logf("Failed to liste and serve: %v", errLas)
		}
	}()

	pid := syscall.Getpid()
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Errorf("Failed to find current process: %v", err)
	}

	var wg sync.WaitGroup
	installSigHandler := make(chan struct{}, 1)
	go func() {
		wg.Add(1)
		defer wg.Done()
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM)

		installSigHandler <- struct{}{}

		<-sigs

		// ongoing requests passing in before shutdown
		time.Sleep(d / 2)
		r, err2 := waitConnGet("http://" + o.Address)
		if r != nil {
			defer r.Body.Close()
		}
		if err2 != nil {
			t.Fatalf("Cannot connect to the local server for testing: %v ", err2)
		}
		if r.StatusCode != 200 {
			t.Fatalf("Status code should be 200, instead got: %d\n", r.StatusCode)
		}
		body, err2 := ioutil.ReadAll(r.Body)
		if err2 != nil {
			t.Fatalf("Failed to stream response body: %v", err2)
		}
		if s := string(body); s != "OK" {
			t.Errorf("Failed to get the right content: %s", s)
		}

		// requests on closed listener should fail
		time.Sleep(d / 2)
		r2, err2 := waitConnGet("http://" + o.Address)
		if r2 != nil {
			defer r2.Body.Close()
		}
		if err2 == nil {
			t.Fatalf("Can connect to a closed server for testing")
		}
	}()

	<-installSigHandler
	time.Sleep(d / 2)

	if err = p.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("Failed to signal process: %v", err)
	}
	wg.Wait()
	time.Sleep(d)
}
