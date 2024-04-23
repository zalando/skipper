package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func setupProxy(t *testing.T, doc string) (*testProxy, *httptest.Server) {
	endpointRegistry := defaultEndpointRegistry()

	tp, err := newTestProxyWithParams(doc, Params{
		EnablePassiveHealthCheck: true,
		EndpointRegistry:         endpointRegistry,
	})
	require.NoError(t, err)

	ps := httptest.NewServer(tp.proxy)

	return tp, ps
}

func TestPHCWithoutRequests(t *testing.T) {
	services := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		defer service.Close()
	}

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			tp, ps := setupProxy(t, fmt.Sprintf(`* -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL))
			defer tp.close()
			defer ps.Close()
			rsp := sendGetRequest(t, ps, 0)
			assert.Equal(t, http.StatusOK, rsp.StatusCode)
			rsp.Body.Close()

			time.Sleep(10 * period)
			/* this test is needed to check PHC will not crash without requests sent during period at all */
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		tp, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL))
		defer tp.close()
		defer ps.Close()
		rsp := sendGetRequest(t, ps, 0)
		assert.Equal(t, http.StatusOK, rsp.StatusCode)
		rsp.Body.Close()

		time.Sleep(10 * period)
		/* this test is needed to check PHC will not crash without requests sent during period at all */
	})
}

func TestPHCForSingleHealthyEndpoint(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer service.Close()
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
	services := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		defer service.Close()
	}

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			tp, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL))
			defer tp.close()
			defer ps.Close()
			failedReqs := sendGetRequests(t, ps)
			assert.Equal(t, 0, failedReqs)
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		tp, ps := setupProxy(t, fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL))
		defer tp.close()
		defer ps.Close()
		failedReqs := sendGetRequests(t, ps)
		assert.Equal(t, 0, failedReqs)
	})
}

func TestPHCForMultipleHealthyAndOneUnhealthyEndpoints(t *testing.T) {
	services := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		serviceNum := i
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if serviceNum == 0 {
				// emulating unhealthy endpoint
				time.Sleep(100 * time.Millisecond)
			}
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		defer service.Close()
	}

	for _, algorithm := range []string{"random", "consistentHash", "roundRobin", "powerOfRandomNChoices"} {
		t.Run(algorithm, func(t *testing.T) {
			tp, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL))
			defer tp.close()
			defer ps.Close()
			failedReqs := sendGetRequests(t, ps)
			assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
		})
	}

	t.Run("consistent hash with balance factor", func(t *testing.T) {
		tp, ps := setupProxy(t, fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> consistentHashBalanceFactor(1.25) -> <consistentHash, "%s", "%s", "%s">`,
			services[0].URL, services[1].URL, services[2].URL))
		defer tp.close()
		defer ps.Close()
		failedReqs := sendGetRequests(t, ps)
		assert.InDelta(t, 0, failedReqs, 0.1*float64(nRequests))
	})
}
