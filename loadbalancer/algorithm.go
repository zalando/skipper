package loadbalancer

import (
	"errors"
	"hash/fnv"
	"math/rand"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
)

// Algorithm indicates the used load balancing algorithm.
type Algorithm int

const (
	// None is the default non-specified algorithm.
	None Algorithm = iota

	// RoundRobin indicates round-robin load balancing between the backend endpoints.
	RoundRobin

	// Random indicates random choice between the backend endpoints.
	Random

	// ConsistentHash indicates choice between the backends based on their hashed address.
	ConsistentHash

	// PowerOfChoices selects N random endpoints and picks the one with least outstanding requests from them.
	PowerOfChoices
)

var (
	algorithms = map[Algorithm]initializeAlgorithm{
		RoundRobin:     newRoundRobin,
		Random:         newRandom,
		ConsistentHash: newConsistentHash,
		PowerOfChoices: newPowerOfChoices,
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
	r.index = (r.index + 1) % ctx.Route.LBEndpoints.Length()
	return ctx.Route.LBEndpoints.At(r.index)
}

type random struct {
	rand *rand.Rand
	mutex sync.Mutex
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	t := time.Now().UnixNano()
	return &random{rand: rand.New(rand.NewSource(t))}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	r.mutex.Lock()
	index := r.rand.Intn(ctx.Route.LBEndpoints.Length())
	r.mutex.Unlock()
	return ctx.Route.LBEndpoints.At(index)
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
		return ctx.Route.LBEndpoints.At(rand.Intn(ctx.Route.LBEndpoints.Length()))
	}
	sum = h.Sum32()
	choice := int(sum) % ctx.Route.LBEndpoints.Length()
	if choice < 0 {
		choice = ctx.Route.LBEndpoints.Length() + choice
	}
	return ctx.Route.LBEndpoints.At(choice)
}

type powerOfChoices struct{
	rand *rand.Rand
	numberOfChoices int
	mutex sync.Mutex
}

// Selects N random backends and picks the one with less outstanding requests.
func newPowerOfChoices(endpoints []string) routing.LBAlgorithm {
	t := time.Now().UnixNano()
	return &powerOfChoices{
		rand: rand.New(rand.NewSource(t)),
		numberOfChoices: 2, // TODO: make this the value part of skipper configuration.
	}
}
func (a *powerOfChoices) GetRandomChoice(length int) int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.rand.Intn(length)
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (a *powerOfChoices) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	chosen := make([]int, 0, a.numberOfChoices)
	for i := 0 ; i < a.numberOfChoices ; i++  {
		candidate := a.GetRandomChoice(ctx.Route.LBEndpoints.Length())
		// Pick a different endpoint if was already selected
		for contains(chosen, candidate) {
			candidate = a.GetRandomChoice(ctx.Route.LBEndpoints.Length())
		}
		chosen = append(chosen, candidate)
	}
	bestEndpoint := ctx.Route.LBEndpoints.At(chosen[0])
	bestScore := bestEndpoint.Metrics.GetInflightRequests()
	for _, endpointIdx := range chosen {
		endpoint := ctx.Route.LBEndpoints.At(endpointIdx)
		inflight := endpoint.Metrics.GetInflightRequests()
		if inflight < bestScore {
			bestEndpoint = endpoint
		}
	}
	return bestEndpoint
}

type (
	algorithmProvider   struct{}
	initializeAlgorithm func(endpoints []string) routing.LBAlgorithm
)

// NewAlgorithmProvider creates a routing.PostProcessor used to initialize
// the algorithm of load balancing routes.
func NewAlgorithmProvider() routing.PostProcessor {
	return &algorithmProvider{}
}


// AlgorithmFromString parses the string representation of the algorithm definition.
func AlgorithmFromString(a string) (Algorithm, error) {
	switch a {
	case "":
		// This means that the user didn't explicitly specify which
		// algorithm should be used, and we will use a default one.
		return None, nil
	case "roundRobin":
		return RoundRobin, nil
	case "random":
		return Random, nil
	case "consistentHash":
		return ConsistentHash, nil
	case "powerOfChoices":
		return PowerOfChoices, nil
	default:
		return None, errors.New("unsupported algorithm")
	}
}

// String returns the string representation of an algorithm definition.
func (a Algorithm) String() string {
	switch a {
	case RoundRobin:
		return "roundRobin"
	case Random:
		return "random"
	case ConsistentHash:
		return "consistentHash"
	case PowerOfChoices:
		return "powerOfChoices"
	default:
		return ""
	}
}

func parseEndpoints(r *routing.Route) error {
	err, endpoints := routing.NewEndpointCollection(r)
	if err != nil {
		return err
	}
	r.LBEndpoints = endpoints
	return nil
}

func setAlgorithm(r *routing.Route) error {
	t, err := AlgorithmFromString(r.Route.LBAlgorithm)
	if err != nil {
		return err
	}

	initialize := defaultAlgorithm
	if t != None {
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
