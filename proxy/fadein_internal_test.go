package proxy

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/routing"
)

const (
	fadeInRequestCount = 300_000
	bucketCount        = 20
	monotonyTolerance  = 0.4 // we need to use a high tolerance for CI testing

	defaultFadeInDuration     = 500 * time.Millisecond
	defaultFadeInDurationHuge = 24 * time.Hour // we need this to be sure we're at the very beginning of fading in
)

func absint(i int) int {
	if i < 0 {
		return -i
	}

	return i
}

func tolerance(prev, next int) int {
	return int(float64(prev+next) * monotonyTolerance / 2)
}

func checkMonotony(direction, prev, next int) bool {
	t := tolerance(prev, next)
	switch direction {
	case 1:
		return next-prev >= -t
	case -1:
		return next-prev <= t
	default:
		return absint(next-prev) < t
	}
}

func multiply(k float64, d time.Duration) time.Duration {
	return time.Duration(k * float64(d))
}

func initializeEndpoints(endpointAges []float64, algorithmName string, fadeInDuration time.Duration) (*routing.Route, *Proxy, []string) {
	var detectionTimes []time.Time
	now := time.Now()
	for _, ea := range endpointAges {
		endpointAgeDuration := multiply(ea, fadeInDuration)
		detectionTimes = append(detectionTimes, now.Add(-endpointAgeDuration))
	}

	var eps []string
	for i, ea := range endpointAges {
		endpointAgeDuration := multiply(ea, fadeInDuration)
		eps = append(eps, fmt.Sprintf("http://ep-%d-%s.test", i, endpointAgeDuration))
	}

	registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
	eskipRoute := eskip.Route{BackendType: eskip.LBBackend, LBAlgorithm: algorithmName}
	for i := range eps {
		eskipRoute.LBEndpoints = append(eskipRoute.LBEndpoints, eps[i])
		registry.GetMetrics(eps[i]).SetDetected(detectionTimes[i])
	}

	route := &routing.Route{
		Route:            eskipRoute,
		LBFadeInDuration: fadeInDuration,
		LBFadeInExponent: 1,
		LBEndpoints:      []routing.LBEndpoint{},
	}

	rt := loadbalancer.NewAlgorithmProvider().Do([]*routing.Route{route})
	route = rt[0]
	registry.Do([]*routing.Route{route})

	eps = []string{}
	for i, ea := range endpointAges {
		endpointAgeDuration := multiply(ea, fadeInDuration)
		eps = append(eps, fmt.Sprintf("ep-%d-%s.test:80", i, endpointAgeDuration))
		registry.GetMetrics(eps[i]).SetDetected(detectionTimes[i])
	}

	proxy := &Proxy{registry: registry, fadein: &fadeIn{rnd: rand.New(rand.NewPCG(0, 0))}, quit: make(chan struct{})}
	return route, proxy, eps
}

// This function is needed to calculate the duration needed to send fadeInRequestCount requests.
// It is needed to send number of requests close to fadeInRequestCount on one hand and to have them sent exactly
// in the fade-in duration returned on the other hand.
func calculateFadeInDuration(t *testing.T, algorithmName string, endpointAges []float64) time.Duration {
	const precalculateRatio = 10

	route, proxy, _ := initializeEndpoints(endpointAges, algorithmName, defaultFadeInDuration)
	defer proxy.Close()

	randGen := rand.New(rand.NewPCG(0, 0))

	t.Log("preemulation start", time.Now())
	// Preemulate the load balancer loop to find out the approximate amount of RPS
	begin := time.Now()
	for range fadeInRequestCount / precalculateRatio {
		_ = proxy.selectEndpoint(&context{route: route, request: &http.Request{}, stateBag: map[string]interface{}{loadbalancer.ConsistentHashKey: strconv.Itoa(randGen.IntN(100000))}})
	}
	preemulationDuration := time.Since(begin)

	return preemulationDuration * precalculateRatio
}

func testFadeInMonotony(
	t *testing.T,
	name string,
	algorithmName string,
	endpointAges ...float64,
) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		randGen := rand.New(rand.NewPCG(0, 0))
		fadeInDuration := calculateFadeInDuration(t, algorithmName, endpointAges)
		route, proxy, eps := initializeEndpoints(endpointAges, algorithmName, fadeInDuration)
		defer proxy.Close()

		t.Log("test start", time.Now())
		var stats []string
		stop := time.After(fadeInDuration)
		// Emulate the load balancer loop, sending requests to it with random hash keys
		// over and over again till fadeIn period is over.
		func() {
			for {
				ep := proxy.selectEndpoint(&context{route: route, request: &http.Request{}, stateBag: map[string]interface{}{loadbalancer.ConsistentHashKey: strconv.Itoa(randGen.IntN(100000))}})
				stats = append(stats, ep.Host)
				select {
				case <-stop:
					return
				default:
				}
			}
		}()

		// Split fade-in period into buckets and count how many times each endpoint was selected.
		t.Log("test done", time.Now())
		t.Log("CSV timestamp," + strings.Join(eps, ","))
		bucketSize := len(stats) / bucketCount
		var allBuckets []map[string]int
		for i := range bucketCount {
			bucketStats := make(map[string]int)
			for j := i * bucketSize; j < (i+1)*bucketSize; j++ {
				bucketStats[stats[j]]++
			}

			allBuckets = append(allBuckets, bucketStats)
		}

		directions := make(map[string]int)
		for _, epi := range eps {
			first := allBuckets[0][epi]
			last := allBuckets[len(allBuckets)-1][epi]
			t := tolerance(first, last)
			switch {
			case last-first > t:
				directions[epi] = 1
			case last-first < t:
				directions[epi] = -1
			}
		}

		for i := range allBuckets {
			// trim first and last (warmup and settling)
			if i < 2 || i == len(allBuckets)-1 {
				continue
			}

			for _, epi := range eps {
				if !checkMonotony(
					directions[epi],
					allBuckets[i-1][epi],
					allBuckets[i][epi],
				) {
					t.Error("non-monotonic change", epi, i)
				}
			}
		}

		for i, bucketStats := range allBuckets {
			var showStats []string
			for _, epi := range eps {
				showStats = append(showStats, fmt.Sprintf("%d", bucketStats[epi]))
			}

			// Print CSV-like output for, where row number represents time and
			// column represents endpoint. You can visualize it using
			// ./skptesting/run_fadein_test.sh from the skipper repo root.
			t.Log("CSV " + fmt.Sprintf("%d,", i) + strings.Join(showStats, ","))
		}
	})
}

// Those tests check that the amount of requests per period for each endpoint is monotonical over the time.
// For every endpoint, it could increase, decrease or stay the same.
func TestFadeInMonotony(t *testing.T) {
	old := 2.0
	testFadeInMonotony(t, "power-of-n-random-choices, 0", loadbalancer.PowerOfRandomNChoices.String(), old, old)
	testFadeInMonotony(t, "power-of-n-random-choices, 1", loadbalancer.PowerOfRandomNChoices.String(), 0, old)
	testFadeInMonotony(t, "power-of-n-random-choices, 2", loadbalancer.PowerOfRandomNChoices.String(), 0, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 3", loadbalancer.PowerOfRandomNChoices.String(), old, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 4", loadbalancer.PowerOfRandomNChoices.String(), old, old, old, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 5", loadbalancer.PowerOfRandomNChoices.String(), old, old, old, 0, 0, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 6", loadbalancer.PowerOfRandomNChoices.String(), old, 0, 0, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 7", loadbalancer.PowerOfRandomNChoices.String(), old, 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 8", loadbalancer.PowerOfRandomNChoices.String(), 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "power-of-n-random-choices, 9", loadbalancer.PowerOfRandomNChoices.String(), 1.0/2.0, 1.0/3.0, 1.0/4.0)

	testFadeInMonotony(t, "round-robin, 0", loadbalancer.RoundRobin.String(), old, old)
	testFadeInMonotony(t, "round-robin, 1", loadbalancer.RoundRobin.String(), 0, old)
	testFadeInMonotony(t, "round-robin, 2", loadbalancer.RoundRobin.String(), 0, 0)
	testFadeInMonotony(t, "round-robin, 3", loadbalancer.RoundRobin.String(), old, 0)
	testFadeInMonotony(t, "round-robin, 4", loadbalancer.RoundRobin.String(), old, old, old, 0)
	testFadeInMonotony(t, "round-robin, 5", loadbalancer.RoundRobin.String(), old, old, old, 0, 0, 0)
	testFadeInMonotony(t, "round-robin, 6", loadbalancer.RoundRobin.String(), old, 0, 0, 0)
	testFadeInMonotony(t, "round-robin, 7", loadbalancer.RoundRobin.String(), old, 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "round-robin, 8", loadbalancer.RoundRobin.String(), 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "round-robin, 9", loadbalancer.RoundRobin.String(), 1.0/2.0, 1.0/3.0, 1.0/4.0)

	testFadeInMonotony(t, "random, 0", loadbalancer.Random.String(), old, old)
	testFadeInMonotony(t, "random, 1", loadbalancer.Random.String(), 0, old)
	testFadeInMonotony(t, "random, 2", loadbalancer.Random.String(), 0, 0)
	testFadeInMonotony(t, "random, 3", loadbalancer.Random.String(), old, 0)
	testFadeInMonotony(t, "random, 4", loadbalancer.Random.String(), old, old, old, 0)
	testFadeInMonotony(t, "random, 5", loadbalancer.Random.String(), old, old, old, 0, 0, 0)
	testFadeInMonotony(t, "random, 6", loadbalancer.Random.String(), old, 0, 0, 0)
	testFadeInMonotony(t, "random, 7", loadbalancer.Random.String(), old, 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "random, 8", loadbalancer.Random.String(), 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "random, 9", loadbalancer.Random.String(), 1.0/2.0, 1.0/3.0, 1.0/4.0)

	testFadeInMonotony(t, "consistent-hash, 0", loadbalancer.ConsistentHash.String(), old, old)
	testFadeInMonotony(t, "consistent-hash, 1", loadbalancer.ConsistentHash.String(), 0, old)
	testFadeInMonotony(t, "consistent-hash, 2", loadbalancer.ConsistentHash.String(), 0, 0)
	testFadeInMonotony(t, "consistent-hash, 3", loadbalancer.ConsistentHash.String(), old, 0)
	testFadeInMonotony(t, "consistent-hash, 4", loadbalancer.ConsistentHash.String(), old, old, old, 0)
	testFadeInMonotony(t, "consistent-hash, 5", loadbalancer.ConsistentHash.String(), old, old, old, 0, 0, 0)
	testFadeInMonotony(t, "consistent-hash, 6", loadbalancer.ConsistentHash.String(), old, 0, 0, 0)
	testFadeInMonotony(t, "consistent-hash, 7", loadbalancer.ConsistentHash.String(), old, 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "consistent-hash, 8", loadbalancer.ConsistentHash.String(), 0, 0, 0, 0, 0, 0)
	testFadeInMonotony(t, "consistent-hash, 9", loadbalancer.ConsistentHash.String(), 1.0/2.0, 1.0/3.0, 1.0/4.0)
}

func testFadeInLoadBetweenOldAndNewEps(
	t *testing.T,
	name string,
	algorithmName string,
	nOld int, nNew int,
) {
	t.Run(name, func(t *testing.T) {
		randGen := rand.New(rand.NewPCG(uint64(0), 0))
		const (
			numberOfReqs            = 100000
			acceptableErrorNearZero = 10
			old                     = 1.0
			new                     = 0.0
		)
		endpointAges := []float64{}
		for range nOld {
			endpointAges = append(endpointAges, old)
		}
		for range nNew {
			endpointAges = append(endpointAges, new)
		}

		route, proxy, eps := initializeEndpoints(endpointAges, algorithmName, defaultFadeInDurationHuge)
		defer proxy.Close()
		nReqs := map[string]int{}

		t.Log("test start", time.Now())
		// Emulate the load balancer loop, sending requests to it with random hash keys
		// over and over again till fadeIn period is over.
		for range numberOfReqs {
			ep := proxy.selectEndpoint(&context{route: route, request: &http.Request{}, stateBag: map[string]interface{}{loadbalancer.ConsistentHashKey: strconv.Itoa(randGen.IntN(100000))}})
			nReqs[ep.Host]++
		}

		if nOld == 0 {
			expectedReqsPerEndpoint := numberOfReqs / nNew
			for _, ep := range eps {
				assert.InEpsilon(t, expectedReqsPerEndpoint, nReqs[ep], 0.2)
			}
		} else {
			expectedReqsPerOldEndpoint := numberOfReqs / nOld
			for idx, ep := range eps {
				if endpointAges[idx] == old {
					assert.InEpsilon(t, expectedReqsPerOldEndpoint, nReqs[ep], 0.2)
				}
				if endpointAges[idx] == new {
					assert.InDelta(t, 0, nReqs[ep], acceptableErrorNearZero)
				}
			}
		}
	})
}

// Those tests check that the amount of requests per period for every endpoint at the very beginning of fading in (when all endpoints are new)
// and at the very end of fading in (when all endpoints are old) is correct.
func TestFadeInLoadBetweenOldAndNewEps(t *testing.T) {
	for nOld := range 6 {
		for nNew := range 6 {
			if nOld == 0 && nNew == 0 {
				continue
			}

			testFadeInLoadBetweenOldAndNewEps(t, fmt.Sprintf("power-of-n-random-choices, %d old, %d new", nOld, nNew), loadbalancer.PowerOfRandomNChoices.String(), nOld, nNew)
			testFadeInLoadBetweenOldAndNewEps(t, fmt.Sprintf("consistent-hash, %d old, %d new", nOld, nNew), loadbalancer.ConsistentHash.String(), nOld, nNew)
			testFadeInLoadBetweenOldAndNewEps(t, fmt.Sprintf("random, %d old, %d new", nOld, nNew), loadbalancer.Random.String(), nOld, nNew)
			testFadeInLoadBetweenOldAndNewEps(t, fmt.Sprintf("round-robin, %d old, %d new", nOld, nNew), loadbalancer.RoundRobin.String(), nOld, nNew)
		}
	}
}

func testSelectEndpointEndsWhenAllEndpointsAreFading(
	t *testing.T,
	name string,
	algorithmName string,
	nEndpoints int,
) {
	t.Run(name, func(t *testing.T) {
		// Initialize every endpoint with zero: every endpoint is new
		endpointAges := make([]float64, nEndpoints)
		route, proxy, _ := initializeEndpoints(endpointAges, algorithmName, defaultFadeInDurationHuge)
		defer proxy.Close()
		applied := make(chan struct{})

		go func() {
			proxy.selectEndpoint(&context{route: route, request: &http.Request{}, stateBag: map[string]interface{}{loadbalancer.ConsistentHashKey: "someConstantString"}})
			close(applied)
		}()

		select {
		case <-time.After(time.Second):
			t.Errorf("a.Apply has not finished in a reasonable time")
		case <-applied:
			break
		}
	})
}

func TestSelectEndpointEndsWhenAllEndpointsAreFading(t *testing.T) {
	for nEndpoints := 1; nEndpoints < 10; nEndpoints++ {
		testSelectEndpointEndsWhenAllEndpointsAreFading(t, "power-of-n-random-choices", loadbalancer.PowerOfRandomNChoices.String(), nEndpoints)
		testSelectEndpointEndsWhenAllEndpointsAreFading(t, "consistent-hash", loadbalancer.ConsistentHash.String(), nEndpoints)
		testSelectEndpointEndsWhenAllEndpointsAreFading(t, "random", loadbalancer.Random.String(), nEndpoints)
		testSelectEndpointEndsWhenAllEndpointsAreFading(t, "round-robin", loadbalancer.RoundRobin.String(), nEndpoints)
	}
}

func benchmarkFadeIn(
	b *testing.B,
	name string,
	algorithmName string,
	clients int,
	endpointAges ...float64,
) {
	b.Run(name, func(b *testing.B) {
		randGen := rand.New(rand.NewPCG(uint64(0), 0))
		route, proxy, _ := initializeEndpoints(endpointAges, algorithmName, defaultFadeInDurationHuge)
		defer proxy.Close()
		var wg sync.WaitGroup

		// Emulate the load balancer loop, sending requests to it with random hash keys
		// over and over again till fadeIn period is over.
		b.ResetTimer()
		for i := range clients {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				for j := 0; j < b.N/clients; j++ {
					_ = proxy.selectEndpoint(&context{route: route, request: &http.Request{}, stateBag: map[string]interface{}{loadbalancer.ConsistentHashKey: strconv.Itoa(randGen.IntN(100000))}})
				}
			}(i)
		}

		wg.Wait()
	})
}

func repeatedSlice(v float64, n int) []float64 {
	var s []float64
	for range n {
		s = append(s, v)
	}
	return s
}

func BenchmarkFadeIn(b *testing.B) {
	old := 2.0
	clients := []int{1, 4, 16, 64, 256}
	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("power-of-n-random-choices, 11, %d clients", c), loadbalancer.PowerOfRandomNChoices.String(), c, repeatedSlice(old, 200)...)
	}

	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("random, 11, %d clients", c), loadbalancer.Random.String(), c, repeatedSlice(old, 200)...)
	}

	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("round-robin, 11, %d clients", c), loadbalancer.RoundRobin.String(), c, repeatedSlice(old, 200)...)
	}

	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("consistent-hash, 11, %d clients", c), loadbalancer.ConsistentHash.String(), c, repeatedSlice(old, 200)...)
	}
}
