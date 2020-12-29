package proxy_test

import (
	"io/ioutil"
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
		MaxHits:       10,
		TimeWindow:    timeWindow,
		CleanInterval: 2 * timeWindow,
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		RateLimiters:         ratelimit.NewRegistry(ratelimitSettings),
	}, r...)
	defer p.Close()

	requestAndExpect(t, p.URL, 10, http.StatusNotFound, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 10, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 10, http.StatusNotFound, nil)
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

	requestAndExpect(t, p.URL, 10, http.StatusOK, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 10, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 10, http.StatusOK, nil)
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

	requestAndExpect(t, p.URL, 10, http.StatusOK, nil)

	expectHeader := strconv.Itoa(ratelimitSettings.MaxHits * int(time.Hour/ratelimitSettings.TimeWindow))

	requestAndExpect(t, p.URL, 10, http.StatusTooManyRequests, http.Header{ratelimit.Header: []string{expectHeader}})

	time.Sleep(timeWindow)

	requestAndExpect(t, p.URL, 10, http.StatusOK, nil)
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

	requestAndExpect(t, p.URL, 1, http.StatusOK, nil)

	requestAndExpect(t, p.URL, 1, http.StatusTooManyRequests, http.Header{ratelimit.RetryAfterHeader: []string{strconv.Itoa(limit)}})
}

func requestAndExpect(t *testing.T, url string, repeat int, expectCode int, expectHeader http.Header) {
	for i := 0; i < repeat; i++ {
		code, header, err := doRequest(url)
		if err != nil {
			t.Fatal(err)
		}
		if code != expectCode {
			t.Fatalf("unexpected code, expected %d, got %d", expectCode, code)
		}

		for name := range expectHeader {
			expected := expectHeader.Get(name)
			got := header.Get(name)
			if got != expected {
				t.Fatalf("unexpected header %s, expected %s, got %s", name, expected, got)
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

	_, err = ioutil.ReadAll(rsp.Body)
	if err != nil {
		return -1, nil, err
	}
	return rsp.StatusCode, rsp.Header, nil
}
