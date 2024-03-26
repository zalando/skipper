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
	nRequests            = 15_000
	rtFailureProbability = 0.8
	period               = 100 * time.Millisecond
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
	if err != nil {
		require.NoError(t, err)
	}
	req.Header.Add("ConsistentHashKey", fmt.Sprintf("%d", consistentHashKey))

	rsp, err := ps.Client().Do(req)
	if err != nil {
		require.NoError(t, err)
	}
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
		balanceFactors := []bool{false}
		if algorithm == "consistentHash" {
			balanceFactors = []bool{false, true}
		}

		for _, balanceFactor := range balanceFactors {
			t.Run(fmt.Sprintf("%s_%t", algorithm, balanceFactor), func(t *testing.T) {
				endpointRegistry := defaultEndpointRegistry()

				balanceFactorStr := ""
				if balanceFactor {
					balanceFactorStr = ` -> consistentHashBalanceFactor(1.25)`
				}
				doc := fmt.Sprintf(`* %s -> <%s, "%s", "%s", "%s">`,
					balanceFactorStr, algorithm, services[0].URL, services[1].URL, services[2].URL)

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

				rsp := sendGetRequest(t, ps, 0)
				assert.Equal(t, http.StatusOK, rsp.StatusCode)
				rsp.Body.Close()

				time.Sleep(10 * period)
				/* this test is needed to check PHC will not crash without requests sent during period at all */
			})
		}
	}
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
		balanceFactors := []bool{false}
		if algorithm == "consistentHash" {
			balanceFactors = []bool{false, true}
		}

		for _, balanceFactor := range balanceFactors {
			t.Run(fmt.Sprintf("%s_%t", algorithm, balanceFactor), func(t *testing.T) {
				endpointRegistry := defaultEndpointRegistry()

				balanceFactorStr := ""
				if balanceFactor {
					balanceFactorStr = ` -> consistentHashBalanceFactor(1.25)`
				}
				doc := fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") %s -> <%s, "%s", "%s", "%s">`,
					balanceFactorStr, algorithm, services[0].URL, services[1].URL, services[2].URL)

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
			})
		}
	}
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
		balanceFactors := []bool{false}
		if algorithm == "consistentHash" {
			balanceFactors = []bool{false, true}
		}

		for _, balanceFactor := range balanceFactors {
			t.Run(fmt.Sprintf("%s_%t", algorithm, balanceFactor), func(t *testing.T) {
				endpointRegistry := defaultEndpointRegistry()

				balanceFactorStr := ""
				if balanceFactor {
					balanceFactorStr = ` -> consistentHashBalanceFactor(1.25)`
				}
				doc := fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") %s -> <%s, "%s", "%s", "%s">`,
					balanceFactorStr, algorithm, services[0].URL, services[1].URL, services[2].URL)

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
				assert.InDelta(t, 0.33*rtFailureProbability*(1.0-rtFailureProbability)*float64(nRequests), failedReqs, 0.1*float64(nRequests))
			})
		}
	}
}
