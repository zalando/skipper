package loadbalancer

import (
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"net/url"
	"sort"
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

	// PowerOfRandomNChoices selects N random endpoints and picks the one with least outstanding requests from them.
	PowerOfRandomNChoices
)

const powerOfRandomNChoicesDefaultN = 2

const ConsistentHashKey = "consistentHashKey"

var (
	algorithms = map[Algorithm]initializeAlgorithm{
		RoundRobin:            newRoundRobin,
		Random:                newRandom,
		ConsistentHash:        newConsistentHash,
		PowerOfRandomNChoices: newPowerOfRandomNChoices,
	}
	defaultAlgorithm = newRoundRobin
)

func fadeInState(now time.Time, duration time.Duration, detected time.Time) (time.Duration, bool) {
	rel := now.Sub(detected)
	return rel, rel > 0 && rel < duration
}

func fadeIn(now time.Time, duration time.Duration, exponent float64, detected time.Time) float64 {
	rel, fadingIn := fadeInState(now, duration, detected)
	if !fadingIn {
		return 1
	}

	return math.Pow(float64(rel)/float64(duration), exponent)
}

func shiftWeighted(rnd *rand.Rand, ctx *routing.LBContext, w []float64, now time.Time) routing.LBEndpoint {
	var sum float64
	weightSums := w
	rt := ctx.Route
	ep := ctx.Route.LBEndpoints
	for _, epi := range ep {
		wi := fadeIn(now, rt.LBFadeInDuration, rt.LBFadeInExponent, epi.Detected)
		sum += wi
		weightSums = append(weightSums, sum)
	}

	choice := ep[len(weightSums)-1]
	r := rnd.Float64() * sum
	for i := range weightSums {
		if weightSums[i] > r {
			choice = ep[i]
			break
		}
	}

	return choice
}

func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, wi []int, wf []float64, now time.Time) routing.LBEndpoint {
	notFadingIndexes := wi
	ep := ctx.Route.LBEndpoints
	for i := 0; i < len(ep); i++ {
		if _, fadingIn := fadeInState(now, ctx.Route.LBFadeInDuration, ep[i].Detected); !fadingIn {
			notFadingIndexes = append(notFadingIndexes, i)
		}
	}

	// if all endpoints are fading, the simplest approach is to use the oldest,
	// this departs from the desired curve, but guarantees monotonic fade-in. From
	// the perspective of the oldest endpoint, this is temporarily the same as if
	// there was no fade-in.
	if len(notFadingIndexes) == 0 {
		return shiftWeighted(rnd, ctx, wf, now)
	}

	// otherwise equally distribute between the old endpoints
	return ep[notFadingIndexes[rnd.Intn(len(notFadingIndexes))]]
}

func withFadeIn(rnd *rand.Rand, ctx *routing.LBContext, wi []int, wf []float64, choice int) routing.LBEndpoint {
	now := time.Now()
	f := fadeIn(
		now,
		ctx.Route.LBFadeInDuration,
		ctx.Route.LBFadeInExponent,
		ctx.Route.LBEndpoints[choice].Detected,
	)

	if rnd.Float64() < f {
		return ctx.Route.LBEndpoints[choice]
	}

	return shiftToRemaining(rnd, ctx, wi, wf, now)
}

type roundRobin struct {
	mx               sync.Mutex
	index            int
	rnd              *rand.Rand
	notFadingIndexes []int
	fadingWeights    []float64
}

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec
	return &roundRobin{
		index: rnd.Intn(len(endpoints)),
		rnd:   rnd,

		// preallocating frequently used slice
		notFadingIndexes: make([]int, 0, len(endpoints)),
		fadingWeights:    make([]float64, 0, len(endpoints)),
	}
}

// Apply implements routing.LBAlgorithm with a roundrobin algorithm.
func (r *roundRobin) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	r.mx.Lock()
	defer r.mx.Unlock()
	r.index = (r.index + 1) % len(ctx.Route.LBEndpoints)

	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[r.index]
	}

	return withFadeIn(r.rnd, ctx, r.notFadingIndexes, r.fadingWeights, r.index)
}

type random struct {
	rand             *rand.Rand
	notFadingIndexes []int
	fadingWeights    []float64
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	t := time.Now().UnixNano()
	// #nosec
	return &random{
		rand: rand.New(rand.NewSource(t)),

		// preallocating frequently used slice
		notFadingIndexes: make([]int, 0, len(endpoints)),
		fadingWeights:    make([]float64, 0, len(endpoints)),
	}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	i := r.rand.Intn(len(ctx.Route.LBEndpoints))
	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[i]
	}

	return withFadeIn(r.rand, ctx, r.notFadingIndexes, r.fadingWeights, i)
}

type (
	endpointHash struct {
		index int    // index of endpoint in endpoint list
		hash  uint32 // hash of endpoint
	}
	consistentHash []endpointHash // list of endpoints sorted by hash value
)

func (ch consistentHash) Len() int           { return len(ch) }
func (ch consistentHash) Less(i, j int) bool { return ch[i].hash < ch[j].hash }
func (ch consistentHash) Swap(i, j int)      { ch[i], ch[j] = ch[j], ch[i] }

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	ch := consistentHash(make([]endpointHash, len(endpoints)))
	for i, ep := range endpoints {
		ch[i] = endpointHash{i, hash(ep)}
	}
	sort.Sort(ch)
	return ch
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// Returns index of endpoint with closest hash to key's hash
func (ch consistentHash) search(key string) int {
	h := hash(key)
	i := sort.Search(ch.Len(), func(i int) bool { return ch[i].hash >= h })
	if i == ch.Len() { // rollover
		i = 0
	}
	return ch[i].index
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (ch consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	key, ok := ctx.Params[ConsistentHashKey].(string)
	if !ok {
		key = net.RemoteHost(ctx.Request).String()
	}
	choice := ch.search(key)

	return ctx.Route.LBEndpoints[choice]
}

type powerOfRandomNChoices struct {
	mx              sync.Mutex
	rand            *rand.Rand
	numberOfChoices int
}

// newPowerOfRandomNChoices selects N random backends and picks the one with less outstanding requests.
func newPowerOfRandomNChoices(endpoints []string) routing.LBAlgorithm {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec
	return &powerOfRandomNChoices{
		rand:            rnd,
		numberOfChoices: powerOfRandomNChoicesDefaultN,
	}
}

// Apply implements routing.LBAlgorithm with power of random N choices algorithm.
func (p *powerOfRandomNChoices) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	ne := len(ctx.Route.LBEndpoints)

	p.mx.Lock()
	defer p.mx.Unlock()

	best := ctx.Route.LBEndpoints[p.rand.Intn(ne)]

	for i := 1; i < p.numberOfChoices; i++ {
		ce := ctx.Route.LBEndpoints[p.rand.Intn(ne)]

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
