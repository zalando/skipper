package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics/metricstest"
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
		MinRequests:                   10,
		MaxHealthCheckDropProbability: 1.0,
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

func setupServices(t *testing.T, healthy, unhealthy int) []*httptest.Server {
	services := []*httptest.Server{}
	for i := 0; i < healthy; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		t.Cleanup(service.Close)
	}
	for i := 0; i < unhealthy; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		t.Cleanup(service.Close)
	}
	return services
}

func TestPHCWithoutRequests(t *testing.T) {
	services := setupServices(t, 3, 0)

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
			rsp := sendGetRequest(t, ps, 0)
			assert.Equal(t, http.StatusOK, rsp.StatusCode)
			rsp.Body.Close()

			time.Sleep(10 * period)
			/* this test is needed to check PHC will not crash without requests sent during period at all */
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
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

	doc := fmt.Sprintf(`* -> "%s"`, service.URL)
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
			_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
			failedReqs := sendGetRequests(t, ps)
			assert.Equal(t, 0, failedReqs)
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
		failedReqs := sendGetRequests(t, ps)
		assert.Equal(t, 0, failedReqs)
	})
}

func TestPHCForMultipleHealthyAndOneUnhealthyEndpoints(t *testing.T) {
	services := setupServices(t, 2, 1)
	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
			failedReqs := sendGetRequests(t, ps)
			assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
			mockMetrics.WithCounters(func(counters map[string]int64) {
				assert.InDelta(t, float64(nRequests), float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(nRequests)) // allow 30% error
			})
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL), defaultEndpointRegistry())
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
	})
}

func TestPHCForMultipleHealthyAndMultipleUnhealthyEndpoints(t *testing.T) {
	services := setupServices(t, 2, 2)
	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {

			endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
				PassiveHealthCheckEnabled:     true,
				StatsResetPeriod:              period,
				MinRequests:                   1, // with 3 test case fails
				MaxHealthCheckDropProbability: 1.0,
				MinHealthCheckDropProbability: 0.01,
			})

			mockMetrics, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL, services[3].URL), endpointRegistry)
			failedReqs := sendGetRequests(t, ps)
			assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
			mockMetrics.WithCounters(func(counters map[string]int64) {
				assert.InDelta(t, float64(nRequests)*2.0, float64(counters["passive-health-check.endpoints.dropped"]), 0.3*float64(nRequests)) // allow 30% error
			})
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{
			PassiveHealthCheckEnabled:     true,
			StatsResetPeriod:              period,
			MinRequests:                   1, // with 3 test case fails
			MaxHealthCheckDropProbability: 1.0,
			MinHealthCheckDropProbability: 0.01,
		})
		_, ps := setupProxyWithCustomEndpointRegisty(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL, services[3].URL), endpointRegistry)
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
	})
}
