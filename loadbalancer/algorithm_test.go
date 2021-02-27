package loadbalancer

import (
	"fmt"
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
		key := ep // key equal to endpoint has the same hash and therefore selects it
		selected := ch.Apply(&routing.LBContext{Request: r, Route: rt, Params: map[string]interface{}{ConsistentHashKey: key}})
		if selected != rt.LBEndpoints[i] {
			t.Errorf("expected: %v, got %v", rt.LBEndpoints[i], selected)
		}
	}
}
