package routesrv

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

type muxHandler struct {
	handler http.Handler
	mu      sync.RWMutex
}

func (m *muxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.handler.ServeHTTP(w, r)
}

func newKubeAPI(t *testing.T, specs ...io.Reader) http.Handler {
	t.Helper()
	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, specs...)
	if err != nil {
		t.Fatalf("cannot initialize kubernetes api: %s", err)
	}
	return api
}
func newKubeServer(t *testing.T, specs ...io.Reader) (*httptest.Server, *muxHandler) {
	t.Helper()
	handler := &muxHandler{handler: newKubeAPI(t, specs...)}
	return httptest.NewUnstartedServer(handler), handler
}

func loadKubeYAML(t *testing.T, path string) io.Reader {
	t.Helper()
	y, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to open kubernetes resources fixture %s: %v", path, err)
	}
	return bytes.NewBuffer(y)
}

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})
	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
}

func TestServerShutdownHTTP(t *testing.T) {
	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/lb-target-multi.yaml"))
	defer ks.Close()
	ks.Start()

	o := skipper.Options{
		Kubernetes:        true,
		KubernetesURL:     "http://" + ks.Listener.Addr().String(),
		SourcePollTimeout: 500 * time.Millisecond,
	}
	const shutdownDelay = 1 * time.Second

	address, err := findAddress()
	if err != nil {
		t.Fatalf("Failed to find address: %v", err)
	}
	supportAddress, err := findAddress()
	if err != nil {
		t.Fatalf("Failed to find supportAddress: %v", err)
	}

	o.Address, o.SupportListener, o.WaitForHealthcheckInterval = address, supportAddress, shutdownDelay
	baseURL := "http://" + address
	supportBaseURL := "http://" + supportAddress
	testEndpoints := []string{baseURL + "/routes", supportBaseURL + "/metrics"}

	t.Logf("kube endpoint: %q", o.KubernetesURL)
	for _, u := range testEndpoints {
		t.Logf("test endpoint: %q", u)
	}

	rs, err := New(o)
	if err != nil {
		t.Fatalf("Failed to create a routesrv: %v", err)
	}

	time.Sleep(o.SourcePollTimeout * 2)

	cli := http.Client{
		Timeout: time.Second,
	}
	rsp, err := cli.Get(o.KubernetesURL + "/api/v1/services")
	if err != nil {
		t.Fatalf("Failed to get %q: %v", o.KubernetesURL, err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get status OK for %q: %d", o.KubernetesURL, rsp.StatusCode)
	}

	sigs := make(chan os.Signal, 1)

	errCh := make(chan error)
	go func() {
		err := run(rs, o, sigs)
		if err != nil {
			errCh <- err
		}
	}()

	// make sure we started all listeners correctly
	for i := range 5 {
		var (
			err error
			rsp *http.Response
		)

		for _, u := range testEndpoints {
			rsp, err = http.DefaultClient.Get(u)
			if err != nil {
				err = fmt.Errorf("failed to get %q: %v", u, err)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if rsp.StatusCode != 200 {
				err = fmt.Errorf("failed to get expected status code 200 for %q, got: %d", u, rsp.StatusCode)

				time.Sleep(10 * time.Millisecond)
				continue
			}
			err = nil
		}
		if i == 4 && err != nil {
			t.Fatalf("Failed to get %v", err)
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

	// test that we get connection refused after 1.5 shutdown interval elapsed
	time.Sleep(shutdownDelay)

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
