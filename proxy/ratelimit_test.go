package proxy_test

import (
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/ratelimit"
)

func TestWithoutRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := &eskip.Route{Backend: backend.URL}
	p := proxytest.New(fr, r)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		if rsp.StatusCode == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}

	for i := 0; i < 100; i++ {
		d0 := doc(128)
		request(d0)
	}

}

func TestCheckDisableRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters: ratelimit.NewRegistry(ratelimit.Settings{
			Type:          ratelimit.DisableRatelimit,
			MaxHits:       10,
			TimeWindow:    1 * time.Second,
			CleanInterval: 2 * time.Second,
		}),
	}, r...)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) int {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return -1
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return -1
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		return rsp.StatusCode
	}

	for i := 0; i < 100; i++ {
		d0 := doc(128)
		if request(d0) == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}

}

func TestCheckLocalRateLimitForShuntRoutes(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL, BackendType: eskip.ShuntBackend}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:          ratelimit.LocalRatelimit,
		MaxHits:       10,
		TimeWindow:    timeWindow,
		CleanInterval: 2 * timeWindow,
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         ratelimit.NewRegistry(ratelimitSettings),
	}, r...)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) (int, http.Header) {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return -1, nil
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return -1, nil
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		return rsp.StatusCode, rsp.Header
	}

	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, header := request(d0)
		if code != http.StatusTooManyRequests {
			t.Fatal("should be ratelimitted")
		}
		v := header.Get(ratelimit.Header)
		if v == "" {
			t.Fatalf("should set ratelimit header %s", ratelimit.Header)
		}
		expected := ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow)
		i, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("failed to convert string number %s to number: %v", v, err)
		}
		if i != expected {
			t.Fatalf("should calculateratelimit header correctly: %d expected: %d", i, expected)
		}
	}
	time.Sleep(timeWindow)
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
}

func TestCheckLocalRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:          ratelimit.LocalRatelimit,
		MaxHits:       10,
		TimeWindow:    timeWindow,
		CleanInterval: 2 * timeWindow,
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         ratelimit.NewRegistry(ratelimitSettings),
	}, r...)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) (int, http.Header) {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return -1, nil
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return -1, nil
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		return rsp.StatusCode, rsp.Header
	}

	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, header := request(d0)
		if code != http.StatusTooManyRequests {
			t.Fatal("should be ratelimitted")
		}
		v := header.Get(ratelimit.Header)
		if v == "" {
			t.Fatalf("should set ratelimit header %s", ratelimit.Header)
		}
		expected := ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow)
		i, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("failed to convert string number %s to number: %v", v, err)
		}
		if i != expected {
			t.Fatalf("should calculateratelimit header correctly: %d expected: %d", i, expected)
		}
	}
	time.Sleep(timeWindow)
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
}

func TestCheckServiceRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:       ratelimit.ServiceRatelimit,
		MaxHits:    10,
		TimeWindow: timeWindow,
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         ratelimit.NewRegistry(ratelimitSettings),
	}, r...)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) (int, http.Header) {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return -1, nil
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return -1, nil
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		return rsp.StatusCode, rsp.Header
	}

	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, header := request(d0)
		if code != http.StatusTooManyRequests {
			t.Fatal("should be ratelimitted")
		}
		v := header.Get(ratelimit.Header)
		if v == "" {
			t.Fatalf("should set ratelimit header %s", ratelimit.Header)
		}
		expected := ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow)
		i, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("failed to convert string number %s to number: %v", v, err)
		}
		if i != expected {
			t.Fatalf("should calculate ratelimit header correctly: %d expected: %d", i, expected)
		}
	}
	time.Sleep(timeWindow)
	for i := 0; i < 10; i++ {
		d0 := doc(128)
		code, _ := request(d0)
		if code == http.StatusTooManyRequests {
			t.Fatal("should not be ratelimitted")
		}
	}
}

func TestRetryAfterHeader(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	const limit = 5
	const timeWindow = time.Duration(limit) * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:       ratelimit.ServiceRatelimit,
		MaxHits:    1,
		TimeWindow: timeWindow,
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         ratelimit.NewRegistry(ratelimitSettings),
	}, r...)
	defer p.Close()

	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	request := func(doc []byte) (int, http.Header) {
		req, err := http.NewRequest("GET", p.URL+"/", nil)
		if err != nil {
			t.Fatal("foo", "failed to create request", err)
			return -1, nil
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal("Do req", "failed to make request", err)
			return -1, nil
		}

		defer rsp.Body.Close()
		_, err = ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal("read", "failed to read response", err)
		}

		return rsp.StatusCode, rsp.Header
	}

	d0 := doc(128)
	code, _ := request(d0)
	if code == http.StatusTooManyRequests {
		t.Fatal("should not be ratelimitted")
	}

	d1 := doc(128)
	code, header := request(d1)
	if code != http.StatusTooManyRequests {
		t.Fatal("should be ratelimitted")
	}
	v := header.Get(ratelimit.RetryAfterHeader)
	if v == "" {
		t.Fatalf("should set retry header %s", ratelimit.RetryAfterHeader)
	}
	expected := limit
	i, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("failed to convert string number %s to number: %v", v, err)
	}
	if i != expected {
		t.Fatalf("should calculate ratelimit header correctly: %d expected: %d", i, expected)
	}
}
