package loadbalancer

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
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
		if len(rr) != 1 || rr[0].LBEndpoints != nil || rr[0].LBAlgorithm != nil {
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

		if rr[0].LBEndpoints.Length() != 1 ||
			rr[0].LBEndpoints.At(0).Scheme != "https" ||
			rr[0].LBEndpoints.At(0).Host != "www.example.org" {
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

		if rr[0].LBEndpoints.Length() != 1 ||
			rr[0].LBEndpoints.At(0).Scheme != "https" ||
			rr[0].LBEndpoints.At(0).Host != "www.example.org" {
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

		if rr[0].LBEndpoints.Length() != 1 ||
			rr[0].LBEndpoints.At(0).Scheme != "https" ||
			rr[0].LBEndpoints.At(0).Host != "www.example.org" {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*consistentHash); !ok {
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

		if rr[0].LBEndpoints.Length() != 1 ||
			rr[0].LBEndpoints.At(0).Scheme != "https" ||
			rr[0].LBEndpoints.At(0).Host != "www.example.org" {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*random); !ok {
			t.Fatal("failed to set the right algorithm")
		}
	})

	t.Run("LB route with explicit powerOfChoices algorithm", func(t *testing.T) {
		p := NewAlgorithmProvider()
		r := &routing.Route{
			Route: eskip.Route{
				BackendType: eskip.LBBackend,
				LBAlgorithm: "powerOfChoices",
				LBEndpoints: []string{"https://www.example.org"},
			},
		}

		rr := p.Do([]*routing.Route{r})
		if len(rr) != 1 {
			t.Fatal("failed to process LB route")
		}

		if rr[0].LBEndpoints.Length() != 1 ||
			rr[0].LBEndpoints.At(0).Scheme != "https" ||
			rr[0].LBEndpoints.At(0).Host != "www.example.org" {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*powerOfChoices); !ok {
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
			name:          "powerOfChoices algorithm",
			iterations:    100,
			jitter:        0,
			expected:      N,
			algorithm:     newPowerOfChoices(eps),
			algorithmName: "powerOfChoices",
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
