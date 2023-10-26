package routesrv

import (
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zalando/skipper"
)

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})
	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
}

func TestServerShutdownHTTP(t *testing.T) {
	o := skipper.Options{
		SourcePollTimeout: 500 * time.Millisecond,
		SupportListener:   ":9911",
	}
	const shutdownDelay = 1 * time.Second

	address, err := findAddress()
	if err != nil {
		t.Fatalf("Failed to find address: %v", err)
	}

	o.Address, o.WaitForHealthcheckInterval = address, shutdownDelay
	baseURL := "http://" + address
	// test support listener
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	supportBaseURL := "http://" + host + o.SupportListener
	testEndpoints := []string{baseURL + "/routes", supportBaseURL + "/metrics"}

	rs, err := New(o)
	if err != nil {
		t.Fatalf("Failed to create a routesrv: %v", err)
	}

	sigs := make(chan os.Signal, 1)

	errCh := make(chan error)
	go func() {
		err := run(rs, o, sigs)
		if err != nil {
			errCh <- err
		}
	}()

	time.Sleep(o.SourcePollTimeout * 2)
	for _, u := range testEndpoints {
		rsp, err := http.DefaultClient.Get(u)
		if err != nil {
			t.Fatalf("Failed to get %q: %v", u, err)
		}
		if rsp.StatusCode != 200 {
			t.Fatalf("Failed to get expected status code 200 for %q, got: %d", u, rsp.StatusCode)
		}
	}

	// initiate shutdown
	sigs <- syscall.SIGTERM

	// test that we can fetch even within termination
	time.Sleep(shutdownDelay / 2)

	for _, u := range testEndpoints {
		rsp, err := http.DefaultClient.Get(u)
		if err != nil {
			t.Fatalf("Failed to get %q after SIGTERM: %v", u, err)
		}
		if rsp.StatusCode != 200 {
			t.Fatalf("Failed to get expected status code 200 for %q after SIGTERM, got: %d", u, rsp.StatusCode)
		}
	}

	// test that we get connection refused after shutdown
	time.Sleep(shutdownDelay / 2)

	for _, u := range testEndpoints {
		_, err = http.DefaultClient.Get(u)
		switch err {
		case nil:
			t.Fatalf("Failed to get error as expected: %q", u)
		default:
			if e := err.Error(); !strings.Contains(e, "refused") {
				t.Fatalf("Failed to get connection refused, got: %s", e)
			}
		}
	}

	select {
	case err := <-errCh:
		t.Fatalf("Failed to shutdown: %v", err)
	default:
	}
}
