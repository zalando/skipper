package loadbalancer

import (
	"errors"
	"hash/fnv"
	"math/rand"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
)

type algorithmType int

const (
	none algorithmType = iota
	roundRobinAlgorithm
	randomAlgorithm
	consistentHashAlgorithm
)

var (
	algorithms = map[algorithmType]initializeAgorithm{
		roundRobinAlgorithm:     newRoundRobin,
		randomAlgorithm:         newRandom,
		consistentHashAlgorithm: newConsistentHash,
	}
	defaultAlgorithm = newRoundRobin
)

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	i := time.Now().UnixNano()
	rand.Seed(i)
	return &roundRobin{
		index: rand.Intn(len(endpoints)),
	}
}

type roundRobin struct {
	mx    sync.Mutex
	index int
}

// Apply implements routing.LBAlgorithm with a roundrobin algorithm.
func (r *roundRobin) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	r.mx.Lock()
	defer r.mx.Unlock()
	r.index = (r.index + 1) % len(ctx.Route.LBEndpoints)
	return ctx.Route.LBEndpoints[r.index]
}

type random struct {
	rand *rand.Rand
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	t := time.Now().UnixNano()
	return &random{rand: rand.New(rand.NewSource(t))}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	return ctx.Route.LBEndpoints[r.rand.Intn(len(ctx.Route.LBEndpoints))]
}

type consistentHash struct{}

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	return &consistentHash{}
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (*consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	var sum uint32
	h := fnv.New32()

	key := net.RemoteHost(ctx.Request).String()
	if _, err := h.Write([]byte(key)); err != nil {
		log.Errorf("Failed to write '%s' into hash: %v", key, err)
		return ctx.Route.LBEndpoints[0]
	}
	sum = h.Sum32()
	choice := int(sum) % len(ctx.Route.LBEndpoints)
	if choice < 0 {
		choice = len(ctx.Route.LBEndpoints) + choice
	}
	return ctx.Route.LBEndpoints[choice]
}

type (
	algorithmProvider  struct{}
	initializeAgorithm func(endpoints []string) routing.LBAlgorithm
)

// NewAlgorithmProvider creates a routing.PostProcessor used to initialize
// the algorithm of load balancing routes.
func NewAlgorithmProvider() routing.PostProcessor {
	return &algorithmProvider{}
}

func algorithmTypeFromString(a string) (algorithmType, error) {
	switch a {
	case "":
		// This means that the user didn't explicitly specify which
		// algorithm should be used, and we will use a default one.
		return none, nil
	case "roundRobin":
		return roundRobinAlgorithm, nil
	case "random":
		return randomAlgorithm, nil
	case "consistentHash":
		return consistentHashAlgorithm, nil
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

	initialize := defaultAlgorithm
	if t != none {
		initialize = algorithms[t]
	}

	r.LBAlgorithm = initialize(r.Route.LBEndpoints)
	return nil
}

// Do implements routing.PostProcessor
func (p *algorithmProvider) Do(r []*routing.Route) []*routing.Route {
	rr := make([]*routing.Route, 0, len(r))
	for _, ri := range r {
		if ri.Route.BackendType != eskip.LBBackend {
			rr = append(rr, ri)
			continue
		}

		if len(ri.Route.LBEndpoints) == 0 {
			log.Errorf("failed to post-process LB route: %s, no endpoints defined", ri.Id)
			continue
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
