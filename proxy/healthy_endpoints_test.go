package proxy

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
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
	req, err := http.NewRequest("GET", ps.URL, nil)
	require.NoError(t, err)
	req.Header.Add("ConsistentHashKey", fmt.Sprintf("%d", consistentHashKey))

	rsp, err := ps.Client().Do(req)
	require.NoError(t, err)
	return rsp
}

func sendGetRequests(t *testing.T, ps *httptest.Server) (failed int) {
	for i := 0; i < nRequests; i++ {
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
	return setupProxyWithCustomEndpointRegisty(t, doc, defaultEndpointRegistry())
}

func setupProxyWithCustomEndpointRegisty(t *testing.T, doc string, endpointRegistry *routing.EndpointRegistry) (*metricstest.MockMetrics, *httptest.Server) {
	m := &metricstest.MockMetrics{}

	tp, err := newTestProxyWithParams(doc, Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
		Metrics:                  m,
	})
	require.NoError(t, err)

	ps := httptest.NewServer(tp.proxy)

	t.Cleanup(tp.close)
	t.Cleanup(ps.Close)

	return m, ps
}

func setupServices(t *testing.T, healthy, unhealthy int) []string {
	services := []string{}
	for i := 0; i < healthy; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service.URL)
		t.Cleanup(service.Close)
	}
	for i := 0; i < unhealthy; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service.URL)
		t.Cleanup(service.Close)
	}
	return services
}

func TestPHCWithoutRequests(t *testing.T) {
	services := setupServices(t, 3, 0)

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			_, ps := setupProxy(t, fmt.Sprintf(`* -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0], services[1], services[2]))
			rsp := sendGetRequest(t, ps, 0)
			assert.Equal(t, http.StatusOK, rsp.StatusCode)
			rsp.Body.Close()

			time.Sleep(10 * period)
			/* this test is needed to check PHC will not crash without requests sent during period at all */
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0], services[1], services[2]))
		rsp := sendGetRequest(t, ps, 0)
		assert.Equal(t, http.StatusOK, rsp.StatusCode)
		rsp.Body.Close()

		time.Sleep(10 * period)
		/* this test is needed to check PHC will not crash without requests sent during period at all */
	})
}

func TestPHCForSingleHealthyEndpoint(t *testing.T) {
	service := setupServices(t, 1, 0)[0]
	endpointRegistry := defaultEndpointRegistry()

	doc := fmt.Sprintf(`* -> "%s"`, service)
	tp, err := newTestProxyWithParams(doc, Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
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

func TestPHCForMultipleHealthyEndpoints(t *testing.T) {
	services := setupServices(t, 3, 0)

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			_, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0], services[1], services[2]))
			va := fireVegeta(t, ps, 3000, 1*time.Second, 5*time.Second)
			count200, ok := va.CountStatus(200)
			assert.True(t, ok)
			assert.InDelta(t, count200, 15000, 50) // 3000*5s, the delta is for CI
			assert.Equal(t, count200, int(va.TotalRequests()))
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0], services[1], services[2]))
		va := fireVegeta(t, ps, 3000, 1*time.Second, 5*time.Second)
		count200, ok := va.CountStatus(200)
		assert.True(t, ok)
		assert.Equal(t, count200, int(va.TotalRequests()))
	})
}

func TestPHCForMultipleHealthyAndOneUnhealthyEndpoints(t *testing.T) {
	services := setupServices(t, 2, 1)
	for _, algorithm := range []string{"random", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			mockMetrics, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0], services[1], services[2]))

			va := fireVegeta(t, ps, 3000, 1*time.Second, 5*time.Second)

			count200, ok := va.CountStatus(200)
			assert.True(t, ok)

			count504, ok := va.CountStatus(504)
			assert.True(t, ok)

			failedRequests := math.Abs(float64(va.TotalRequests())) - float64(count200)
			t.Logf("total requests: %d, count200: %d, count504: %d, failedRequests: %f", va.TotalRequests(), count200, count504, failedRequests)

			assert.InDelta(t, float64(count504), failedRequests, 5)
			assert.InDelta(t, 0, float64(failedRequests), 0.1*float64(va.TotalRequests()))
			mockMetrics.WithCounters(func(counters map[string]int64) {
				assert.InDelta(t, float64(va.TotalRequests()), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(va.TotalRequests())) // allow 30% error
				assert.InDelta(t, float64(va.TotalRequests()), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(va.TotalRequests()))   // allow 30% error
			})
		})
	}

	t.Run("consistentHash", func(t *testing.T) {
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
			PassiveHealthCheckEnabled:     true,
			StatsResetPeriod:              1 * time.Second,
			MinRequests:                   1, // with 2 test case fails on github actions
			MaxHealthCheckDropProbability: 0.95,
			MinHealthCheckDropProbability: 0.01,
		})
		mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <consistentHash, "%s", "%s", "%s">`,
			services[0], services[1], services[2]), endpointRegistry)
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
		mockMetrics.WithCounters(func(counters map[string]int64) {
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(nRequests)) // allow 30% error
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests))   // allow 30% error
		})
	})

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		mockMetrics, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0], services[1], services[2]))
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
		mockMetrics.WithCounters(func(counters map[string]int64) {
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(nRequests)) // allow 30% error
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests))   // allow 30% error
		})
	})
}

func TestPHCForMultipleHealthyAndMultipleUnhealthyEndpoints(t *testing.T) {
	services := setupServices(t, 2, 2)
	for _, algorithm := range []string{"random", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			mockMetrics, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s", "%s">`,
				algorithm, services[0], services[1], services[2], services[3]))

			va := fireVegeta(t, ps, 3000, 1*time.Second, 5*time.Second)

			count200, ok := va.CountStatus(200)
			assert.True(t, ok)

			count504, ok := va.CountStatus(504)
			assert.True(t, ok)

			failedRequests := math.Abs(float64(va.TotalRequests())) - float64(count200)
			t.Logf("total requests: %d, count200: %d, count504: %d, failedRequests: %f", va.TotalRequests(), count200, count504, failedRequests)

			assert.InDelta(t, float64(count504), failedRequests, 5)
			assert.InDelta(t, 0, float64(failedRequests), 0.3*float64(va.TotalRequests()))
			mockMetrics.WithCounters(func(counters map[string]int64) {
				assert.InDelta(t, 2*float64(va.TotalRequests()), float64(counters["passive-health-check.endpoints.dropped"]), 0.6*float64(va.TotalRequests()))
				assert.InDelta(t, float64(va.TotalRequests()), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(va.TotalRequests())) // allow 30% error
			})
		})
	}

	t.Run("consistentHash", func(t *testing.T) {
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
			PassiveHealthCheckEnabled:     true,
			StatsResetPeriod:              1 * time.Second,
			MinRequests:                   1, // with 2 test case fails on github actions and -race
			MaxHealthCheckDropProbability: 0.95,
			MinHealthCheckDropProbability: 0.01,
		})
		mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <consistentHash, "%s", "%s", "%s", "%s">`,
			services[0], services[1], services[2], services[3]), endpointRegistry)
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.2*float64(nRequests))
		mockMetrics.WithCounters(func(counters map[string]int64) {
			assert.InDelta(t, 2*float64(nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.6*float64(nRequests))
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
		})
	})

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
			PassiveHealthCheckEnabled:     true,
			StatsResetPeriod:              1 * time.Second,
			MinRequests:                   1, // with 2 test case fails on github actions and -race
			MaxHealthCheckDropProbability: 0.95,
			MinHealthCheckDropProbability: 0.01,
		})
		mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s", "%s">`,
			services[0], services[1], services[2], services[3]), endpointRegistry)
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.2*float64(nRequests))
		mockMetrics.WithCounters(func(counters map[string]int64) {
			assert.InDelta(t, 2*float64(nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.6*float64(nRequests))
			assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.requests.passed"]), 0.3*float64(nRequests)) // allow 30% error
		})
	})
}
