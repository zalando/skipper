package loadbalancer

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

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

func shiftWeighted(rnd *rand.Rand, ctx *routing.LBContext, now time.Time) routing.LBEndpoint {
	var sum float64
	weightSums := []float64{}
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

func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, wi []int, now time.Time) routing.LBEndpoint {
	notFadingIndexes := wi
	ep := ctx.Route.LBEndpoints

	// if all endpoints are fading, the simplest approach is to use the oldest,
	// this departs from the desired curve, but guarantees monotonic fade-in. From
	// the perspective of the oldest endpoint, this is temporarily the same as if
	// there was no fade-in.
	if len(notFadingIndexes) == 0 {
		return shiftWeighted(rnd, ctx, now)
	}

	// otherwise equally distribute between the old endpoints
	return ep[notFadingIndexes[rnd.Intn(len(notFadingIndexes))]]
}

func withFadeIn(rnd *rand.Rand, ctx *routing.LBContext, choice int, algo routing.LBAlgorithm) routing.LBEndpoint {
	ep := ctx.Route.LBEndpoints
	now := time.Now()
	f := fadeIn(
		now,
		ctx.Route.LBFadeInDuration,
		ctx.Route.LBFadeInExponent,
		ctx.Route.LBEndpoints[choice].Detected,
	)

	if rnd.Float64() < f {
		return ep[choice]
	}
	notFadingIndexes := []int{}
	for i := 0; i < len(ep); i++ {
		if _, fadingIn := fadeInState(now, ctx.Route.LBFadeInDuration, ep[i].Detected); !fadingIn {
			notFadingIndexes = append(notFadingIndexes, i)
		}
	}

	switch a := algo.(type) {
	case *roundRobin:
		return shiftToRemaining(a.rnd, ctx, notFadingIndexes, now)
	case *random:
		return shiftToRemaining(a.rand, ctx, notFadingIndexes, now)
	case *consistentHash:
		// If all endpoints are fading, normal consistent hash result
		if len(notFadingIndexes) == 0 {
			return ep[choice]
		}
		// otherwise calculate consistent hash again using endpoints which are not fading
		return ep[a.chooseConsistentHashEndpoint(ctx, skipFadingEndpoints(notFadingIndexes))]
	default:
		return ep[choice]
	}
}

type roundRobin struct {
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

	index := int(atomic.AddInt64(&r.index, 1) % int64(len(ctx.Route.LBEndpoints)))

	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[index]
	}

	return withFadeIn(r.rnd, ctx, index, r)
}

type random struct {
	rand *rand.Rand
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	// #nosec
	return &random{
		rand: rand.New(newLockedSource()),
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

	return withFadeIn(r.rand, ctx, i, r)
}

type (
	endpointHash struct {
		index int    // index of endpoint in endpoint list
		hash  uint64 // hash of endpoint
	}
	consistentHash struct {
		hashRing []endpointHash // list of endpoints sorted by hash value
		rand     *rand.Rand
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
		rand:     rnd,
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
func (ch *consistentHash) searchRing(key string, skipEndpoint func(int) bool) int {
	h := hash(key)
	i := sort.Search(ch.Len(), func(i int) bool { return ch.hashRing[i].hash >= h && !skipEndpoint(ch.hashRing[i].index) })
	if i == ch.Len() { // rollover
		i = 0
	}
	return i
}

// Returns index of endpoint with closest hash to key's hash
func (ch *consistentHash) search(key string, skipEndpoint func(int) bool) int {
	ringIndex := ch.searchRing(key, skipEndpoint)
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
func (ch *consistentHash) boundedLoadSearch(key string, balanceFactor float64, ctx *routing.LBContext, skipEndpoint func(int) bool) int {
	ringIndex := ch.searchRing(key, skipEndpoint)
	averageLoad := computeLoadAverage(ctx)
	targetLoad := averageLoad * balanceFactor
	// Loop round ring, starting at endpoint with closest hash. Stop when we find one whose load is less than targetLoad.
	for i := 0; i < ch.Len(); i++ {
		endpointIndex := ch.hashRing[ringIndex].index
		if skipEndpoint(endpointIndex) {
			continue
		}
		load := ctx.Route.LBEndpoints[endpointIndex].Metrics.GetInflightRequests()
		// We know there must be an endpoint whose load <= average load.
		// Since targetLoad >= average load (balancerFactor >= 1), there must also be an endpoint with load <= targetLoad.
		if load <= int(targetLoad) {
			break
		}
		ringIndex = (ringIndex + 1) % ch.Len()
	}

	return ch.hashRing[ringIndex].index
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (ch *consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	choice := ch.chooseConsistentHashEndpoint(ctx, noSkippedEndpoints)

	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[choice]
	}

	return withFadeIn(ch.rand, ctx, choice, ch)
}

func (ch *consistentHash) chooseConsistentHashEndpoint(ctx *routing.LBContext, skipEndpoint func(int) bool) int {
	key, ok := ctx.Params[ConsistentHashKey].(string)
	if !ok {
		key = net.RemoteHost(ctx.Request).String()
	}
	balanceFactor, ok := ctx.Params[ConsistentHashBalanceFactor].(float64)
	var choice int
	if !ok {
		choice = ch.search(key, skipEndpoint)
	} else {
		choice = ch.boundedLoadSearch(key, balanceFactor, ctx, skipEndpoint)
	}

	return choice
}

func skipFadingEndpoints(notFadingEndpoints []int) func(int) bool {
	return func(i int) bool {
		for _, notFadingEndpoint := range notFadingEndpoints {
			if i == notFadingEndpoint {
				return false
			}
		}
		return true
	}
}

func noSkippedEndpoints(_ int) bool {
	return false
}

type powerOfRandomNChoices struct {
	mx              sync.Mutex
	rand            *rand.Rand
	numberOfChoices int
}

// newPowerOfRandomNChoices selects N random backends and picks the one with less outstanding requests.
func newPowerOfRandomNChoices([]string) routing.LBAlgorithm {
	rnd := rand.New(newLockedSource()) // #nosec
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
