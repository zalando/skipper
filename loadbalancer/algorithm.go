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
)

var (
	algorithms = map[Algorithm]initializeAgorithm{
		RoundRobin:     newRoundRobin,
		Random:         newRandom,
		ConsistentHash: newConsistentHash,
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

func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, w []int, now time.Time) routing.LBEndpoint {
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
	return ep[notFadingIndexes[rnd.Intn(len(notFadingIndexes))]]
}

func withFadeIn(rnd *rand.Rand, ctx *routing.LBContext, w []int, choice int) routing.LBEndpoint {
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

	return shiftToRemaining(rnd, ctx, w, now)
}

type roundRobin struct {
	mx         sync.Mutex
	index      int
	rnd        *rand.Rand
	fadeInCalc []int
}

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &roundRobin{
		index: rnd.Intn(len(endpoints)),
		rnd:   rnd,

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

	return withFadeIn(r.rnd, ctx, r.fadeInCalc, r.index)
}

type random struct {
	rand       *rand.Rand
	fadeInCalc []int
}

func newRandom(endpoints []string) routing.LBAlgorithm {
	t := time.Now().UnixNano()
	return &random{
		rand: rand.New(rand.NewSource(t)),

		// preallocating frequently used slice
		fadeInCalc: make([]int, 0, len(endpoints)),
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

	return withFadeIn(r.rand, ctx, r.fadeInCalc, i)
}

type consistentHash struct{}

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	return &consistentHash{}
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
		return ctx.Route.LBEndpoints[rand.Intn(len(ctx.Route.LBEndpoints))]
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

		r.LBEndpoints[i] = routing.LBEndpoint{Scheme: eu.Scheme, Host: eu.Host}
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
