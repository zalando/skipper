package loadbalancer

import (
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"sync"

	"github.com/cespare/xxhash/v2"
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

	// PowerOfRandomNChoices selects N random endpoints and picks the one with least outstanding requests from them.
	PowerOfRandomNChoices
)

const powerOfRandomNChoicesDefaultN = 2
const (
	ConsistentHashKey           = "consistentHashKey"
	ConsistentHashBalanceFactor = "consistentHashBalanceFactor"
)

var (
	algorithms = map[Algorithm]initializeAlgorithm{
		RoundRobin:            newRoundRobin,
		Random:                newRandom,
		ConsistentHash:        newConsistentHash,
		PowerOfRandomNChoices: newPowerOfRandomNChoices,
	}
	defaultAlgorithm = newRoundRobin
)

type roundRobin struct {
	mx    sync.Mutex
	index int64
	rnd   *rand.Rand
}

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	rnd := rand.New(newLockedSource()) // #nosec
	return &roundRobin{
		index: int64(rnd.Intn(len(endpoints))),
		rnd:   rnd,
	}
}

// Apply implements routing.LBAlgorithm with a roundrobin algorithm.
func (r *roundRobin) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	endpoints := ctx.Route.GetHealthyEndpoints()

	r.mx.Lock()
	defer r.mx.Unlock()

	r.index = (r.index + 1) % int64(len(endpoints))
	return endpoints[r.index]
}

type random struct {
	rnd *rand.Rand
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	// #nosec
	return &random{
		rnd: rand.New(newLockedSource()),
	}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	endpoints := ctx.Route.GetHealthyEndpoints()
	i := r.rnd.Intn(len(endpoints))

	return endpoints[i]
}

type (
	endpointHash struct {
		index int    // index of endpoint in endpoint list
		hash  uint64 // hash of endpoint
	}
	consistentHash struct {
		hashRing []endpointHash // list of endpoints sorted by hash value
		rnd      *rand.Rand
	}
)

func (ch *consistentHash) Len() int           { return len(ch.hashRing) }
func (ch *consistentHash) Less(i, j int) bool { return ch.hashRing[i].hash < ch.hashRing[j].hash }
func (ch *consistentHash) Swap(i, j int) {
	ch.hashRing[i], ch.hashRing[j] = ch.hashRing[j], ch.hashRing[i]
}

func newConsistentHashInternal(endpoints []string, hashesPerEndpoint int) routing.LBAlgorithm {
	rnd := rand.New(newLockedSource()) // #nosec
	ch := &consistentHash{
		hashRing: make([]endpointHash, hashesPerEndpoint*len(endpoints)),
		rnd:      rnd,
	}
	for i, ep := range endpoints {
		endpointStartIndex := hashesPerEndpoint * i
		for j := 0; j < hashesPerEndpoint; j++ {
			ch.hashRing[endpointStartIndex+j] = endpointHash{i, hash(fmt.Sprintf("%s-%d", ep, j))}
		}
	}
	sort.Sort(ch)
	return ch
}

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	return newConsistentHashInternal(endpoints, 100)
}

func hash(s string) uint64 {
	return xxhash.Sum64String(s)
}

// Returns index in hash ring with the closest hash to key's hash
func (ch *consistentHash) searchRing(key string, ctx *routing.LBContext, healthyEndpoints map[routing.LBEndpoint]struct{}) int {
	h := hash(key)
	i := sort.Search(len(ch.hashRing), func(i int) bool { return ch.hashRing[i].hash >= h })
	if i == len(ch.hashRing) { // rollover
		i = 0
	}
	_, ok := healthyEndpoints[ctx.Route.LBEndpoints[ch.hashRing[i].index]]
	for !ok {
		i = (i + 1) % len(ch.hashRing)
		_, ok = healthyEndpoints[ctx.Route.LBEndpoints[ch.hashRing[i].index]]
	}
	return i
}

// Returns index of endpoint with closest hash to key's hash
func (ch *consistentHash) search(key string, ctx *routing.LBContext, healthyEndpoints map[routing.LBEndpoint]struct{}) int {
	ringIndex := ch.searchRing(key, ctx, healthyEndpoints)
	return ch.hashRing[ringIndex].index
}

func computeLoadAverage(ctx *routing.LBContext) float64 {
	sum := 1.0 // add 1 to include the request that just arrived
	endpoints := ctx.Route.LBEndpoints
	for _, v := range endpoints {
		sum += float64(v.Metrics.GetInflightRequests())
	}
	return sum / float64(len(endpoints))
}

// Returns index of endpoint with closest hash to key's hash, which is also below the target load
// skipEndpoint function is used to skip endpoints we don't want, such as fading endpoints
func (ch *consistentHash) boundedLoadSearch(key string, balanceFactor float64, ctx *routing.LBContext, healthyEndpoints map[routing.LBEndpoint]struct{}) int {
	ringIndex := ch.searchRing(key, ctx, healthyEndpoints)
	averageLoad := computeLoadAverage(ctx)
	targetLoad := averageLoad * balanceFactor
	// Loop round ring, starting at endpoint with closest hash. Stop when we find one whose load is less than targetLoad.
	for i := 0; i < len(ch.hashRing); i++ {
		endpointIndex := ch.hashRing[ringIndex].index
		_, ok := healthyEndpoints[ctx.Route.LBEndpoints[endpointIndex]]
		if !ok {
			continue
		}
		load := ctx.Route.LBEndpoints[endpointIndex].Metrics.GetInflightRequests()
		// We know there must be an endpoint whose load <= average load.
		// Since targetLoad >= average load (balancerFactor >= 1), there must also be an endpoint with load <= targetLoad.
		if load <= int(targetLoad) {
			break
		}
		ringIndex = (ringIndex + 1) % len(ch.hashRing)
	}

	return ch.hashRing[ringIndex].index
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (ch *consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	return ctx.Route.LBEndpoints[ch.chooseConsistentHashEndpoint(ctx, ctx.Route.GetHealthyEndpointsSet())]
}

func (ch *consistentHash) chooseConsistentHashEndpoint(
	ctx *routing.LBContext,
	healthyEndpoints map[routing.LBEndpoint]struct{},
) int {
	key, ok := ctx.Params[ConsistentHashKey].(string)
	if !ok {
		key = net.RemoteHost(ctx.Request).String()
	}
	balanceFactor, ok := ctx.Params[ConsistentHashBalanceFactor].(float64)
	var choice int
	if !ok {
		choice = ch.search(key, ctx, healthyEndpoints)
	} else {
		choice = ch.boundedLoadSearch(key, balanceFactor, ctx, healthyEndpoints)
	}

	return choice
}

type powerOfRandomNChoices struct {
	mx              sync.Mutex
	rnd             *rand.Rand
	numberOfChoices int
}

// newPowerOfRandomNChoices selects N random backends and picks the one with less outstanding requests.
func newPowerOfRandomNChoices([]string) routing.LBAlgorithm {
	rnd := rand.New(newLockedSource()) // #nosec
	return &powerOfRandomNChoices{
		rnd:             rnd,
		numberOfChoices: powerOfRandomNChoicesDefaultN,
	}
}

// Apply implements routing.LBAlgorithm with power of random N choices algorithm.
func (p *powerOfRandomNChoices) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	ne := len(ctx.Route.LBEndpoints)

	p.mx.Lock()
	defer p.mx.Unlock()

	best := ctx.Route.LBEndpoints[p.rnd.Intn(ne)]

	for i := 1; i < p.numberOfChoices; i++ {
		ce := ctx.Route.LBEndpoints[p.rnd.Intn(ne)]

		if p.getScore(ce) > p.getScore(best) {
			best = ce
		}
	}
	return best
}

// getScore returns negative value of inflightrequests count.
func (p *powerOfRandomNChoices) getScore(e routing.LBEndpoint) int {
	// endpoints with higher inflight request should have lower score
	return -e.Metrics.GetInflightRequests()
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
	case "powerOfRandomNChoices":
		return PowerOfRandomNChoices, nil
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
	case PowerOfRandomNChoices:
		return "powerOfRandomNChoices"
	default:
		return ""
	}
}

func parseEndpoints(r *routing.Route) error {
	r.LBEndpoints = make([]routing.LBEndpoint, len(r.Route.LBEndpoints))
	for i, e := range r.Route.LBEndpoints {
		eu, err := url.ParseRequestURI(e)
		if err != nil {
			return err
		}

		r.LBEndpoints[i] = routing.LBEndpoint{
			Scheme:  eu.Scheme,
			Host:    eu.Host,
			Metrics: &routing.LBMetrics{},
		}
	}

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
