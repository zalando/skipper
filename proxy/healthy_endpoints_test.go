package proxy

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/routing"
)

const (
	nRequests            = 10_000
	rtFailureProbability = 0.8
	period               = 1 * time.Second
)

func defaultEndpointRegistry() *routing.EndpointRegistry {
	return routing.NewEndpointRegistry(routing.RegistryOptions{})
}

func TestPHCForSingleHealthyEndpoint(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer service.Close()
	endpointRegistry := defaultEndpointRegistry()

	doc := fmt.Sprintf(`* -> "%s"`, service.URL)
	tp, err := newTestProxyWithParams(doc, Params{
		EndpointRegistry: endpointRegistry,
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

	// Let endpointregistry update all stats
	time.Sleep(period + time.Millisecond)
	dummy := fmt.Sprintf(`Header("Foo", "Bar") -> "%s"`, service.URL)
	tp.dc.UpdateDoc(dummy, nil)
	tp.dc.UpdateDoc(doc, nil)
	time.Sleep(10 * time.Millisecond)

	failedReqs = 0
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
	endpointRegistry := defaultEndpointRegistry()

	doc := fmt.Sprintf(`* -> <random, "%s", "%s", "%s">`, services[0].URL, services[1].URL, services[2].URL)
	tp, err := newTestProxyWithParams(doc, Params{
		EndpointRegistry: endpointRegistry,
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

	// Let endpointregistry update all stats
	time.Sleep(period + time.Millisecond)
	dummy := fmt.Sprintf(`* -> <random, "%s", "%s">`, services[0].URL, services[1].URL)
	tp.dc.UpdateDoc(dummy, nil)
	tp.dc.UpdateDoc(doc, nil)
	time.Sleep(10 * time.Millisecond)

	failedReqs = 0
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

type roundTripperUnhealthyHost struct {
	inner       http.RoundTripper
	host        string
	probability float64
	rnd         *rand.Rand
}

type RoundTripperUnhealthyHostParams struct {
	Host        string
	Probability float64
}

func (rt *roundTripperUnhealthyHost) RoundTrip(r *http.Request) (*http.Response, error) {
	p := rt.rnd.Float64()
	if p < rt.probability && r.URL.Host == rt.host {
		return nil, fmt.Errorf("roundTrip fail injected")
	}

	return rt.inner.RoundTrip(r)
}

func newRoundTripperUnhealthyHost(o *RoundTripperUnhealthyHostParams) func(r http.RoundTripper) http.RoundTripper {
	return func(r http.RoundTripper) http.RoundTripper {
		return &roundTripperUnhealthyHost{inner: r, rnd: rand.New(loadbalancer.NewLockedSource()), host: o.Host, probability: o.Probability}
	}
}

func TestPHCForMultipleHealthyAndOneUnhealthyEndpoints(t *testing.T) {
	services := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		services = append(services, service)
		defer service.Close()
	}
	endpointRegistry := defaultEndpointRegistry()

	doc := fmt.Sprintf(`* -> <random, "%s", "%s", "%s">`, services[0].URL, services[1].URL, services[2].URL)
	tp, err := newTestProxyWithParams(doc, Params{
		EndpointRegistry:           endpointRegistry,
		CustomHttpRoundTripperWrap: newRoundTripperUnhealthyHost(&RoundTripperUnhealthyHostParams{Host: services[0].URL[7:], Probability: rtFailureProbability}),
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
	assert.InDelta(t, 0.33*rtFailureProbability*nRequests, failedReqs, 0.05*nRequests)

	// Let endpointregistry update all stats
	time.Sleep(period + time.Millisecond)
	dummy := fmt.Sprintf(`* -> <random, "%s", "%s">`, services[0].URL, services[1].URL)
	tp.dc.UpdateDoc(dummy, nil)
	tp.dc.UpdateDoc(doc, nil)
	time.Sleep(10 * time.Millisecond)

	failedReqs = 0
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
	assert.InDelta(t, 0.33*rtFailureProbability*nRequests, failedReqs, 0.05*nRequests)
	// After PHC is implemented, I expect failed requests to decrease like this:
	// assert.InDelta(t, 0.33*rtFailureProbability*(1.0-rtFailureProbability)*nRequests, failedReqs, 0.05*nRequests)
}
