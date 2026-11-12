package proxy_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

func BenchmarkHedgeEnabledNoAdditionalAttempt(b *testing.B) {
	var attempts atomicCounter
	benchmarkHedgeRequests(b, 1, func(req *http.Request, _ http.RoundTripper) (*http.Response, error) {
		if req.Header.Get("X-Hedge-Seed") == "" {
			attempts.value.Add(1)
		}
		return benchmarkHedgeResponse(req), nil
	}, &attempts)
}

func BenchmarkHedgeSecondAttempt(b *testing.B) {
	var attempts atomicCounter
	benchmarkHedgeRequests(b, 2, func(req *http.Request, _ http.RoundTripper) (*http.Response, error) {
		if req.Header.Get("X-Hedge-Seed") != "" {
			return benchmarkHedgeResponse(req), nil
		}
		attempts.value.Add(1)
		timer := time.NewTimer(5 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return benchmarkHedgeResponse(req), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}, &attempts)
}

type atomicCounter struct {
	value atomic.Int64
}

func benchmarkHedgeRequests(b *testing.B, endpointCount int, roundTrip func(*http.Request, http.RoundTripper) (*http.Response, error), attempts ...*atomicCounter) {
	b.Helper()
	registry := builtin.MakeRegistry()
	registry.Register(&hedgeStateSeedSpec{sampleCount: 100, eligibleCount: 20, sampleLatency: time.Millisecond})
	endpoints := make([]string, endpointCount)
	for i := range endpoints {
		endpoints[i] = strconv.Quote(fmt.Sprintf("http://backend-%d.example:8080", i))
	}
	route := eskip.MustParse(fmt.Sprintf("r: * -> hedge() -> hedgeStateSeed() -> <roundRobin, %s>;", strings.Join(endpoints, ", ")))
	testProxy := proxytest.WithParams(registry, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		CustomHttpRoundTripperWrap: func(http.RoundTripper) http.RoundTripper {
			return scriptedHedgeRoundTripper(func(req *http.Request) (*http.Response, error) {
				return roundTrip(req, http.DefaultTransport)
			})
		},
	}, route[0])
	b.Cleanup(func() { _ = testProxy.Close() })
	client := testProxy.Client()
	request, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		b.Fatal(err)
	}
	request.Header.Set("X-Hedge-Seed", "true")
	response, err := client.Do(request)
	if err != nil {
		b.Fatal(err)
	}
	_ = response.Body.Close()
	request.Header.Del("X-Hedge-Seed")
	if len(attempts) != 0 {
		attempts[0].value.Store(0)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response, err := client.Do(request)
		if err != nil {
			b.Fatal(err)
		}
		_ = response.Body.Close()
	}
	b.StopTimer()
	if len(attempts) != 0 {
		b.ReportMetric(float64(attempts[0].value.Load()-int64(b.N))/float64(b.N), "extra-attempts/op")
	}
}

func benchmarkHedgeResponse(req *http.Request) *http.Response {
	return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: req}
}
