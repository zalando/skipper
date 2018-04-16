package proxy_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	circuitfilters "github.com/zalando/skipper/filters/circuit"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

type breakerTestContext struct {
	t        *testing.T
	proxy    *proxytest.TestProxy
	backends map[string]*failingBackend
}

type scenarioStep func(*breakerTestContext)

type breakerScenario struct {
	title    string
	settings []circuit.BreakerSettings
	filters  map[string][]*eskip.Filter
	steps    []scenarioStep
}

const (
	testConsecutiveFailureCount = 5
	testBreakerTimeout          = 3 * time.Millisecond
	testHalfOpenRequests        = 3
	testRateWindow              = 10
	testRateFailures            = 4
	defaultHost                 = "default"
)

func urlHostNerr(u string) string {
	return strings.Split(u, "//")[1]
}

func newBreakerProxy(
	backends map[string]*failingBackend,
	settings []circuit.BreakerSettings,
	filters map[string][]*eskip.Filter,
) *proxytest.TestProxy {
	params := proxy.Params{
		CloseIdleConnsPeriod: -1,
	}

	// for testing, mapping the configured backend hosts to the random generated host
	var r []*eskip.Route
	if len(settings) > 0 {
		for i := range settings {
			b := backends[settings[i].Host]
			if b == nil {
				r = append(r, &eskip.Route{
					Id:          defaultHost,
					HostRegexps: []string{fmt.Sprintf("^%s$", defaultHost)},
					Filters:     filters[defaultHost],
					Backend:     backends[defaultHost].url,
				})
			} else {
				r = append(r, &eskip.Route{
					Id:          settings[i].Host,
					HostRegexps: []string{fmt.Sprintf("^%s$", settings[i].Host)},
					Filters:     filters[settings[i].Host],
					Backend:     backends[settings[i].Host].url,
				})
				settings[i].Host = urlHostNerr(backends[settings[i].Host].url)
			}
		}

		params.CircuitBreakers = circuit.NewRegistry(settings...)
	} else {
		r = append(r, &eskip.Route{
			Backend: backends[defaultHost].url,
		})
	}

	fr := builtin.MakeRegistry()
	return proxytest.WithParams(fr, params, r...)
}

func testBreaker(t *testing.T, s breakerScenario) {
	backends := make(map[string]*failingBackend)
	for _, si := range s.settings {
		h := si.Host
		if h == "" {
			h = defaultHost
		}

		backends[h] = newFailingBackend()
		defer backends[h].close()
	}

	if len(backends) == 0 {
		backends[defaultHost] = newFailingBackend()
		defer backends[defaultHost].close()
	}

	p := newBreakerProxy(backends, s.settings, s.filters)
	defer p.Close()

	steps := s.steps
	c := &breakerTestContext{
		t:        t,
		proxy:    p,
		backends: backends,
	}

	for !t.Failed() && len(steps) > 0 {
		steps[0](c)
		steps = steps[1:]
	}
}

func setBackendHostSucceed(c *breakerTestContext, host string) {
	c.backends[host].succeed()
}

func setBackendSucceed(c *breakerTestContext) {
	setBackendHostSucceed(c, defaultHost)
}

func setBackendFailForHost(c *breakerTestContext, host string) {
	c.backends[host].fail()
}

func setBackendHostFail(host string) scenarioStep {
	return func(c *breakerTestContext) {
		setBackendFailForHost(c, host)
	}
}

func setBackendFail(c *breakerTestContext) {
	setBackendFailForHost(c, defaultHost)
}

func setBackendHostDown(c *breakerTestContext, host string) {
	c.backends[host].down()
}

func setBackendDown(c *breakerTestContext) {
	setBackendHostDown(c, defaultHost)
}

func proxyRequestHost(c *breakerTestContext, host string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.proxy.URL, nil)
	if err != nil {
		return nil, err
	}

	req.Host = host

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}

	rsp.Body.Close()
	return rsp, nil
}

func proxyRequest(c *breakerTestContext) (*http.Response, error) {
	return proxyRequestHost(c, defaultHost)
}

func checkStatus(c *breakerTestContext, rsp *http.Response, expected int) {
	if rsp.StatusCode != expected {
		c.t.Errorf(
			"wrong response status: %d, expected %d",
			rsp.StatusCode,
			expected,
		)
	}
}

func requestHostForStatus(c *breakerTestContext, host string, expectedStatus int) *http.Response {
	rsp, err := proxyRequestHost(c, host)
	if err != nil {
		c.t.Error(err)
		return nil
	}

	checkStatus(c, rsp, expectedStatus)
	return rsp
}

func requestHost(host string, expectedStatus int) scenarioStep {
	return func(c *breakerTestContext) {
		requestHostForStatus(c, host, expectedStatus)
	}
}

func request(expectedStatus int) scenarioStep {
	return func(c *breakerTestContext) {
		requestHostForStatus(c, defaultHost, expectedStatus)
	}
}

func requestOpenForHost(c *breakerTestContext, host string) {
	rsp := requestHostForStatus(c, host, 503)
	if c.t.Failed() {
		return
	}

	if rsp.Header.Get("X-Circuit-Open") != "true" {
		c.t.Error("failed to set circuit open header")
	}
}

func requestHostOpen(host string) scenarioStep {
	return func(c *breakerTestContext) {
		requestOpenForHost(c, host)
	}
}

func requestOpen(c *breakerTestContext) {
	requestOpenForHost(c, defaultHost)
}

func checkBackendForCounter(c *breakerTestContext, host string, count int) {
	if c.backends[host].counter() != count {
		c.t.Error("invalid number of requests on the backend")
	}

	c.backends[host].resetCounter()
}

func checkBackendHostCounter(host string, count int) scenarioStep {
	return func(c *breakerTestContext) {
		checkBackendForCounter(c, host, count)
	}
}

func checkBackendCounter(count int) scenarioStep {
	return func(c *breakerTestContext) {
		checkBackendForCounter(c, defaultHost, count)
	}
}

// as in scenario step N times
func times(n int, s ...scenarioStep) scenarioStep {
	return func(c *breakerTestContext) {
		for !c.t.Failed() && n > 0 {
			for _, si := range s {
				si(c)
			}

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
			// checkBackendCounter(0),
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
			times(testConsecutiveFailureCount, request(http.StatusBadGateway)),
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
		settings: []circuit.BreakerSettings{{
			Type:     circuit.FailureRate,
			Failures: testRateFailures,
			Window:   testRateWindow,
		}},
		steps: []scenarioStep{
			times(testRateWindow, request(200)),
			checkBackendCounter(testRateWindow),
			setBackendFail,
			times(testRateFailures, request(500)),
			checkBackendCounter(testRateFailures),
			requestOpen,
			checkBackendCounter(0),
		},
	}, {
		title: "open, fail to close",
		settings: []circuit.BreakerSettings{{
			Type:             circuit.FailureRate,
			Failures:         testRateFailures,
			Window:           testRateWindow,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			times(testRateWindow, request(200)),
			checkBackendCounter(testRateWindow),
			setBackendFail,
			times(testRateFailures, request(500)),
			checkBackendCounter(testRateFailures),
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
			Type:             circuit.FailureRate,
			Failures:         testRateFailures,
			Window:           testRateWindow,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			times(testRateWindow, request(200)),
			checkBackendCounter(testRateWindow),
			setBackendFail,
			times(testRateFailures, request(500)),
			checkBackendCounter(testRateFailures),
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
			Type:             circuit.FailureRate,
			Failures:         testRateFailures,
			Window:           testRateWindow,
			Timeout:          testBreakerTimeout,
			HalfOpenRequests: testHalfOpenRequests,
		}},
		steps: []scenarioStep{
			times(testRateWindow, request(200)),
			checkBackendCounter(testRateWindow),
			setBackendFail,
			times(testRateFailures, request(500)),
			checkBackendCounter(testRateFailures),
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
			Type:     circuit.FailureRate,
			Failures: testRateFailures,
			Window:   testRateWindow,
		}},
		steps: []scenarioStep{
			times(testRateWindow, request(200)),
			checkBackendCounter(testRateWindow),
			setBackendDown,
			times(testRateFailures, request(http.StatusBadGateway)),
			checkBackendCounter(0),
			requestOpen,
		},
	}} {
		t.Run(s.title, func(t *testing.T) {
			testBreaker(t, s)
		})
	}
}

func TestBreakerMultipleHosts(t *testing.T) {
	testBreaker(t, breakerScenario{
		settings: []circuit.BreakerSettings{{
			Type:     circuit.FailureRate,
			Failures: testRateFailures + 2,
			Window:   testRateWindow,
		}, {
			Host: "foo",
			Type: circuit.BreakerDisabled,
		}, {
			Host:     "bar",
			Type:     circuit.FailureRate,
			Failures: testRateFailures,
			Window:   testRateWindow,
		}},
		steps: []scenarioStep{
			times(
				testRateWindow,
				request(200),
				requestHost("foo", 200),
				requestHost("bar", 200),
			),
			checkBackendCounter(testRateWindow),
			checkBackendHostCounter("foo", testRateWindow),
			checkBackendHostCounter("bar", testRateWindow),
			setBackendFail,
			trace("setting fail"),
			setBackendHostFail("foo"),
			setBackendHostFail("bar"),
			times(testRateFailures,
				request(500),
				requestHost("foo", 500),
				requestHost("bar", 500),
			),
			checkBackendCounter(testRateFailures),
			checkBackendHostCounter("foo", testRateFailures),
			checkBackendHostCounter("bar", testRateFailures),
			request(500),
			requestHost("foo", 500),
			requestHostOpen("bar"),
			checkBackendCounter(1),
			checkBackendHostCounter("foo", 1),
			checkBackendHostCounter("bar", 0),
			request(500),
			requestHost("foo", 500),
			checkBackendCounter(1),
			checkBackendHostCounter("foo", 1),
			requestOpen,
			requestHost("foo", 500),
			// checkBackendCounter(0),
			checkBackendHostCounter("foo", 1),
		},
	})
}

func TestBreakerMultipleHostsAndRouteBasedSettings(t *testing.T) {
	testBreaker(t, breakerScenario{
		settings: []circuit.BreakerSettings{{
			Type:     circuit.FailureRate,
			Failures: testRateFailures + 2,
			Window:   testRateWindow,
		}, {
			Host:     "foo",
			Type:     circuit.FailureRate,
			Failures: testRateFailures + 1,
			Window:   testRateWindow,
		}, {
			Host:     "bar",
			Type:     circuit.FailureRate,
			Failures: testRateFailures + 1,
			Window:   testRateWindow,
		}},
		filters: map[string][]*eskip.Filter{
			"foo": {{
				Name: circuitfilters.DisableBreakerName,
			}},
			"bar": {{
				Name: circuitfilters.RateBreakerName,
				Args: []interface{}{
					testRateFailures,
					testRateWindow,
				},
			}},
		},
		steps: []scenarioStep{
			times(
				testRateWindow,
				request(200),
				requestHost("foo", 200),
				requestHost("bar", 200),
			),
			checkBackendCounter(testRateWindow),
			checkBackendHostCounter("foo", testRateWindow),
			checkBackendHostCounter("bar", testRateWindow),
			setBackendFail,
			setBackendHostFail("foo"),
			setBackendHostFail("bar"),
			times(testRateFailures,
				request(500),
				requestHost("foo", 500),
				requestHost("bar", 500),
			),
			checkBackendCounter(testRateFailures),
			checkBackendHostCounter("foo", testRateFailures),
			checkBackendHostCounter("bar", testRateFailures),
			request(500),
			requestHost("foo", 500),
			requestHostOpen("bar"),
			checkBackendCounter(1),
			checkBackendHostCounter("foo", 1),
			checkBackendHostCounter("bar", 0),
			request(500),
			requestHost("foo", 500),
			checkBackendCounter(1),
			checkBackendHostCounter("foo", 1),
			requestOpen,
			requestHost("foo", 500),
			checkBackendCounter(0),
			checkBackendHostCounter("foo", 1),
		},
	})
}
