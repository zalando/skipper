package proxy_test

import (
	"io"
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

	requestAndExpect(t, p.URL, 100, http.StatusOK, nil)
}

func TestCheckDisableRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	reg := ratelimit.NewRegistry(ratelimit.Settings{
		Type:          ratelimit.DisableRatelimit,
		MaxHits:       10,
		TimeWindow:    1 * time.Second,
		CleanInterval: 2 * time.Second,
	})
	defer reg.Close()

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         reg,
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 100, http.StatusOK, nil)
}

func TestCheckLocalRateLimitForShuntRoutes(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL, BackendType: eskip.ShuntBackend}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:          ratelimit.LocalRatelimit,
		MaxHits:       3,
		TimeWindow:    timeWindow,
		CleanInterval: 2 * timeWindow,
	}
	reg := ratelimit.NewRegistry(ratelimitSettings)
	defer reg.Close()

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         reg,
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 3, http.StatusNotFound, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 1, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 3, http.StatusNotFound, nil)
}

func TestCheckLocalRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:          ratelimit.LocalRatelimit,
		MaxHits:       3,
		TimeWindow:    timeWindow,
		CleanInterval: 2 * timeWindow,
	}
	reg := ratelimit.NewRegistry(ratelimitSettings)
	defer reg.Close()

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         reg,
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 3, http.StatusOK, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 1, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 3, http.StatusOK, nil)
}

func TestCheckServiceRateLimit(t *testing.T) {
	fr := builtin.MakeRegistry()
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	r := []*eskip.Route{{Backend: backend.URL}}
	timeWindow := 1 * time.Second
	ratelimitSettings := ratelimit.Settings{
		Type:       ratelimit.ServiceRatelimit,
		MaxHits:    3,
		TimeWindow: timeWindow,
	}
	reg := ratelimit.NewRegistry(ratelimitSettings)
	defer reg.Close()

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         reg,
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 3, http.StatusOK, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 1, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 3, http.StatusOK, nil)
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
	reg := ratelimit.NewRegistry(ratelimitSettings)
	defer reg.Close()

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         reg,
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 1, http.StatusOK, nil)

	requestAndExpect(t, p.URL, 1, http.StatusTooManyRequests, http.Header{ratelimit.RetryAfterHeader: []string{strconv.Itoa(limit)}})
}

func requestAndExpect(t *testing.T, url string, repeat int, expectCode int, expectHeader http.Header) {
	for i := 1; i <= repeat; i++ {
		code, header, err := doRequest(url)
		if err != nil {
			t.Fatalf("request %d/%d: %v", i, repeat, err)
		}
		if code != expectCode {
			t.Fatalf("request %d/%d: unexpected code, expected %d, got %d", i, repeat, expectCode, code)
		}

		for name := range expectHeader {
			expected := expectHeader.Get(name)
			got := header.Get(name)
			if got != expected {
				t.Fatalf("request %d/%d: unexpected header %s, expected %s, got %s", i, repeat, name, expected, got)
			}
		}
	}
}

func doRequest(url string) (int, http.Header, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return -1, nil, err
	}
	req.Close = true

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		return -1, nil, err
	}
	defer rsp.Body.Close()

	_, err = io.ReadAll(rsp.Body)
	if err != nil {
		return -1, nil, err
	}
	return rsp.StatusCode, rsp.Header, nil
}
