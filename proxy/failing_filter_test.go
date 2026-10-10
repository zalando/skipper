package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/shedder"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func TestResponseFilterOnProxyError(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(6)

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer backend.Close()

	var routes = fmt.Sprintf(`
		main: * -> admissionControl("mygroup", "active", "24h", 5, 0, 0.99, 0.9, 1.0) -> "%s";
	`, backend.URL)
	r := eskip.MustParse(routes)

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)
	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				acSpec.PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				acSpec.PostProcessor(),
			},
		},
		r...)
	defer proxy.Close()

	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Error(err)
		return
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to check status: %v", err)
	}
	rsp.Body.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusCounts := make(map[int]int)
	total := uint64(250)
	requestsPerWindow := 50

	for i := 0; i < int(total); i++ {
		if i > 0 && i%requestsPerWindow == 0 {
			acSpec.StepWindows()
		}

		ctx := context.Background()
		var cancel context.CancelFunc
		if (int64(i)+1)%timeoutN == 0 {
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}

		r, err := http.NewRequestWithContext(ctx, "GET", proxy.URL, nil)
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
}

func TestAdmissionControlBeforeLoopback(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(6)

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer backend.Close()

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	routes := fmt.Sprintf(`
		main: * -> admissionControl("mygroup", "active", "24h", 5, 0, 0.99, 0.9, 1.0) -> setPath("/foo") -> <loopback>; r: Path("/foo") -> "%s";
	`, backend.URL)

	r := eskip.MustParse(routes)

	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				acSpec.PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				acSpec.PostProcessor(),
			},
		},
		r...)
	defer proxy.Close()

	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Error(err)
		return
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to check status: %v", err)
	}
	rsp.Body.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusCounts := make(map[int]int)
	total := uint64(250)
	requestsPerWindow := 50

	for i := 0; i < int(total); i++ {
		if i > 0 && i%requestsPerWindow == 0 {
			acSpec.StepWindows()
		}

		ctx := context.Background()
		var cancel context.CancelFunc
		if (int64(i)+1)%timeoutN == 0 {
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}

		r, err := http.NewRequestWithContext(ctx, "GET", proxy.URL, nil)
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
}

func TestAdmissionControlInLoopback(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(6)

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer backend.Close()

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	acSpec := spec.(*shedder.AdmissionControlSpec)
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	routes := fmt.Sprintf(`
		main: * -> setPath("/foo") -> <loopback>; r: Path("/foo") -> admissionControl("mygroup", "active", "24h", 5, 0, 0.99, 0.9, 1.0) -> "%s";
	`, backend.URL)

	r := eskip.MustParse(routes)

	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				acSpec.PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				acSpec.PostProcessor(),
			},
		},
		r...)
	defer proxy.Close()

	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Error(err)
		return
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to check status: %v", err)
	}
	rsp.Body.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusCounts := make(map[int]int)
	total := uint64(250)
	requestsPerWindow := 50

	for i := 0; i < int(total); i++ {
		if i > 0 && i%requestsPerWindow == 0 {
			acSpec.StepWindows()
		}

		ctx := context.Background()
		var cancel context.CancelFunc
		if (int64(i)+1)%timeoutN == 0 {
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}

		r, err := http.NewRequestWithContext(ctx, "GET", proxy.URL, nil)
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
}
