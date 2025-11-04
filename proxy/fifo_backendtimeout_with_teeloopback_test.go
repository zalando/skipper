package proxy_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	skpiotest "github.com/zalando/skipper/io/iotest"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/nettest"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
	"golang.org/x/time/rate"
)

const sourcePollTimeout time.Duration = 6 * time.Millisecond

func TestBackendTimeoutWithSlowBodyShadow(t *testing.T) {
	proxyLog := proxy.NewTestLog()
	defer proxyLog.Close()
	backend := createBackend(t)
	defer backend.Close()

	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sr := skpiotest.NewSlowReader(r.Body, 2*time.Millisecond)
		io.ReadAll(sr)
		// buf, err := io.ReadAll(sr)
		// if err != nil {
		// 	t.Logf("slowBackend: failed to read body: %v", err)
		// } else {
		// 	t.Logf("slowBackend: read %d bytes", len(buf))
		// }
		w.WriteHeader(599)
		w.Write([]byte("slow backend"))
	}))
	defer slowBackend.Close()

	p, mockMetrics, closeProxy := createProxy(t, backend, slowBackend)
	defer closeProxy()

	N := 500 //500000
	resCH := make(chan int, N)
	client, closeClient := createClient(p, 800*time.Millisecond, 120*time.Millisecond)
	defer closeClient()
	sendRequests(t, N, p, client, resCH)
	logFifoMetrics(t, mockMetrics)
	close(resCH)
	checkStatusCode(t, resCH, N)

	// check that we can hit the main route now again correctly
	checkMainRouteIsFine(t, p, client)

	if err := proxyLog.WaitFor("failed to execute loopback request: dialing failed false: context deadline exceeded", time.Second); err != nil {
		t.Fatalf("Failed to get expected error: %v", err)
	} else {
		t.Log("Found error log")
	}
}

func TestBackendTimeoutWithSlowBodyWriterShadow(t *testing.T) {
	proxyLog := proxy.NewTestLog()
	defer proxyLog.Close()
	backend := createBackend(t)
	defer backend.Close()

	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		sw := skpiotest.NewSlowResponseWriter(w, 10*time.Millisecond)
		sw.WriteHeader(599)
		sw.Flush()

		from := bytes.NewBufferString(strings.Repeat("A", 150*1024))
		b := make([]byte, 1024)
		io.CopyBuffer(sw, from, b)
	}))
	defer slowBackend.Close()

	p, mockMetrics, closeProxy := createProxy(t, backend, slowBackend)
	defer closeProxy()

	N := 500 //500000
	resCH := make(chan int, N)
	client, closeClient := createClient(p, 80*time.Millisecond, 120*time.Millisecond)
	defer closeClient()
	sendRequests(t, N, p, client, resCH)
	logFifoMetrics(t, mockMetrics)
	close(resCH)
	checkStatusCode(t, resCH, N)

	// check that we can hit the main route now again correctly
	shouldReturn := checkMainRouteIsFine(t, p, client)
	if shouldReturn {
		return
	}

	if err := proxyLog.WaitFor("failed to execute loopback request: dialing failed false: context deadline exceeded", time.Second); err != nil {
		t.Fatalf("Failed to get expected error: %v", err)
	} else {
		t.Log(`Found "failed to execute loopback request" error log`)
	}

	if err := proxyLog.WaitFor("context: error while discarding remainder response body", time.Second); err != nil {
		t.Fatalf("Failed to get expected error: %v", err)
	} else {
		t.Log(`Found "discarding remainder response body" error log`)
	}

}

func TestBackendTimeoutWithConnectTimingOutShadow(t *testing.T) {
	proxyLog := proxy.NewTestLog()
	defer proxyLog.Close()
	backend := createBackend(t)
	defer backend.Close()

	slowBackend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//println("We should not be HERE:", r.URL.Path)
		sr := skpiotest.NewSlowReader(r.Body, 1*time.Microsecond)
		io.ReadAll(sr)
		w.WriteHeader(599)
		w.Write([]byte("slow backend"))
	}))
	l, err := nettest.NewSlowAcceptListener(&nettest.SlowAcceptListenerOptions{
		Network: "tcp",
		Address: ":0",
		Delay:   1900 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	slowBackend.Listener = l
	slowBackend.Start()
	defer slowBackend.Close()

	p, mockMetrics, closeProxy := createProxy(t, backend, slowBackend)
	defer closeProxy()

	N := 500 //500000
	resCH := make(chan int, N)
	client, closeClient := createClient(p, 80*time.Millisecond, 1500*time.Millisecond)
	defer closeClient()
	client.CloseIdleConnections()
	sendRequests(t, N, p, client, resCH)
	logFifoMetrics(t, mockMetrics)
	close(resCH)
	checkStatusCode(t, resCH, N)

	// restore to be fast listener
	l.(*nettest.SlowAcceptListener).Delay(time.Microsecond)
	client.CloseIdleConnections()

	// check that we can hit the main route now again correctly
	checkMainRouteIsFine(t, p, client)

	// if err := proxyLog.WaitFor("failed to execute loopback request: dialing failed false: context deadline exceeded", time.Second); err != nil {
	// 	t.Fatalf("Failed to get expected error: %v", err)
	// } else {
	// 	t.Log("Found error log")
	// }
}

func createClient(p *proxytest.TestProxy, timeout, rspHeaderTimeout time.Duration) (*skpnet.Client, func()) {
	client := skpnet.NewClient(skpnet.Options{
		Timeout:               timeout,
		ResponseHeaderTimeout: rspHeaderTimeout,
		IdleConnTimeout:       time.Second,
		Log:                   p.Log,
		MaxIdleConnsPerHost:   100,
		MaxIdleConns:          100,
	})
	f := func() {
		defer client.Close()
		defer client.CloseIdleConnections()
	}
	return client, f
}

func createProxy(t *testing.T, backend *httptest.Server, slowBackend *httptest.Server) (*proxytest.TestProxy, *metricstest.MockMetrics, func()) {
	t.Helper()
	doc := fmt.Sprintf(`
main: PathSubtree("/") -> fifo(5000, 20, "1s") -> teeLoopback("tag") -> "%s";
shadow: PathSubtree("/") && Tee("tag") -> fifo(5, 40, "150ms") -> backendTimeout("20ms") -> "%s";`,
		backend.URL, slowBackend.URL)

	fr := builtin.MakeRegistry()
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Fatalf("Failed to create dataclient: %v", err)
	}
	mockMetrics := &metricstest.MockMetrics{}
	epRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	schedulerRegistry := scheduler.RegistryWith(scheduler.Options{
		Metrics:                mockMetrics,
		MetricsUpdateTimeout:   100 * time.Millisecond,
		EnableRouteFIFOMetrics: true,
		EnableRouteLIFOMetrics: true,
	})

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: fr,
			PollTimeout:    sourcePollTimeout,
			DataClients:    []routing.DataClient{dc},
			Metrics:        mockMetrics,
			PostProcessors: []routing.PostProcessor{
				loadbalancer.NewAlgorithmProvider(),
				epRegistry,
				schedulerRegistry,
			},
			Predicates: []routing.PredicateSpec{teePredicate.New()},
		},
		ProxyParams: proxy.Params{
			CloseIdleConnsPeriod: time.Second,
			EndpointRegistry:     epRegistry,
			Metrics:              mockMetrics,
		},
	}.Create()

	close := func() {
		defer dc.Close()
		defer mockMetrics.Close()
		defer schedulerRegistry.Close()
		defer p.Close()
	}

	return p, mockMetrics, close
}

func createBackend(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			t.Logf("backend: failed to read body: %v", err)
			w.WriteHeader(500)
			fmt.Fprintf(w, "backend: failed to read body: %v", err.Error())
		} else {
			w.WriteHeader(200)
			fmt.Fprintf(w, "backend: read %d of data", len(buf))
		}
	}))
}

func checkMainRouteIsFine(t *testing.T, p *proxytest.TestProxy, client *skpnet.Client) bool {
	t.Helper()
	bodyData := strings.Repeat("A", 1023) + "\n"
	buf := bytes.NewBufferString(bodyData)
	sr := skpiotest.NewSlowReader(buf, 1*time.Microsecond)
	req, err := http.NewRequest("PUT", p.URL, sr)
	if err != nil {
		t.Logf("Failed to create request: %v", err)
		return true
	}
	rsp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}
	io.Copy(io.Discard, rsp.Body)
	rsp.Body.Close()
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get 200 response code: %d", rsp.StatusCode)
	} else {
		t.Logf("response code: %d", rsp.StatusCode)
	}
	return false
}

func checkStatusCode(t *testing.T, resCH chan int, N int) {
	t.Helper()
	sometimes := rate.Sometimes{First: 3, Interval: time.Second}
	count := 0
	for code := range resCH {
		if code != 200 {
			sometimes.Do(func() { t.Errorf("request wrong status: %d", code) })
		}
		count++
	}
	if count != N {
		// expected
		t.Logf("Failed to get the same amount of responses want %d got %d", N, count)
	}
}

func logFifoMetrics(t *testing.T, mockMetrics *metricstest.MockMetrics) {
	t.Helper()
	for _, route := range []string{"main", "shadow"} {
		for _, kfmt := range []string{"fifo.%s.active", "fifo.%s.queued", "fifo.%s.error.full", "fifo.%s.error.other", "fifo.%s.error.timeout"} {
			k := fmt.Sprintf(kfmt, route)
			if v, ok := mockMetrics.Gauge(k); ok {
				t.Logf("metric %q: %v", k, v)
			} else {
				t.Logf("metric not found: %q", k)
			}
		}
	}
}

func sendRequests(t *testing.T, N int, p *proxytest.TestProxy, client *skpnet.Client, resCH chan int) {
	t.Helper()
	wg := sync.WaitGroup{}
	sometimes := rate.Sometimes{First: 3, Interval: time.Second}

	for i := range N {
		time.Sleep(100 * time.Microsecond)
		wg.Add(1)
		go func(ch chan<- int) {
			defer wg.Done()
			bodyData := strings.Repeat("A", 1023) + "\n"
			buf := bytes.NewBufferString(bodyData)
			sr := skpiotest.NewSlowReader(buf, 1*time.Microsecond)
			req, err := http.NewRequest("PUT", p.URL+"/"+strconv.Itoa(i), sr)
			if err != nil {
				t.Logf("Failed to create request: %v", err)
				return
			}

			rsp, err := client.Do(req)
			if err != nil {
				t.Logf("Failed to get response: %v", err)
				return
			}
			if rsp.StatusCode != 200 {
				sometimes.Do(func() { t.Logf("response: %s", rsp.Status) })
			}
			io.Copy(io.Discard, rsp.Body)
			ch <- rsp.StatusCode
		}(resCH)
	}
	wg.Wait()
}
