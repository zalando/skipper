package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/shedder"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func runAdmissionControlTest(t *testing.T, routeFormat string, fr filters.Registry, rOpts routing.Options) {
	synctest.Test(t, func(t *testing.T) {
		counter := int64(1)
		serverErrN := int64(37)
		timeoutN := int64(6)

		backendListener := fakeNetListen()
		defer backendListener.Close()

		backend := stdlibhttptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)

			v := atomic.LoadInt64(&counter)
			if v%timeoutN == 0 {
				w.WriteHeader(499)
				return
			} else if v%serverErrN == 0 {
				w.WriteHeader(500)
				w.Write([]byte("FAIL"))
			} else {
				w.WriteHeader(200)
				w.Write([]byte("OK"))
			}
		}))
		backend.Listener.Close()
		backend.Listener = backendListener
		backend.Start()
		defer backend.Close()

		routes := fmt.Sprintf(routeFormat, backend.URL)
		r := eskip.MustParse(routes)

		pParams := proxy.Params{
			AccessLogDisabled: true,
			CustomHttpRoundTripperWrap: func(rt http.RoundTripper) http.RoundTripper {
				tr := rt.(*http.Transport)
				tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return backendListener.connect(), nil
				}
				return tr
			},
		}

		p := proxytest.WithParamsAndRoutingOptionsAndWaitUnstarted(fr, pParams, rOpts, 0, r...)

		proxyListener := fakeNetListen()
		defer proxyListener.Close()

		p.SetListener(proxyListener)
		p.Start()
		defer p.Close()

		client := p.Client()
		tr := client.Transport.(*http.Transport)
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return proxyListener.connect(), nil
		}

		req, err := http.NewRequest("GET", p.URL, nil)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to check status: %v", err)
		}
		rsp.Body.Close()

		statusCounts := make(map[int]int)
		total := uint64(250)
		requestsPerWindow := 50

		for i := 0; i < int(total); i++ {
			if i > 0 && i%requestsPerWindow == 0 {
				time.Sleep(20 * time.Millisecond)
			}

			ctx := context.Background()
			var cancel context.CancelFunc
			if (int64(i)+1)%timeoutN == 0 {
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			r, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			resp, err := client.Do(r)
			if err != nil {
				statusCounts[0]++
				continue
			}
			statusCounts[resp.StatusCode]++
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		synctest.Wait()

		reqCount := total
		countOK := statusCounts[http.StatusOK]
		countErr := statusCounts[http.StatusInternalServerError]
		countBlock := statusCounts[http.StatusServiceUnavailable]
		countClientTimeout := statusCounts[0]

		successRate := float64(countOK) / float64(reqCount)
		t.Logf("Success [0..1]: %0.2f", successRate)

		if successRate < 0.5 || successRate > 0.9 {
			t.Errorf("Test should have a success rate between %0.2f < %0.2f < %0.2f", 0.5, successRate, 0.9)
		}
		if countOK == 0 {
			t.Errorf("Some requests should have passed: %d", countOK)
		}

		if countErr == 0 || countErr > countOK {
			t.Errorf("count status 500 should be more than 0 but lower than OKs: %d > %d", countErr, countOK)
		}

		if countBlock == 0 || countBlock > countOK {
			t.Errorf("count status 503 should be more than 0 but lower than OKs: %d > %d", countBlock, countOK)
		}

		if countClientTimeout == 0 || countClientTimeout > countOK {
			t.Errorf("count status 0 should be more than 0 but lower than OKs: %d > %d", countClientTimeout, countOK)
		}

		t.Logf("total: %d, ok: %d, err: %d, blocked: %d, timeout: %d", reqCount, countOK, countErr, countBlock, countClientTimeout)
	})
}

func TestResponseFilterOnProxyError(t *testing.T) {
	t.Parallel()

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)

	rOpts := routing.Options{
		PreProcessors: []routing.PreProcessor{
			acSpec.PreProcessor(),
		},
		PostProcessors: []routing.PostProcessor{
			acSpec.PostProcessor(),
		},
	}

	routeFormat := `
		main: * -> admissionControl("mygroup", "active", "20ms", 5, 0, 0.99, 0.9, 1.0) -> "%s";
	`
	runAdmissionControlTest(t, routeFormat, fr, rOpts)
}

func TestAdmissionControlBeforeLoopback(t *testing.T) {
	t.Parallel()

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	rOpts := routing.Options{
		PreProcessors: []routing.PreProcessor{
			acSpec.PreProcessor(),
		},
		PostProcessors: []routing.PostProcessor{
			acSpec.PostProcessor(),
		},
	}

	routeFormat := `
		main: * -> admissionControl("mygroup", "active", "20ms", 5, 0, 0.99, 0.9, 1.0) -> setPath("/foo") -> <loopback>; r: Path("/foo") -> "%s";
	`
	runAdmissionControlTest(t, routeFormat, fr, rOpts)
}

func TestAdmissionControlInLoopback(t *testing.T) {
	t.Parallel()

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	rOpts := routing.Options{
		PreProcessors: []routing.PreProcessor{
			acSpec.PreProcessor(),
		},
		PostProcessors: []routing.PostProcessor{
			acSpec.PostProcessor(),
		},
	}

	routeFormat := `
		main: * -> setPath("/foo") -> <loopback>; r: Path("/foo") -> admissionControl("mygroup", "active", "20ms", 5, 0, 0.99, 0.9, 1.0) -> "%s";
	`
	runAdmissionControlTest(t, routeFormat, fr, rOpts)
}
