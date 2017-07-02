package proxy_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

type breakerTestContext struct {
	t       *testing.T
	proxy   *proxytest.TestProxy
	backend *failingBackend
}

type scenarioStep func(*breakerTestContext)

type breakerScenario struct {
	title    string
	settings []circuit.BreakerSettings
	steps    []scenarioStep
}

const (
	testConsecutiveFailureCount = 5
	testBreakerTimeout          = 3 * time.Millisecond
	testHalfOpenRequests        = 3
	testRateWindow              = 10
	testRateFailures            = 4
)

func newBreakerProxy(backendURL string, settings []circuit.BreakerSettings) *proxytest.TestProxy {
	r, err := eskip.Parse(fmt.Sprintf(`* -> "%s"`, backendURL))
	if err != nil {
		panic(err)
	}

	params := proxy.Params{
		CloseIdleConnsPeriod: -1,
	}

	if len(settings) > 0 {
		var breakerOptions circuit.Options
		for _, si := range settings {
			if si.Host == "" {
				breakerOptions.Defaults = si
			}

			breakerOptions.HostSettings = append(breakerOptions.HostSettings, si)
		}

		params.CircuitBreakers = circuit.NewRegistry(breakerOptions)
	}

	fr := builtin.MakeRegistry()
	return proxytest.WithParams(fr, params, r...)
}

func testBreaker(t *testing.T, s breakerScenario) {
	b := newFailingBackend()
	defer b.close()

	p := newBreakerProxy(b.url, s.settings)
	defer p.Close()

	steps := s.steps
	c := &breakerTestContext{
		t:       t,
		proxy:   p,
		backend: b,
	}

	for !t.Failed() && len(steps) > 0 {
		steps[0](c)
		steps = steps[1:]
	}
}

func setBackendSucceed(c *breakerTestContext) {
	c.backend.succeed()
}

func setBackendFail(c *breakerTestContext) {
	c.backend.fail()
}

func setBackendDown(c *breakerTestContext) {
	c.backend.down()
}

func proxyRequest(c *breakerTestContext) (*http.Response, error) {
	rsp, err := http.Get(c.proxy.URL)
	if err != nil {
		return nil, err
	}

	rsp.Body.Close()
	return rsp, nil
}

func checkStatus(c *breakerTestContext, rsp *http.Response, expected int) {
	if rsp.StatusCode != expected {
		c.t.Errorf(
			"wrong response status: %d instead of %d",
			rsp.StatusCode,
			expected,
		)
	}
}

func request(expectedStatus int) scenarioStep {
	return func(c *breakerTestContext) {
		rsp, err := proxyRequest(c)
		if err != nil {
			c.t.Error(err)
			return
		}

		checkStatus(c, rsp, expectedStatus)
	}
}

func requestOpen(c *breakerTestContext) {
	rsp, err := proxyRequest(c)
	if err != nil {
		c.t.Error(err)
		return
	}

	checkStatus(c, rsp, 503)
	if rsp.Header.Get("X-Circuit-Open") != "true" {
		c.t.Error("failed to set circuit open header")
	}
}

func checkBackendCounter(count int) scenarioStep {
	return func(c *breakerTestContext) {
		if c.backend.counter() != count {
			c.t.Error("invalid number of requests on the backend")
		}

		c.backend.resetCounter()
	}
}

// as in scenario step N times
func times(n int, s scenarioStep) scenarioStep {
	return func(c *breakerTestContext) {
		for !c.t.Failed() && n > 0 {
			s(c)
			n--
		}
	}
}

func wait(d time.Duration) scenarioStep {
	return func(*breakerTestContext) {
		time.Sleep(d)
	}
}

func trace(msg string) scenarioStep {
	return func(*breakerTestContext) {
		println(msg)
	}
}

func TestBreakerConsecutive(t *testing.T) {
	for _, s := range []breakerScenario{{
		title: "no breaker",
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendFail,
			times(testConsecutiveFailureCount, request(500)),
			checkBackendCounter(testConsecutiveFailureCount),
			request(500),
			checkBackendCounter(1),
		},
	}, {
		title: "open",
		settings: []circuit.BreakerSettings{{
			Type:     circuit.ConsecutiveFailures,
			Failures: testConsecutiveFailureCount,
		}},
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendFail,
			times(testConsecutiveFailureCount, request(500)),
			checkBackendCounter(testConsecutiveFailureCount),
			requestOpen,
			checkBackendCounter(0),
		},
	}, {
		title: "open, fail to close",
		settings: []circuit.BreakerSettings{{
			Type:             circuit.ConsecutiveFailures,
			Failures:         testConsecutiveFailureCount,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendFail,
			times(testConsecutiveFailureCount, request(500)),
			checkBackendCounter(testConsecutiveFailureCount),
			requestOpen,
			checkBackendCounter(0),
			wait(2 * testBreakerTimeout),
			request(500),
			checkBackendCounter(1),
			requestOpen,
			checkBackendCounter(0),
		},
	}, {
		title: "open, fixed during timeout",
		settings: []circuit.BreakerSettings{{
			Type:             circuit.ConsecutiveFailures,
			Failures:         testConsecutiveFailureCount,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendFail,
			times(testConsecutiveFailureCount, request(500)),
			checkBackendCounter(testConsecutiveFailureCount),
			requestOpen,
			checkBackendCounter(0),
			wait(2 * testBreakerTimeout),
			setBackendSucceed,
			times(testHalfOpenRequests+1, request(200)),
			checkBackendCounter(testHalfOpenRequests + 1),
		},
	}, {
		title: "open, fail again during half open",
		settings: []circuit.BreakerSettings{{
			Type:             circuit.ConsecutiveFailures,
			Failures:         testConsecutiveFailureCount,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendFail,
			times(testConsecutiveFailureCount, request(500)),
			checkBackendCounter(testConsecutiveFailureCount),
			requestOpen,
			checkBackendCounter(0),
			wait(2 * testBreakerTimeout),
			setBackendSucceed,
			times(1, request(200)),
			checkBackendCounter(1),
			setBackendFail,
			times(1, request(500)),
			checkBackendCounter(1),
			requestOpen,
			checkBackendCounter(0),
		},
	}, {
		title: "open due to backend being down",
		settings: []circuit.BreakerSettings{{
			Type:     circuit.ConsecutiveFailures,
			Failures: testConsecutiveFailureCount,
		}},
		steps: []scenarioStep{
			request(200),
			checkBackendCounter(1),
			setBackendDown,
			times(testConsecutiveFailureCount, request(503)),
			checkBackendCounter(0),
			requestOpen,
		},
	}} {
		t.Run(s.title, func(t *testing.T) {
			testBreaker(t, s)
		})
	}
}

func TestBreakerRate(t *testing.T) {
	for _, s := range []breakerScenario{{
		title: "open",
	}} {
		t.Run(s.title, func(t *testing.T) {
			testBreaker(t, s)
		})
	}
}

// rate breaker
// different settings per host
// route settings
