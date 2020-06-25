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

func newRoundRobin(endpoints []string) routing.LBAlgorithm {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &roundRobin{
		index:      rnd.Intn(len(endpoints)),
		rnd:        rnd,
		fadeInCalc: make([]int, len(endpoints)),
	}
}

type roundRobin struct {
	mx         sync.Mutex
	index      int
	rnd        *rand.Rand
	fadeInCalc []int
}

func fadeInState(now time.Time, duration time.Duration, detected time.Time) (time.Duration, bool) {
	rel := now.Sub(detected)
	return rel, rel < duration
}

func fadeIn(now time.Time, duration time.Duration, ease float64, detected time.Time) float64 {
	rel, fadingIn := fadeInState(now, duration, detected)
	if !fadingIn {
		return 1
	}

	return math.Pow(float64(rel)/float64(duration), ease)
}

/*
func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, now time.Time, from int) routing.LBEndpoint {
	fd, fe, ep := ctx.Route.LBFadeInDuration, ctx.Route.LBFadeInEase, ctx.Route.LBEndpoints
	i, c := from, len(ep)
	for {
		i = (i + 1) % len(ep)
		c--
		if c == 1 {
			return ep[i]
		}

		f := fadeIn(now, fd, fe, ep[i].Detected)
		if chance(rnd, f/float64(c)) {
			return ep[i]
		}
	}
}
func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, w []float64, now time.Time, from int) routing.LBEndpoint {
	fd, fe, ep := ctx.Route.LBFadeInDuration, ctx.Route.LBFadeInEase, ctx.Route.LBEndpoints

	var (
		sum float64
		last int
	)

	i := from
	for {
		i = (i + 1) % len(ep)
		if i == from {
			break
		}

		f := fadeIn(now, fd, fe, ep[i].Detected)
		sum += f
		w[i] = sum
		last = i
	}

	r := sum * rand.Float64()
	i = from
	for {
		i = (i + 1) % len(ep)
		if i == last || r < w[i] {
			return ep[i]
		}
	}
}
*/
func shiftToRemaining(rnd *rand.Rand, ctx *routing.LBContext, w []int, now time.Time, from int) routing.LBEndpoint {
	fd, ep := ctx.Route.LBFadeInDuration, ctx.Route.LBEndpoints

	var (
		sum  int
		last int
	)

	i := from
	for {
		i = (i + 1) % len(ep)
		if i == from {
			break
		}

		if _, fadingIn := fadeInState(now, fd, ep[i].Detected); !fadingIn {
			sum++
		}

		w[i] = sum
		last = i
	}

	// this may not be good, because the endpoints can be in different stages of fading in
	// TODO: use an alternative in this case
	if sum == 0 {
		return ep[from]
	}

	r := rand.Intn(sum)
	i = from
	for {
		i = (i + 1) % len(ep)
		if i == last || r < w[i] {
			return ep[i]
		}
	}
}

func withFadeIn(rnd *rand.Rand, ctx *routing.LBContext, w []int, choice int) routing.LBEndpoint {
	now := time.Now()
	f := fadeIn(
		now,
		ctx.Route.LBFadeInDuration,
		ctx.Route.LBFadeInEase,
		ctx.Route.LBEndpoints[choice].Detected,
	)

	if rnd.Float64() < f {
		return ctx.Route.LBEndpoints[choice]
	}

	return shiftToRemaining(rnd, ctx, w, now, choice)
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
		rand:       rand.New(rand.NewSource(t)),
		fadeInCalc: make([]int, len(endpoints)),
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

type consistentHash struct {
	rnd        *rand.Rand
	fadeInCalc []int
}

func newConsistentHash(endpoints []string) routing.LBAlgorithm {
	return &consistentHash{
		rnd:        rand.New(rand.NewSource(time.Now().UnixNano())),
		fadeInCalc: make([]int, len(endpoints)),
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
		return ctx.Route.LBEndpoints[rand.Intn(len(ctx.Route.LBEndpoints))]
	}
	sum = h.Sum32()
	choice := int(sum) % len(ctx.Route.LBEndpoints)
	if choice < 0 {
		choice = len(ctx.Route.LBEndpoints) + choice
	}

	if ctx.Route.LBFadeInDuration <= 0 {
		return ctx.Route.LBEndpoints[choice]
	}

	return withFadeIn(ch.rnd, ctx, ch.fadeInCalc, choice)
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
