package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			endpointRegistry := defaultEndpointRegistry()
			doc := fmt.Sprintf(`* -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL)

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

			rsp, err := ps.Client().Get(ps.URL)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, http.StatusOK, rsp.StatusCode)
			rsp.Body.Close()

			time.Sleep(10 * period)
			/* this test is needed to check PHC will not crash without requests sent during period at all */
		})
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

	failedReqs := 0
	for i := 0; i < nRequests; i++ {
		rsp, err := ps.Client().Get(ps.URL)
		if err != nil {
			t.Fatal(err)
		}

		if rsp.StatusCode != http.StatusOK {
			failedReqs++
		}
		rsp.Body.Close()
	}
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
			endpointRegistry := defaultEndpointRegistry()
			doc := fmt.Sprintf(`* -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL)

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

			failedReqs := 0
			for i := 0; i < nRequests; i++ {
				req, err := http.NewRequest("GET", ps.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Add("ConsistentHashKey", fmt.Sprintf("%d", i))

				rsp, err := ps.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}

				if rsp.StatusCode != http.StatusOK {
					failedReqs++
				}
				rsp.Body.Close()
			}
			assert.Equal(t, 0, failedReqs)
		})
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
		t.Run(algorithm, func(t *testing.T) {
			endpointRegistry := defaultEndpointRegistry()
			doc := fmt.Sprintf(`* -> backendTimeout("5ms") -> consistentHashKey("${request.header.ConsistentHashKey}") -> <%s, "%s", "%s", "%s">`,
				algorithm, services[0].URL, services[1].URL, services[2].URL)

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

			failedReqs := 0
			for i := 0; i < nRequests; i++ {
				req, err := http.NewRequest("GET", ps.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Add("ConsistentHashKey", fmt.Sprintf("%d", i))

				rsp, err := ps.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}

				if rsp.StatusCode != http.StatusOK {
					failedReqs++
				}
				rsp.Body.Close()
			}
			assert.InDelta(t, 0.33*rtFailureProbability*(1.0-rtFailureProbability)*float64(nRequests), failedReqs, 0.1*float64(nRequests))
		})
	}
}
