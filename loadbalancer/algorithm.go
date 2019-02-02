package loadbalancer

import (
	"errors"
	"math/rand"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type algorithmType int

const (
	none algorithmType = iota
	roundRobinAlgorithm
)

type roundRobin struct {
	mx    sync.Mutex
	index int
}

type algorithmProvider struct{}

var algorithms = map[algorithmType]func(int) routing.LBAlgorithm{
	roundRobinAlgorithm: newRoundRobin,
}

var defaultAlgorithm = newRoundRobin

func newRoundRobin(initialMax int) routing.LBAlgorithm {
	return &roundRobin{
		index: rand.Intn(initialMax),
	}
}

func (r *roundRobin) Apply(endpoints []routing.LBEndpoint) routing.LBEndpoint {
	r.mx.Lock()
	defer r.mx.Unlock()
	r.index = (r.index + 1) % len(endpoints)
	return endpoints[r.index]
}

func NewAlgorithmProvider() routing.PostProcessor {
	return &algorithmProvider{}
}

func algorithmTypeFromString(a string) (algorithmType, error) {
	switch a {
	case "":
		return none, nil
	case "roundRobin":
		return roundRobinAlgorithm, nil
	default:
		return none, errors.New("unsupported algorithm")
	}
}

func parseEndpoints(r *routing.Route) error {
	r.LBEndpoints = make([]routing.LBEndpoint, len(r.Route.LBEndpoints))
	for i, e := range r.Route.LBEndpoints {
		eu, err := url.ParseRequestURI(e)
		if err != nil {
			return err
		}

		r.LBEndpoints[i] = routing.LBEndpoint{Scheme: eu.Scheme, Host: eu.Host}
	}

	return nil
}

func setAlgorithm(r *routing.Route) error {
	t, err := algorithmTypeFromString(r.Route.LBAlgorithm)
	if err != nil {
		return err
	}

	a := defaultAlgorithm
	if t != none {
		a = algorithms[t]
	}

	r.LBAlgorithm = a(len(r.LBEndpoints))
	return nil
}

// Do implements routing.PostProcessor
func (p *algorithmProvider) Do(r []*routing.Route) []*routing.Route {
	var rr []*routing.Route
	for _, ri := range r {
		if ri.Route.BackendType != eskip.LBBackend {
			rr = append(rr, ri)
			continue
		}

		if len(ri.LBEndpoints) == 0 {
			log.Errorf("failed to post-process LB route: %s, no endpoints defined", ri.Id)
		}

		if err := parseEndpoints(ri); err != nil {
			log.Errorf("failed to parse LB endpoints for route %s: %v", ri.Id, err)
			continue
		}

		if err := setAlgorithm(ri); err != nil {
			log.Errorf("failed to set LB algorithm implementation for route %s: %v", ri.Id, err)
			continue
		}

		rr = append(rr, ri)
	}

	return rr
}
