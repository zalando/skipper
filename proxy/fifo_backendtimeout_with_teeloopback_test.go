package proxy_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
	"golang.org/x/time/rate"
)

const sourcePollTimeout time.Duration = 6 * time.Millisecond

type slowReader struct {
	r io.Reader
	d time.Duration
}

// NewSlowReader creates a new slowReader wrapping the given io.Reader
// with a 1 millisecond delay after each byte.
func NewSlowReader(r io.Reader, d time.Duration) *slowReader {
	return &slowReader{
		r: r,
		d: d,
	}
}

// Read implements the io.Reader interface.
// It reads one byte at a time from the underlying reader,
// sleeps for the specified Delay, and populates the provided buffer 'p'.
func (sr *slowReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for i := range len(p) {
		oneByte := make([]byte, 1)
		bytesRead, err := sr.r.Read(oneByte)

		if err != nil {
			if n > 0 && err == io.EOF {
				return n, nil
			}
			return n, err
		}

		// single byte read
		if bytesRead == 1 {
			p[i] = oneByte[0]
			n++
			time.Sleep(sr.d)
		} else if n > 0 {
			return n, nil
		} else {
			return 0, nil
		}
	}

	return n, nil
}

func TestBackendTimeoutWithShadow(t *testing.T) {
	proxyLog := proxy.NewTestLog()
	defer proxyLog.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer backend.Close()
	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sr := NewSlowReader(r.Body, 2*time.Millisecond)
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

	doc := fmt.Sprintf(`
main: Path("/") -> fifo(5000, 20, "1s") -> teeLoopback("tag") -> "%s";
shadow: Path("/") && Tee("tag") -> fifo(5, 40, "150ms") -> backendTimeout("20ms") -> "%s";`,
		backend.URL, slowBackend.URL)

	fr := builtin.MakeRegistry()
	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Fatalf("Failed to create dataclient: %v", err)
	}
	defer dc.Close()
	mockMetrics := &metricstest.MockMetrics{}
	defer mockMetrics.Close()
	epRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	schedulerRegistry := scheduler.RegistryWith(scheduler.Options{
		Metrics:                mockMetrics,
		MetricsUpdateTimeout:   100 * time.Millisecond,
		EnableRouteFIFOMetrics: true,
		EnableRouteLIFOMetrics: true,
	})
	defer schedulerRegistry.Close()

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
	defer p.Close()

	N := 500 //500000
	wg := sync.WaitGroup{}
	sometimes := rate.Sometimes{First: 3, Interval: time.Second}
	resCH := make(chan int, N)
	client := skpnet.NewClient(skpnet.Options{
		Timeout:               80 * time.Millisecond,
		ResponseHeaderTimeout: 120 * time.Millisecond,
		IdleConnTimeout:       time.Second,
		Log:                   p.Log,
		MaxIdleConnsPerHost:   100,
		MaxIdleConns:          100,
	})
	defer client.Close()
	defer client.CloseIdleConnections()

	for range N {
		time.Sleep(100 * time.Microsecond)
		wg.Add(1)
		go func(ch chan<- int) {
			defer wg.Done()
			bodyData := strings.Repeat("A", 1023) + "\n"
			buf := bytes.NewBufferString(bodyData)
			sr := NewSlowReader(buf, 1*time.Microsecond)
			req, err := http.NewRequest("PUT", p.URL, sr)
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

	close(resCH)
	sometimes = rate.Sometimes{First: 3, Interval: time.Second}

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

	// check that we can hit the main route now again correctly
	bodyData := strings.Repeat("A", 1023) + "\n"
	buf := bytes.NewBufferString(bodyData)
	sr := NewSlowReader(buf, 1*time.Microsecond)
	req, err := http.NewRequest("PUT", p.URL, sr)
	if err != nil {
		t.Logf("Failed to create request: %v", err)
		return
	}
	rsp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}
	io.Copy(io.Discard, rsp.Body)
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get 200 response code: %d", rsp.StatusCode)
	} else {
		t.Logf("response code: %d", rsp.StatusCode)
	}

	if err := proxyLog.WaitFor("failed to execute loopback request: dialing failed false: context deadline exceeded", time.Second); err != nil {
		t.Fatalf("Failed to get expected error: %v", err)
	} else {
		t.Log("Found error log")
	}
}
