package proxy

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics/metricstest"
	zhttptest "github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/routing"
)

const (
	nRequests = 15_000
	period    = 100 * time.Millisecond
)

func defaultEndpointRegistry() *routing.EndpointRegistry {
	return routing.NewEndpointRegistry(routing.RegistryOptions{
		PassiveHealthCheckEnabled:     true,
		StatsResetPeriod:              period,
		MinRequests:                   2,
		MaxHealthCheckDropProbability: 0.95,
		MinHealthCheckDropProbability: 0.01,
	})
}

func sendGetRequest(t *testing.T, ps *httptest.Server, consistentHashKey int) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", ps.URL, nil)
	require.NoError(t, err)
	req.Header.Add("ConsistentHashKey", fmt.Sprintf("%d", consistentHashKey))

	rsp, err := ps.Client().Do(req)
	require.NoError(t, err)
	return rsp
}

func sendGetRequests(t *testing.T, ps *httptest.Server) (failed int) {
	t.Helper()
	for i := range nRequests {
		rsp := sendGetRequest(t, ps, i)
		if rsp.StatusCode != http.StatusOK {
			failed++
		}
		rsp.Body.Close()
	}
	return
}

func fireVegeta(t *testing.T, ps *httptest.Server, freq int, per time.Duration, timeout time.Duration) *zhttptest.VegetaAttacker {
	t.Helper()
	va := zhttptest.NewVegetaAttacker(ps.URL, freq, per, timeout)
	va.Attack(io.Discard, 5*time.Second, t.Name())
	return va
}

func setupProxy(t *testing.T, doc string) (*metricstest.MockMetrics, *httptest.Server) {
	t.Helper()
	m := &metricstest.MockMetrics{}
	endpointRegistry := defaultEndpointRegistry()
	proxyParams := Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
		Metrics:                  m,
		PassiveHealthCheck: &PassiveHealthCheck{
			MaxUnhealthyEndpointsRatio: 1.0,
		},
	}

	return m, setupProxyWithCustomProxyParams(t, doc, proxyParams)
}

func setupProxyWithCustomEndpointRegisty(t *testing.T, doc string, endpointRegistry *routing.EndpointRegistry) (*metricstest.MockMetrics, *httptest.Server) {
	t.Helper()
	m := &metricstest.MockMetrics{}
	proxyParams := Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
		Metrics:                  m,
		PassiveHealthCheck: &PassiveHealthCheck{
			MaxUnhealthyEndpointsRatio: 1.0,
		},
	}

	return m, setupProxyWithCustomProxyParams(t, doc, proxyParams)
}

func setupProxyWithCustomProxyParams(t *testing.T, doc string, proxyParams Params) *httptest.Server {
	t.Helper()
	tp, err := newTestProxyWithParams(doc, proxyParams)
	require.NoError(t, err)

	ps := httptest.NewServer(tp.proxy)

	t.Cleanup(tp.close)
	t.Cleanup(ps.Close)

	return ps
}

func setupServices(t *testing.T, healthy, unhealthy int) string {
	t.Helper()
	services := []string{}
	for range healthy {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service.URL)
		t.Cleanup(service.Close)
	}
	for range unhealthy {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service.URL)
		t.Cleanup(service.Close)
	}
	return fmt.Sprint(`"`, strings.Join(services, `", "`), `"`)
}

func TestPHCWithoutRequests(t *testing.T) {
	servicesString := setupServices(t, 3, 0)

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			_, ps := setupProxy(t, fmt.Sprintf(`* -> <%s, %s>`,
				algorithm, servicesString))
			rsp := sendGetRequest(t, ps, 0)
			assert.Equal(t, http.StatusOK, rsp.StatusCode)
			rsp.Body.Close()

			time.Sleep(10 * period)
			/* this test is needed to check PHC will not crash without requests sent during period at all */
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashBalanceFactor(1.25) -> <consistentHash, %s>`,
			servicesString))
		rsp := sendGetRequest(t, ps, 0)
		assert.Equal(t, http.StatusOK, rsp.StatusCode)
		rsp.Body.Close()

		time.Sleep(10 * period)
		/* this test is needed to check PHC will not crash without requests sent during period at all */
	})
}

func TestPHCForSingleHealthyEndpoint(t *testing.T) {
	servicesString := setupServices(t, 1, 0)
	endpointRegistry := defaultEndpointRegistry()

	doc := fmt.Sprintf(`* -> %s`, servicesString)
	tp, err := newTestProxyWithParams(doc, Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
		PassiveHealthCheck: &PassiveHealthCheck{
			MaxUnhealthyEndpointsRatio: 1.0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	failedReqs := sendGetRequests(t, ps)
	assert.Equal(t, 0, failedReqs)
}

func TestPHC(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	rpsFactor := 0.0
	for _, tt := range []struct {
		name               string
		healthy, unhealthy int
	}{
		{"single healthy", 1, 0},
		{"multiple healthy", 2, 0},
		{"multiple healthy and one unhealthy", 2, 1},
		{"multiple healthy and multiple unhealthy", 2, 2},
	} {
		if (tt.healthy != 0 && tt.unhealthy == 0) || (tt.healthy == 0 && tt.unhealthy != 0) {
			t.Logf("test %s has no mitigations", tt.name)
			rpsFactor = 0.0
		} else {
			t.Logf("test %s has mitigations", tt.name)
			rpsFactor = 1.0
		}
		t.Run(tt.name, func(t *testing.T) {
			servicesString := setupServices(t, tt.healthy, tt.unhealthy)
			for _, algorithm := range []string{"random", "roundRobin", "powerOfRandomNChoices"} {
				t.Run(algorithm, func(t *testing.T) {
					t.Parallel()
					mockMetrics, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("20ms") -> <%s, %s>`,
						algorithm, servicesString))
					va := fireVegeta(t, ps, 5000, 1*time.Second, 5*time.Second)

					count200, ok := va.CountStatus(200)
					assert.True(t, ok)

					count504, ok := va.CountStatus(504)
					assert.Condition(t, func() bool {
						if tt.unhealthy == 0 && (ok || count504 == 0) {
							return true
						} else if tt.unhealthy > 0 && ok {
							return true
						} else {
							return false
						}
					})

					failedRequests := math.Abs(float64(va.TotalRequests())) - float64(count200)
					t.Logf("total requests: %d, count200: %d, count504: %d, failedRequests: %f", va.TotalRequests(), count200, count504, failedRequests)

					assert.InDelta(t, float64(count504), failedRequests, 5)
					assert.InDelta(t, 0, float64(failedRequests), 0.3*float64(va.TotalRequests()))
					mockMetrics.WithCounters(func(counters map[string]int64) {
						assert.InDelta(t, float64(tt.unhealthy)*float64(va.TotalRequests()), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(tt.unhealthy)*float64(va.TotalRequests()))
						assert.InDelta(t, float64(va.TotalRequests())*rpsFactor, float64(counters["passive-health-check.requests.passed"]), 0.3*float64(va.TotalRequests())) // allow 30% error
					})
				})
			}

			t.Run("consistentHash", func(t *testing.T) {
				consistentHashCustomEndpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
					PassiveHealthCheckEnabled:     true,
					StatsResetPeriod:              1 * time.Second,
					MinRequests:                   1, // with 2 test case fails on github actions
					MaxHealthCheckDropProbability: 0.95,
					MinHealthCheckDropProbability: 0.01,
				})
				mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <consistentHash, %s>`,
					servicesString), consistentHashCustomEndpointRegistry)
				failedReqs := sendGetRequests(t, ps)
				assert.InDelta(t, 0, failedReqs, 0.2*float64(nRequests))
				mockMetrics.WithCounters(func(counters map[string]int64) {
					assert.InDelta(t, float64(tt.unhealthy*nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(tt.unhealthy)*float64(nRequests))
					assert.InDelta(t, float64(nRequests)*rpsFactor, float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
				})
			})

			t.Run("consistent hash with balance factor", func(t *testing.T) {
				consistentHashCustomEndpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
					PassiveHealthCheckEnabled:     true,
					StatsResetPeriod:              1 * time.Second,
					MinRequests:                   1, // with 2 test case fails on github actions
					MaxHealthCheckDropProbability: 0.95,
					MinHealthCheckDropProbability: 0.01,
				})
				mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, %s>`,
					servicesString), consistentHashCustomEndpointRegistry)
				failedReqs := sendGetRequests(t, ps)
				assert.InDelta(t, 0, failedReqs, 0.2*float64(nRequests))
				mockMetrics.WithCounters(func(counters map[string]int64) {
					assert.InDelta(t, float64(tt.unhealthy*nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(tt.unhealthy)*float64(nRequests))
					assert.InDelta(t, float64(nRequests)*rpsFactor, float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
				})
			})
		})
	}
}

func TestPHCNoHealthyEndpoints(t *testing.T) {
	const (
		healthy   = 0
		unhealthy = 4
	)

	servicesString := setupServices(t, healthy, unhealthy)
	mockMetrics, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("20ms") -> <roundRobin, %s>`,
		servicesString))
	va := fireVegeta(t, ps, 5000, 1*time.Second, 5*time.Second)

	count200, ok := va.CountStatus(200)
	assert.False(t, ok)

	count504, ok := va.CountStatus(504)
	assert.True(t, ok)

	failedRequests := math.Abs(float64(va.TotalRequests())) - float64(count200)
	t.Logf("total requests: %d, count200: %d, count504: %d, failedRequests: %f", va.TotalRequests(), count200, count504, failedRequests)

	assert.InDelta(t, float64(count504), failedRequests, 5)
	assert.InDelta(t, float64(va.TotalRequests()), float64(failedRequests), 0.1*float64(va.TotalRequests()))
	mockMetrics.WithCounters(func(counters map[string]int64) {
		assert.InDelta(t, float64(unhealthy)*float64(va.TotalRequests()), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(unhealthy)*float64(va.TotalRequests()))
		assert.InDelta(t, 0.0, float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
	})
}

func TestPHCMaxUnhealthyEndpointsRatioParam(t *testing.T) {
	const (
		healthy                    = 0
		unhealthy                  = 4
		maxUnhealthyEndpointsRatio = 0.49 // slightly less than 0.5 to avoid equality and looking up the third endpoint
	)

	servicesString := setupServices(t, healthy, unhealthy)
	mockMetrics := &metricstest.MockMetrics{}
	endpointRegistry := defaultEndpointRegistry()
	proxyParams := Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
		Metrics:                  mockMetrics,
		PassiveHealthCheck: &PassiveHealthCheck{
			MaxUnhealthyEndpointsRatio: maxUnhealthyEndpointsRatio,
		},
	}
	ps := setupProxyWithCustomProxyParams(t, fmt.Sprintf(`* -> backendTimeout("20ms") -> <random, %s>`,
		servicesString), proxyParams)
	va := fireVegeta(t, ps, 5000, 1*time.Second, 5*time.Second)

	count200, ok := va.CountStatus(200)
	assert.False(t, ok)

	count504, ok := va.CountStatus(504)
	assert.True(t, ok)

	failedRequests := math.Abs(float64(va.TotalRequests())) - float64(count200)
	t.Logf("total requests: %d, count200: %d, count504: %d, failedRequests: %f", va.TotalRequests(), count200, count504, failedRequests)

	assert.InDelta(t, float64(count504), failedRequests, 5)
	assert.InDelta(t, float64(va.TotalRequests()), float64(failedRequests), 0.1*float64(va.TotalRequests()))
	mockMetrics.WithCounters(func(counters map[string]int64) {
		assert.InDelta(t, maxUnhealthyEndpointsRatio*float64(unhealthy)*float64(va.TotalRequests()),
			float64(counters["passive-health-check.endpoints.dropped"]),
			0.3*maxUnhealthyEndpointsRatio*float64(unhealthy)*float64(va.TotalRequests()),
		)
		assert.InDelta(t, 0.0, float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
	})
}
