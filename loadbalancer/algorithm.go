package loadbalancer

import (
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"net/url"
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

func shiftToOldest(ctx *routing.LBContext) routing.LBEndpoint {
	ep := ctx.Route.LBEndpoints
	oldest := ep[0]
	for _, epi := range ep[1:] {
		if epi.Detected.Before(oldest.Detected) {
			oldest = epi
		}
	}

	return oldest
}

func shiftToRemaining(ctx *routing.LBContext, sr *safeRand, w []int, now time.Time) routing.LBEndpoint {
	notFadingIndexes := w
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
		return shiftToOldest(ctx)
	}

	// otherwise equally distribute between the old endpoints
	return ep[notFadingIndexes[sr.getIntn(len(notFadingIndexes))]]
}

func withFadeIn(ctx *routing.LBContext, sr *safeRand, w []int, choice int) routing.LBEndpoint {
	now := time.Now()
	f := fadeIn(
		now,
		ctx.Route.LBFadeInDuration,
		ctx.Route.LBFadeInExponent,
		ctx.Route.LBEndpoints[choice].Detected,
	)

	if sr.getFloat64() < f {
		return ctx.Route.LBEndpoints[choice]
	}

	return shiftToRemaining(ctx, sr, w, now)
}

// safeRand is a struct for managing concurrent safe random access
// with the custom source.
type safeRand struct {
	rnd *rand.Rand
	mx  sync.Mutex
}

func newSafeRand() *safeRand {
	src := rand.NewSource(time.Now().UnixNano())
	return &safeRand{
		rnd: rand.New(src),
	}
}

// getIntn returns random int within the given range.
func (s *safeRand) getIntn(lbe int) int {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.rnd.Intn(lbe)
}

// getFloat64 returns random float64
func (s *safeRand) getFloat64() float64 {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.rnd.Float64()
}

type roundRobin struct {
	mx         sync.Mutex
	index      int
	sr         *safeRand
	fadeInCalc []int
}

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	sr := newSafeRand()
	return &roundRobin{
		index: sr.getIntn(len(endpoints)),
		sr:    sr,

		// preallocating frequently used slice
		fadeInCalc: make([]int, 0, len(endpoints)),
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

	return withFadeIn(ctx, r.sr, r.fadeInCalc, r.index)
}

type random struct {
	sr         *safeRand
	fadeInCalc []int
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	return &random{
		sr: newSafeRand(),
		// preallocating frequently used slice
		fadeInCalc: make([]int, 0, len(endpoints)),
	}
}

// Apply implements routing.LBAlgorithm with a stateless random algorithm.
func (r *random) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	i := r.sr.getIntn(len(ctx.Route.LBEndpoints))
	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[i]
	}

	return withFadeIn(ctx, r.sr, r.fadeInCalc, i)
}

type consistentHash struct {
	sr *safeRand
}

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	return &consistentHash{
		sr: newSafeRand(),
	}
}

// Apply implements routing.LBAlgorithm with a consistent hash algorithm.
func (ch *consistentHash) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	if len(ctx.Route.LBEndpoints) == 1 {
		return ctx.Route.LBEndpoints[0]
	}

	var sum uint32
	h := fnv.New32()

	key := net.RemoteHost(ctx.Request).String()
	if _, err := h.Write([]byte(key)); err != nil {
		log.Errorf("Failed to write '%s' into hash: %v", key, err)
		return ctx.Route.LBEndpoints[ch.sr.getIntn(len(ctx.Route.LBEndpoints))]
	}
	sum = h.Sum32()
	choice := int(sum) % len(ctx.Route.LBEndpoints)
	if choice < 0 {
		choice = len(ctx.Route.LBEndpoints) + choice
	}

	return ctx.Route.LBEndpoints[choice]
}

type powerOfRandomNChoices struct {
	sr              *safeRand
	numberOfChoices int
}

// newPowerOfRandomNChoices selects N random backends and picks the one with less outstanding requests.
func newPowerOfRandomNChoices(endpoints []string) routing.LBAlgorithm {
	return &powerOfRandomNChoices{
		sr:              newSafeRand(),
		numberOfChoices: 2,
	}
}

// Apply implements routing.LBAlgorithm with power of random N choices algorithm.
func (p *powerOfRandomNChoices) Apply(ctx *routing.LBContext) routing.LBEndpoint {
	ne := len(ctx.Route.LBEndpoints)

	best := ctx.Route.LBEndpoints[p.sr.getIntn(ne)]

	for i := 1; i < p.numberOfChoices; i++ {
		ce := ctx.Route.LBEndpoints[p.sr.getIntn(ne)]

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
