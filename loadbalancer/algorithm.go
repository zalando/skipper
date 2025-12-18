package loadbalancer

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	snet "github.com/zalando/skipper/net"
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
	index int64
}

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	return &roundRobin{
		index: rand.Int64N(int64(len(endpoints))), // #nosec
	}
}

// Apply implements routing.LBAlgorithm with a roundrobin algorithm.
func (r *roundRobin) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.LBEndpoints) == 1 {
		return ctx.LBEndpoints[0]
	}

	choice := int(atomic.AddInt64(&r.index, 1) % int64(len(ctx.LBEndpoints))) // #nosec
	return ctx.LBEndpoints[choice]
}

type random struct {
	mu  sync.Mutex
	rnd *rand.Rand
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	// #nosec
	return &random{
		rnd: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.LBEndpoints) == 1 {
		return ctx.LBEndpoints[0]
	}

	r.mu.Lock()
	choice := r.rnd.IntN(len(ctx.LBEndpoints)) // #nosec
	r.mu.Unlock()
	return ctx.LBEndpoints[choice]
}

type (
	endpointHash struct {
		index int    // index of endpoint in endpoint list
		hash  uint64 // hash of endpoint
	}
	consistentHash struct {
		hashRing []endpointHash // list of endpoints sorted by hash value
	}
)

func (ch *consistentHash) Len() int           { return len(ch.hashRing) }
func (ch *consistentHash) Less(i, j int) bool { return ch.hashRing[i].hash < ch.hashRing[j].hash }
func (ch *consistentHash) Swap(i, j int) {
	ch.hashRing[i], ch.hashRing[j] = ch.hashRing[j], ch.hashRing[i]
}

func newConsistentHashInternal(endpoints []string, hashesPerEndpoint int) routing.LBAlgorithm {
	ch := &consistentHash{
		hashRing: make([]endpointHash, hashesPerEndpoint*len(endpoints)),
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

func skipEndpoint(c *routing.LBContext, index int) bool {
	host := c.Route.LBEndpoints[index].Host
	for i := range c.LBEndpoints {
		if c.LBEndpoints[i].Host == host {
			return false
		}
	}
	return true
}

// Returns index in hash ring with the closest hash to key's hash
func (ch *consistentHash) searchRing(key string, ctx *routing.LBContext) int {
	h := hash(key)
	i := sort.Search(ch.Len(), func(i int) bool { return ch.hashRing[i].hash >= h })
	if i == ch.Len() { // rollover
		i = 0
	}
	for skipEndpoint(ctx, ch.hashRing[i].index) {
		i = (i + 1) % ch.Len()
	}
	return i
}

// Returns index of endpoint with closest hash to key's hash
func (ch *consistentHash) search(key string, ctx *routing.LBContext) int {
	ringIndex := ch.searchRing(key, ctx)
	return ch.hashRing[ringIndex].index
}

func computeLoadAverage(ctx *routing.LBContext) float64 {
	sum := 1.0 // add 1 to include the request that just arrived
	endpoints := ctx.LBEndpoints
	for _, v := range endpoints {
		sum += float64(v.Metrics.InflightRequests())
	}
	return sum / float64(len(endpoints))
}

// Returns index of endpoint with closest hash to key's hash, which is also below the target load
// skipEndpoint function is used to skip endpoints we don't want, for example, fading endpoints
func (ch *consistentHash) boundedLoadSearch(key string, balanceFactor float64, ctx *routing.LBContext) int {
	ringIndex := ch.searchRing(key, ctx)
	averageLoad := computeLoadAverage(ctx)
	targetLoad := averageLoad * balanceFactor
	// Loop round ring, starting at endpoint with closest hash. Stop when we find one whose load is less than targetLoad.
	for i := 0; i < ch.Len(); i++ {
		endpointIndex := ch.hashRing[ringIndex].index
		if skipEndpoint(ctx, endpointIndex) {
			continue
		}
		load := ctx.Route.LBEndpoints[endpointIndex].Metrics.InflightRequests()
		// We know there must be an endpoint whose load <= average load.
		// Since targetLoad >= average load (balancerFactor >= 1), there must also be an endpoint with load <= targetLoad.
		if float64(load) <= targetLoad {
			break
		}
		ringIndex = (ringIndex + 1) % ch.Len()
	}

	return ch.hashRing[ringIndex].index
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (ch *consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.LBEndpoints) == 1 {
		return ctx.LBEndpoints[0]
	}

	// The index returned from this call is taken from hash ring which is built from data about
	// all endpoints, including fading in, unhealthy, etc. ones. The index stored in hash ring is
	// the index of the endpoint in the original list of endpoints.
	choice := ch.chooseConsistentHashEndpoint(ctx)
	return ctx.Route.LBEndpoints[choice]
}

func (ch *consistentHash) chooseConsistentHashEndpoint(ctx *routing.LBContext) int {
	key, ok := ctx.Params[ConsistentHashKey].(string)
	if !ok {
		key = snet.RemoteHost(ctx.Request).String()
	}
	balanceFactor, ok := ctx.Params[ConsistentHashBalanceFactor].(float64)
	var choice int
	if !ok {
		choice = ch.search(key, ctx)
	} else {
		choice = ch.boundedLoadSearch(key, balanceFactor, ctx)
	}

	return choice
}

type powerOfRandomNChoices struct {
	mu              sync.Mutex
	rnd             *rand.Rand
	numberOfChoices int
}

// newPowerOfRandomNChoices selects N random backends and picks the one with less outstanding requests.
func newPowerOfRandomNChoices([]string) routing.LBAlgorithm {
	return &powerOfRandomNChoices{
		rnd:             rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)), // #nosec
		numberOfChoices: powerOfRandomNChoicesDefaultN,
	}
}

// Apply implements routing.LBAlgorithm with power of random N choices algorithm.
func (p *powerOfRandomNChoices) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	ne := len(ctx.LBEndpoints)

	p.mu.Lock()
	defer p.mu.Unlock()

	best := ctx.LBEndpoints[p.rnd.IntN(ne)] // #nosec

	for i := 1; i < p.numberOfChoices; i++ {
		ce := ctx.LBEndpoints[p.rnd.IntN(ne)] // #nosec

		if p.getScore(ce) > p.getScore(best) {
			best = ce
		}
	}
	return best
}

// getScore returns negative value of inflightrequests count.
func (p *powerOfRandomNChoices) getScore(e routing.LBEndpoint) int64 {
	// endpoints with higher inflight request should have lower score
	return -int64(e.Metrics.InflightRequests())
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
		scheme, host, err := snet.SchemeHost(e)
		if err != nil {
			return err
		}

		r.LBEndpoints[i] = routing.LBEndpoint{
			Scheme: scheme,
			Host:   host,
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
