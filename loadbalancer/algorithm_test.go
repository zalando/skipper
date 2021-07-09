package loadbalancer

import (
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
)

func TestSelectAlgorithm(t *testing.T) {
	t.Run("not an LB route", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.NetworkBackend,
				Backend:     "https://www.example.org",
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 || len(rr[0].LBEndpoints) != 0 || rr[0].LBAlgorithm != nil {
			t.Fatal("processed non-LB route")
		}
	})

	t.Run("LB route with default algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*roundRobin); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit round-robin algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*roundRobin); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit consistentHash algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "consistentHash",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(consistentHash); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit random algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "random",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*random); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit powerOfRandomNChoices algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "powerOfRandomNChoices",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*powerOfRandomNChoices); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with invalid algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "fooBar",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 0 {
			t.Fatal("failed to drop invalid LB route")
		}
	})

	t.Run("LB route with no LB endpoints", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 0 {
			t.Fatal("failed to drop invalid LB route")
		}
	})

	t.Run("LB route with invalid LB endpoints", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []string{"://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 0 {
			t.Fatal("failed to drop invalid LB route")
		}
	})
}

func TestApply(t *testing.T) {
	N := 10
	eps := make([]string, 0, N)
	for i := 0; i < N; i++ {
		ep := fmt.Sprintf("http://127.0.0.1:123%d/foo", i)
		eps = append(eps, ep)
	}

	for _, tt := range []struct {
		name          string
		iterations    int
		jitter        int
		expected      int
		algorithm     routing.LBAlgorithm
		algorithmName string
	}{
		{
			name:          "random algorithm",
			iterations:    100,
			jitter:        1,
			expected:      N,
			algorithm:     newRandom(eps),
			algorithmName: "random",
		}, {
			name:          "roundrobin algorithm",
			iterations:    100,
			jitter:        1,
			expected:      N,
			algorithm:     newRoundRobin(eps),
			algorithmName: "roundRobin",
		}, {
			name:          "consistentHash algorithm",
			iterations:    100,
			jitter:        0,
			expected:      1,
			algorithm:     newConsistentHash(eps),
			algorithmName: "consistentHash",
		}, {
			name:          "powerOfRandomNChoices algorithm",
			iterations:    100,
			jitter:        0,
			expected:      N,
			algorithm:     newPowerOfRandomNChoices(eps),
			algorithmName: "powerOfRandomNChoices",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://127.0.0.1:1234/foo", nil)
			p := NewAlgorithmProvider()
			r := &routing.Route{
				Route: eskip.Route{
					BackendType: eskip.LBBackend,
					LBAlgorithm: tt.algorithmName,
					LBEndpoints: eps,
				},
			}
			rt := p.Do([]*routing.Route{r})

			lbctx := &routing.LBContext{
				Request: req,
				Route:   rt[0],
			}

			// test
			h := make(map[string]int)
			for i := 0; i < tt.iterations; i++ {
				lbe := tt.algorithm.Apply(lbctx)
				h[lbe.Host] += 1
			}

			if len(h) != tt.expected {
				t.Fatalf("Failed to get expected result %d != %d", tt.expected, len(h))
			}
		})
	}
}

func TestConsistentHashSearch(t *testing.T) {
	apply := func(key string, endpoints []string) string {
		ch := newConsistentHash(endpoints).(consistentHash)
		return endpoints[ch.search(key)]
	}

	endpoints := []string{"http://127.0.0.1:8080", "http://127.0.0.2:8080", "http://127.0.0.3:8080"}
	const key = "192.168.0.1"

	ep := apply(key, endpoints)

	// remove endpoint
	endpoints = endpoints[1:]

	ep1 := apply(key, endpoints)
	if ep != ep1 {
		t.Errorf("expected to select %s, got %s", ep, ep1)
	}

	// add endpoint
	endpoints = append(endpoints, "http://127.0.0.4:8080")

	ep2 := apply(key, endpoints)
	if ep != ep2 {
		t.Errorf("expected to select %s, got %s", ep, ep2)
	}
}

func TestConsistentHashBoundedLoadSearch(t *testing.T) {
	endpoints := []string{"http://127.0.0.1:8080", "http://127.0.0.2:8080", "http://127.0.0.3:8080"}
	r, _ := http.NewRequest("GET", "http://127.0.0.1:1234/foo", nil)
	route := NewAlgorithmProvider().Do([]*routing.Route{{
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
			LBAlgorithm: ConsistentHash.String(),
			LBEndpoints: endpoints,
		},
	}})[0]
	ch := route.LBAlgorithm.(consistentHash)
	ctx := &routing.LBContext{Request: r, Route: route, Params: map[string]interface{}{ConsistentHashBalanceFactor: 1.25}}
	noLoad := ch.Apply(ctx)
	nonBounded := ch.Apply(&routing.LBContext{Request: r, Route: route, Params: map[string]interface{}{}})

	if noLoad != nonBounded {
		t.Error("When no endpoints are overloaded, the chosen endpoint should be the same as standard consistentHash")
	}
	// now we know that noLoad is the endpoint which should be requested for somekey if load is not an issue.
	addInflightRequests(noLoad, 20)
	failover1 := ch.Apply(ctx)
	if failover1 == nonBounded {
		t.Error("When the selected endpoint is overloaded, the chosen endpoint should be different to standard consistentHash")
	}

	// now if 2 endpoints are overloaded, the request should go to the final endpoint
	addInflightRequests(failover1, 20)
	failover2 := ch.Apply(ctx)
	if failover2 == nonBounded || failover2 == failover1 {
		t.Error("Only the final endpoint had load below the average * balanceFactor, so it should have been selected.")
	}

	// now all will have same load, should select the original endpoint again
	addInflightRequests(failover2, 20)
	allLoaded := ch.Apply(ctx)
	if allLoaded != nonBounded {
		t.Error("When all endpoints have the same load, the consistentHash endpoint should be chosen again.")
	}
}

func TestConsistentHashKey(t *testing.T) {
	endpoints := []string{"http://127.0.0.1:8080", "http://127.0.0.2:8080", "http://127.0.0.3:8080"}
	ch := newConsistentHash(endpoints)

	r, _ := http.NewRequest("GET", "http://127.0.0.1:1234/foo", nil)
	r.RemoteAddr = "192.168.0.1:8765"

	rt := NewAlgorithmProvider().Do([]*routing.Route{{
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
			LBAlgorithm: ConsistentHash.String(),
			LBEndpoints: endpoints,
		},
	}})[0]

	defaultEndpoint := ch.Apply(&routing.LBContext{Request: r, Route: rt, Params: make(map[string]interface{})})
	remoteHostEndpoint := ch.Apply(&routing.LBContext{Request: r, Route: rt, Params: map[string]interface{}{ConsistentHashKey: net.RemoteHost(r).String()}})

	if defaultEndpoint != remoteHostEndpoint {
		t.Error("remote host should be used as a default key")
	}

	for i, ep := range endpoints {
		key := fmt.Sprintf("%s-%d", ep, 1) // "ep-0" to "ep-99" is the range of keys for this endpoint. If we use this as the hash key it should select endpoint ep.
		selected := ch.Apply(&routing.LBContext{Request: r, Route: rt, Params: map[string]interface{}{ConsistentHashKey: key}})
		if selected != rt.LBEndpoints[i] {
			t.Errorf("expected: %v, got %v", rt.LBEndpoints[i], selected)
		}
	}
}

func TestConsistentHashBoundedLoadDistribution(t *testing.T) {
	endpoints := []string{"http://127.0.0.1:8080", "http://127.0.0.2:8080", "http://127.0.0.3:8080"}
	r, _ := http.NewRequest("GET", "http://127.0.0.1:1234/foo", nil)
	route := NewAlgorithmProvider().Do([]*routing.Route{{
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
			LBAlgorithm: ConsistentHash.String(),
			LBEndpoints: endpoints,
		},
	}})[0]
	ch := route.LBAlgorithm.(consistentHash)
	balanceFactor := 1.25
	ctx := &routing.LBContext{Request: r, Route: route, Params: map[string]interface{}{ConsistentHashBalanceFactor: balanceFactor}}

	for i := 0; i < 100; i++ {
		ep := ch.Apply(ctx)
		ifr0 := route.LBEndpoints[ch[0].index].Metrics.GetInflightRequests()
		ifr1 := route.LBEndpoints[ch[1].index].Metrics.GetInflightRequests()
		ifr2 := route.LBEndpoints[ch[2].index].Metrics.GetInflightRequests()
		avg := float64(ifr0+ifr1+ifr2) / 3.0
		limit := int(avg*balanceFactor) + 1
		if ifr0 > limit || ifr1 > limit || ifr2 > limit {
			t.Errorf("Expected in-flight requests for each endpoint to be less than %d. In-flight request counts: %d, %d, %d", limit, ifr0, ifr1, ifr2)
		}
		ep.Metrics.IncInflightRequest()
	}
}

func TestConsistentHashKeyDistribution(t *testing.T) {
	endpoints := []string{"http://abc.com:9001", "http://def.com9002", "http://ghi.com:9003", "http://jkl.com:9004", "http://mno.com:9005", "http://pqr.com:9006", "http://stu.com:9007", "http://vwx.com:9008", "http://yza.com:9009", "http://sjsjsjsj.com:9010"}

	const requestCount = 100_000
	stdDev1VNode := measureStdDev(t, endpoints, requestCount, 1)
	stdDev100VNode := measureStdDev(t, endpoints, requestCount, 100)

	if stdDev100VNode >= stdDev1VNode {
		t.Errorf("The standard deviation percentage for request count per endpoint should have been less with more vnodes. 1 vnode std dev was %f, 100vnodes was %f", stdDev1VNode, stdDev100VNode)
	}

	if stdDev100VNode >= 45 { // Chosen as arbitrary target. 1 vnode is about 86% and 100 vnodes is about 40%
		t.Errorf("Standard deviation was too high for 100 vnodes, got %f", stdDev100VNode)
	}
}

func addInflightRequests(endpoint routing.LBEndpoint, count int) {
	for i := 0; i < count; i++ {
		endpoint.Metrics.IncInflightRequest()
	}
}

func measureStdDev(t *testing.T, endpoints []string, requestCount int, vnodes int) float64 {
	route := NewAlgorithmProvider().Do([]*routing.Route{{
		Route: eskip.Route{
			BackendType: eskip.LBBackend,
			LBAlgorithm: ConsistentHash.String(),
			LBEndpoints: endpoints,
		},
	}})[0]
	ch := newConsistentHashInternal(endpoints, vnodes)
	counters := map[string]int{}
	for i := 0; i < requestCount; i++ {
		ctx := makeCtx(route, i)
		selected := ch.Apply(ctx)
		counters[selected.Host] += 1
	}

	sum := 0
	for _, v := range counters {
		sum += v
	}
	if sum != requestCount {
		t.Errorf("Sent 100,000 requests so should have selected 100,000 endpoints, got %d", sum)
	}
	return stdDeviation(counters, sum)
}

func makeCtx(route *routing.Route, reqId int) *routing.LBContext {
	sku := fmt.Sprintf("sku-%d", reqId)
	r, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:9999/products/%s", sku), nil)
	return &routing.LBContext{Request: r, Route: route, Params: map[string]interface{}{ConsistentHashKey: sku}}
}

func stdDeviation(counters map[string]int, sum int) float64 {
	mean := float64(sum) / float64(len(counters))
	summedDiffs := 0.0
	for _, v := range counters {
		diff := float64(v) - mean
		summedDiffs += diff * diff
	}
	stdDev := math.Sqrt(summedDiffs / float64(len(counters)))
	return (stdDev * 100) / mean
}
