package loadbalancer

import (
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
			rr[0].LBEndpoints[0].Host != "www.example.org" {
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
			rr[0].LBEndpoints[0].Host != "www.example.org" {
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
			rr[0].LBEndpoints[0].Host != "www.example.org" {
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

		if len(rr[0].LBEndpoints) != 1 ||
			rr[0].LBEndpoints[0].Scheme != "https" ||
			rr[0].LBEndpoints[0].Host != "www.example.org" {
			t.Fatal("failed to set the endpoints")
		}

		if _, ok := rr[0].LBAlgorithm.(*random); !ok {
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
