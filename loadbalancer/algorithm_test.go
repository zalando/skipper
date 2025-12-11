package loadbalancer

import (
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org:443" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*roundRobin); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit round-robin algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "roundRobin",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org:443" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*roundRobin); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit consistentHash algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "consistentHash",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org:443" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*consistentHash); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit random algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "random",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org:443" ||
			rr[0].LBEndpoints[0].Metrics == nil {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*random); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit powerOfRandomNChoices algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "powerOfRandomNChoices",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org:443" ||
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
	const R = 1000
	const N = 10
	eps := make([]string, 0, N)
	for i := 0; i < N; i++ {
		ep := fmt.Sprintf("http://127.0.0.1:123%d/foo", i)
		eps = append(eps, ep)
	}

	for _, tt := range []struct {
		name          string
		expected      int
		algorithm     routing.LBAlgorithm
		algorithmName string
	}{
		{
			name:          "random algorithm",
			expected:      N,
			algorithm:     newRandom(eps),
			algorithmName: "random",
		}, {
			name:          "roundrobin algorithm",
			expected:      N,
			algorithm:     newRoundRobin(eps),
			algorithmName: "roundRobin",
		}, {
			name:          "consistentHash algorithm",
			expected:      1,
			algorithm:     newConsistentHash(eps),
			algorithmName: "consistentHash",
		}, {
			name:          "powerOfRandomNChoices algorithm",
			expected:      N,
			algorithm:     newPowerOfRandomNChoices(eps),
			algorithmName: "powerOfRandomNChoices",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://127.0.0.1:1234/foo", nil)
			p := NewAlgorithmProvider()
			endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
			defer endpointRegistry.Close()
			r := &routing.Route{
				Route: eskip.Route{
					BackendType: eskip.LBBackend,
					LBAlgorithm: tt.algorithmName,
					LBEndpoints: eps,
				},
			}
			rt := p.Do([]*routing.Route{r})
			endpointRegistry.Do([]*routing.Route{r})

			lbctx := &routing.LBContext{
				Request:     req,
				Route:       rt[0],
				LBEndpoints: rt[0].LBEndpoints,
			}

			h := make(map[string]int)
			for i := 0; i < R; i++ {
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
		p := NewAlgorithmProvider()
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: ConsistentHash.String(),
				LBEndpoints: endpoints,
			},
		}
		p.Do([]*routing.Route{r})
		endpointRegistry.Do([]*routing.Route{r})

		ch := newConsistentHash(endpoints).(*consistentHash)
		ctx := &routing.LBContext{Route: r, LBEndpoints: r.LBEndpoints, Params: map[string]interface{}{ConsistentHashKey: key}}
		return endpoints[ch.search(key, ctx)]
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

	ch := route.LBAlgorithm.(*consistentHash)
	ctx := &routing.LBContext{
		Request:     r,
		Route:       route,
		LBEndpoints: route.LBEndpoints,
		Params:      map[string]interface{}{ConsistentHashBalanceFactor: 1.25},
	}
	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	defer endpointRegistry.Close()
	endpointRegistry.Do([]*routing.Route{route})
	noLoad := ch.Apply(ctx)
	nonBounded := ch.Apply(&routing.LBContext{Request: r, Route: route, LBEndpoints: route.LBEndpoints, Params: map[string]interface{}{}})

	if noLoad != nonBounded {
		t.Error("When no endpoints are overloaded, the chosen endpoint should be the same as standard consistentHash")
	}
	// now we know that noLoad is the endpoint which should be requested for somekey if load is not an issue.
	addInflightRequests(endpointRegistry, noLoad, 20)
	failover1 := ch.Apply(ctx)
	if failover1 == nonBounded {
		t.Error("When the selected endpoint is overloaded, the chosen endpoint should be different to standard consistentHash")
	}

	// now if 2 endpoints are overloaded, the request should go to the final endpoint
	addInflightRequests(endpointRegistry, failover1, 20)
	failover2 := ch.Apply(ctx)
	if failover2 == nonBounded || failover2 == failover1 {
		t.Error("Only the final endpoint had load below the average * balanceFactor, so it should have been selected.")
	}

	// now all will have same load, should select the original endpoint again
	addInflightRequests(endpointRegistry, failover2, 20)
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

	defaultEndpoint := ch.Apply(&routing.LBContext{Request: r, Route: rt, LBEndpoints: rt.LBEndpoints, Params: make(map[string]interface{})})
	remoteHostEndpoint := ch.Apply(&routing.LBContext{Request: r, Route: rt, LBEndpoints: rt.LBEndpoints, Params: map[string]interface{}{ConsistentHashKey: net.RemoteHost(r).String()}})

	if defaultEndpoint != remoteHostEndpoint {
		t.Error("remote host should be used as a default key")
	}

	for i, ep := range endpoints {
		key := fmt.Sprintf("%s-%d", ep, 1) // "ep-0" to "ep-99" is the range of keys for this endpoint. If we use this as the hash key it should select endpoint ep.
		selected := ch.Apply(&routing.LBContext{Request: r, Route: rt, LBEndpoints: rt.LBEndpoints, Params: map[string]interface{}{ConsistentHashKey: key}})
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

	ch := route.LBAlgorithm.(*consistentHash)
	balanceFactor := 1.25
	ctx := &routing.LBContext{
		Request:     r,
		Route:       route,
		LBEndpoints: route.LBEndpoints,
		Params:      map[string]interface{}{ConsistentHashBalanceFactor: balanceFactor},
	}
	endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	defer endpointRegistry.Close()
	endpointRegistry.Do([]*routing.Route{route})

	for i := 0; i < 100; i++ {
		ep := ch.Apply(ctx)
		ifr0 := route.LBEndpoints[0].Metrics.InflightRequests()
		ifr1 := route.LBEndpoints[1].Metrics.InflightRequests()
		ifr2 := route.LBEndpoints[2].Metrics.InflightRequests()

		assert.Equal(t, int64(ifr0), endpointRegistry.GetMetrics(route.LBEndpoints[0].Host).InflightRequests())
		assert.Equal(t, int64(ifr1), endpointRegistry.GetMetrics(route.LBEndpoints[1].Host).InflightRequests())
		assert.Equal(t, int64(ifr2), endpointRegistry.GetMetrics(route.LBEndpoints[2].Host).InflightRequests())

		avg := float64(ifr0+ifr1+ifr2) / 3.0
		limit := int64(avg*balanceFactor) + 1
		if ifr0 > limit || ifr1 > limit || ifr2 > limit {
			t.Errorf("Expected in-flight requests for each endpoint to be less than %d. In-flight request counts: %d, %d, %d", limit, ifr0, ifr1, ifr2)
		}
		endpointRegistry.GetMetrics(ep.Host).IncInflightRequest()
	}
}

func TestConsistentHashKeyDistribution(t *testing.T) {
	endpoints := []string{"http://10.2.0.1:8080", "http://10.2.0.2:8080", "http://10.2.0.3:8080", "http://10.2.0.4:8080", "http://10.2.0.5:8080", "http://10.2.0.6:8080", "http://10.2.0.7:8080", "http://10.2.0.8:8080", "http://10.2.0.9:8080", "http://10.2.0.10:8080"}

	stdDev1hashPerEndpoint := measureStdDev(endpoints, 1)
	stdDev100HashesPerEndpoint := measureStdDev(endpoints, 100)

	if stdDev100HashesPerEndpoint >= stdDev1hashPerEndpoint {
		t.Errorf("Standard deviation with 100 hashes per endpoint should be lower than with 1 hash per endpoint. 100 hashes: %f, 1 hash: %f", stdDev100HashesPerEndpoint, stdDev1hashPerEndpoint)
	}

	if stdDev100HashesPerEndpoint >= 10 { // arbitrary target to flag accidental breaking changes. Currently is 5.93
		t.Errorf("Standard deviation was too high for 100 vnodes, got %f", stdDev100HashesPerEndpoint)
	}
}

func addInflightRequests(registry *routing.EndpointRegistry, endpoint routing.LBEndpoint, count int) {
	for i := 0; i < count; i++ {
		endpoint.Metrics.IncInflightRequest()
		registry.GetMetrics(endpoint.Host).IncInflightRequest()
	}
}

// Measures how fair the hash ring is to each endpoint.
// i.e. Of the possible hashes, how many will go to each endpoint. The lower the standard deviation the better.
func measureStdDev(endpoints []string, hashesPerEndpoint int) float64 {
	ch := newConsistentHashInternal(endpoints, hashesPerEndpoint).(*consistentHash)
	ringOwnership := map[int]uint64{}
	prevPartitionEndHash := uint64(0)
	for i := 0; i < len(ch.hashRing); i++ {
		endpointIndex := ch.hashRing[i].index
		partitionEndHash := ch.hashRing[i].hash
		ringOwnership[endpointIndex] += partitionEndHash - prevPartitionEndHash
		prevPartitionEndHash = partitionEndHash
	}
	ringOwnership[ch.hashRing[0].index] += math.MaxUint64 - prevPartitionEndHash
	return stdDeviation(ringOwnership)
}

func stdDeviation(counters map[int]uint64) float64 {
	sum := uint64(0)
	for _, v := range counters {
		sum += v
	}
	mean := float64(sum) / float64(len(counters))
	summedDiffs := 0.0
	for _, v := range counters {
		diff := float64(v) - mean
		summedDiffs += diff * diff
	}
	stdDev := math.Sqrt(summedDiffs / float64(len(counters)))
	return (stdDev * 100) / mean
}

func BenchmarkRandomAlgorithm(b *testing.B) {
	N := 10
	eps := make([]string, N)
	j := 0
	k := 0
	for i := range N {
		j++
		if j > 255 {
			k++
			j = 0
		}
		eps[i] = fmt.Sprintf("10.0.%d.%d", k, j)
	}
	if k > 255 {
		b.Fatalf("Failed to benchmark: k > 255, k=%d", k)
	}

	alg := newRandom(eps)

	lbeps := make([]routing.LBEndpoint, len(eps))
	for i := range len(eps) {
		lbe := routing.LBEndpoint{
			Scheme:  "http",
			Host:    eps[i],
			Metrics: nil,
		}
		lbeps[i] = lbe
	}

	lbc := &routing.LBContext{
		LBEndpoints: lbeps,
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		alg.Apply(lbc)
	}
}
