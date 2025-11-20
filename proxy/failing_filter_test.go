package proxy_test

import (
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
	"github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func TestResponseFilterOnProxyError(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(5)
	timeout := 100 * time.Millisecond

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)

		v := atomic.LoadInt64(&counter)
		if v%timeoutN == 0 {
			time.Sleep(timeout) // 499 no response
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
                main: * -> admissionControl("mygroup", "active", "1s", 5, 0, 0.99, 0.9, 1.0) -> "%s";
        `, backend.URL)
	r := eskip.MustParse(routes)

	fr := make(filters.Registry)
	spec := shedder.NewAdmissionControl(shedder.Options{})
	fr.Register(spec)
	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				spec.(*shedder.AdmissionControlSpec).PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				spec.(*shedder.AdmissionControlSpec).PostProcessor(),
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

	// vegeta test
	rate := 50
	sec := 5
	d := time.Duration(sec) * time.Second
	total := uint64(rate * sec)
	va := httptest.NewVegetaAttacker(proxy.URL, rate, time.Second, timeout)
	va.Attack(io.Discard, d, "mytest")
	t.Logf("Success [0..1]: %0.2f", va.Success())

	if successRate := va.Success(); successRate < 0.5 || successRate > 0.9 {
		t.Errorf("Test should have a success rate between %0.2f < %0.2f < %0.2f", 0.5, successRate, 0.9)
	}
	reqCount := va.TotalRequests()
	if reqCount < total {
		t.Errorf("Test should run %d requests got: %d", total, reqCount)
	}
	countOK, ok := va.CountStatus(http.StatusOK)
	if countOK == 0 {
		t.Errorf("Some requests should have passed: %d %v", countOK, ok)
	}

	countErr, ok := va.CountStatus(http.StatusInternalServerError)
	if !ok || countErr > countOK {
		t.Errorf("count status 500 should be more than 0 but lower than OKs: %d > %d: %v", countErr, countOK, ok)
	}

	countBlock, ok := va.CountStatus(http.StatusServiceUnavailable)
	if !ok || countBlock > countOK {
		t.Errorf("count status 503 should be more than 0 but lower than OKs: %d > %d: %v", countBlock, countOK, ok)
	}

	countClientTimeout, ok := va.CountStatus(0)
	if !ok || countClientTimeout > countOK {
		t.Errorf("count status 0 should be more than 0 but lower than OKs: %d > %d: %v", countClientTimeout, countOK, ok)
	}

	t.Logf("total: %d, ok: %d, err: %d, blocked: %d, timeout: %d", reqCount, countOK, countErr, countBlock, countClientTimeout)
}

func TestAdmissionControlBeforeLoopback(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(5)
	timeout := 100 * time.Millisecond

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)
		v := atomic.LoadInt64(&counter)
		if v%timeoutN == 0 {
			time.Sleep(timeout) // 499 no response
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
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	routes := fmt.Sprintf(`
                main: * -> admissionControl("mygroup", "active", "1s", 5, 0, 0.99, 0.9, 1.0) -> setPath("/foo") -> <loopback>; r: Path("/foo") -> "%s";
        `, backend.URL)

	r := eskip.MustParse(routes)

	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				spec.(*shedder.AdmissionControlSpec).PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				spec.(*shedder.AdmissionControlSpec).PostProcessor(),
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

	// vegeta test
	rate := 50
	sec := 5
	d := time.Duration(sec) * time.Second
	total := uint64(rate * sec)
	va := httptest.NewVegetaAttacker(proxy.URL, rate, time.Second, timeout)
	va.Attack(io.Discard, d, "mytest")
	t.Logf("Success [0..1]: %0.2f", va.Success())

	if successRate := va.Success(); successRate < 0.5 || successRate > 0.9 {
		t.Errorf("Test should have a success rate between %0.2f < %0.2f < %0.2f", 0.5, successRate, 0.9)
	}
	reqCount := va.TotalRequests()
	if reqCount < total {
		t.Errorf("Test should run %d requests got: %d", total, reqCount)
	}
	countOK, ok := va.CountStatus(http.StatusOK)
	if countOK == 0 {
		t.Errorf("Some requests should have passed: %d %v", countOK, ok)
	}

	countErr, ok := va.CountStatus(http.StatusInternalServerError)
	if !ok || countErr > countOK {
		t.Errorf("count status 500 should be more than 0 but lower than OKs: %d > %d: %v", countErr, countOK, ok)
	}

	countBlock, ok := va.CountStatus(http.StatusServiceUnavailable)
	if !ok || countBlock > countOK {
		t.Errorf("count status 503 should be more than 0 but lower than OKs: %d > %d: %v", countBlock, countOK, ok)
	}

	countClientTimeout, ok := va.CountStatus(0)
	if !ok || countClientTimeout > countOK {
		t.Errorf("count status 0 should be more than 0 but lower than OKs: %d > %d: %v", countClientTimeout, countOK, ok)
	}

	t.Logf("total: %d, ok: %d, err: %d, blocked: %d, timeout: %d", reqCount, countOK, countErr, countBlock, countClientTimeout)
}

func TestAdmissionControlInLoopback(t *testing.T) {
	t.Parallel()
	counter := int64(1)
	serverErrN := int64(37)
	timeoutN := int64(5)
	timeout := 100 * time.Millisecond

	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)
		v := atomic.LoadInt64(&counter)
		if v%timeoutN == 0 {
			time.Sleep(timeout) // 499 no response
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
	fr.Register(spec)
	fr.Register(builtin.NewSetPath())

	routes := fmt.Sprintf(`
                main: * -> setPath("/foo") -> <loopback>; r: Path("/foo") -> admissionControl("mygroup", "active", "1s", 5, 0, 0.99, 0.9, 1.0) -> "%s";
        `, backend.URL)

	r := eskip.MustParse(routes)

	proxy := proxytest.WithParamsAndRoutingOptions(fr,
		proxy.Params{
			AccessLogDisabled: true,
		},
		routing.Options{
			PreProcessors: []routing.PreProcessor{
				spec.(*shedder.AdmissionControlSpec).PreProcessor(),
			},
			PostProcessors: []routing.PostProcessor{
				spec.(*shedder.AdmissionControlSpec).PostProcessor(),
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

	// vegeta test
	rate := 50
	sec := 5
	d := time.Duration(sec) * time.Second
	total := uint64(rate * sec)
	va := httptest.NewVegetaAttacker(proxy.URL, rate, time.Second, timeout)
	va.Attack(io.Discard, d, "mytest")
	t.Logf("Success [0..1]: %0.2f", va.Success())

	if successRate := va.Success(); successRate < 0.5 || successRate > 0.9 {
		t.Errorf("Test should have a success rate between %0.2f < %0.2f < %0.2f", 0.5, successRate, 0.9)
	}
	reqCount := va.TotalRequests()
	if reqCount < total {
		t.Errorf("Test should run %d requests got: %d", total, reqCount)
	}
	countOK, ok := va.CountStatus(http.StatusOK)
	if countOK == 0 {
		t.Errorf("Some requests should have passed: %d %v", countOK, ok)
	}

	countErr, ok := va.CountStatus(http.StatusInternalServerError)
	if !ok || countErr > countOK {
		t.Errorf("count status 500 should be more than 0 but lower than OKs: %d > %d: %v", countErr, countOK, ok)
	}

	countBlock, ok := va.CountStatus(http.StatusServiceUnavailable)
	if !ok || countBlock > countOK {
		t.Errorf("count status 503 should be more than 0 but lower than OKs: %d > %d: %v", countBlock, countOK, ok)
	}

	countClientTimeout, ok := va.CountStatus(0)
	if !ok || countClientTimeout > countOK {
		t.Errorf("count status 0 should be more than 0 but lower than OKs: %d > %d: %v", countClientTimeout, countOK, ok)
	}

	t.Logf("total: %d, ok: %d, err: %d, blocked: %d, timeout: %d", reqCount, countOK, countErr, countBlock, countClientTimeout)
}
