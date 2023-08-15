package loadbalancer

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

const (
	fadeInDuration     = 500 * time.Millisecond
	fadeInDurationHuge = 24 * time.Hour // we need this to be sure we're at the very beginning of fading in
	bucketCount        = 20
	monotonyTolerance  = 0.4 // we need to use a high tolerance for CI testing
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

func initializeEndpoints(endpointAges []time.Duration, fadeInDuration time.Duration) (*routing.LBContext, []string) {
	var detectionTimes []time.Time
	now := time.Now()
	for _, ea := range endpointAges {
		detectionTimes = append(detectionTimes, now.Add(-ea))
	}

	var eps []string
	for i := range endpointAges {
		eps = append(eps, fmt.Sprintf("ep-%d-%s.test", i, endpointAges[i]))
	}

	route := routing.NewRoute(eskip.Route{})
	route.LBFadeInDuration = fadeInDuration
	route.LBFadeInExponent = 1
	ctx := &routing.LBContext{
		Params: map[string]interface{}{},
		Route:  route,
	}

	for i := range eps {
		ctx.Route.LBEndpoints = append(ctx.Route.LBEndpoints, routing.LBEndpoint{
			Host:     eps[i],
			Detected: detectionTimes[i],
		})
	}

	return ctx, eps
}

func testFadeIn(
	t *testing.T,
	name string,
	algorithm func([]string) routing.LBAlgorithm,
	endpointAges ...time.Duration,
) {
	t.Run(name, func(t *testing.T) {
		ctx, eps := initializeEndpoints(endpointAges, fadeInDuration)

		a := algorithm(eps)
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		t.Log("test start", time.Now())
		var stats []string
		stop := time.After(fadeInDuration)
		// Emulate the load balancer loop, sending requests to it with random hash keys
		// over and over again till fadeIn period is over.
		func() {
			for {
				ctx.Params[ConsistentHashKey] = strconv.Itoa(rnd.Intn(100000))
				ep := a.Apply(ctx)
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
		for i := 0; i < bucketCount; i++ {
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

func newConsistentHashForTest(endpoints []string) routing.LBAlgorithm {
	// The default parameter 100 is too small to get even distribution
	return newConsistentHashInternal(endpoints, 1000)
}

func TestFadeIn(t *testing.T) {
	old := 2 * fadeInDuration
	testFadeIn(t, "round-robin, 0", newRoundRobin, old, old)
	testFadeIn(t, "round-robin, 1", newRoundRobin, 0, old)
	testFadeIn(t, "round-robin, 2", newRoundRobin, 0, 0)
	testFadeIn(t, "round-robin, 3", newRoundRobin, old, 0)
	testFadeIn(t, "round-robin, 4", newRoundRobin, old, old, old, 0)
	testFadeIn(t, "round-robin, 5", newRoundRobin, old, old, old, 0, 0, 0)
	testFadeIn(t, "round-robin, 6", newRoundRobin, old, 0, 0, 0)
	testFadeIn(t, "round-robin, 7", newRoundRobin, old, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "round-robin, 8", newRoundRobin, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "round-robin, 9", newRoundRobin, fadeInDuration/2, fadeInDuration/3, fadeInDuration/4)

	testFadeIn(t, "random, 0", newRandom, old, old)
	testFadeIn(t, "random, 1", newRandom, 0, old)
	testFadeIn(t, "random, 2", newRandom, 0, 0)
	testFadeIn(t, "random, 3", newRandom, old, 0)
	testFadeIn(t, "random, 4", newRandom, old, old, old, 0)
	testFadeIn(t, "random, 5", newRandom, old, old, old, 0, 0, 0)
	testFadeIn(t, "random, 6", newRandom, old, 0, 0, 0)
	testFadeIn(t, "random, 7", newRandom, old, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "random, 8", newRandom, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "random, 9", newRandom, fadeInDuration/2, fadeInDuration/3, fadeInDuration/4)

	testFadeIn(t, "consistent-hash, 0", newConsistentHashForTest, old, old)
	testFadeIn(t, "consistent-hash, 1", newConsistentHashForTest, 0, old)
	testFadeIn(t, "consistent-hash, 2", newConsistentHashForTest, 0, 0)
	testFadeIn(t, "consistent-hash, 3", newConsistentHashForTest, old, 0)
	testFadeIn(t, "consistent-hash, 4", newConsistentHashForTest, old, old, old, 0)
	testFadeIn(t, "consistent-hash, 5", newConsistentHashForTest, old, old, old, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 6", newConsistentHashForTest, old, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 7", newConsistentHashForTest, old, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 8", newConsistentHashForTest, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 9", newConsistentHashForTest, fadeInDuration/2, fadeInDuration/3, fadeInDuration/4)
}

// func testFadeInLoadBetweenOldEps(
// 	t *testing.T,
// 	name string,
// 	algorithm func([]string) routing.LBAlgorithm,
// 	nOld int, nNew int,
// ) {
// 	t.Run(name, func(t *testing.T) {
// 		const (
// 			numberOfReqs            = 100000
// 			acceptableErrorNearZero = 10
// 			old                     = fadeInDurationHuge
// 			new                     = time.Duration(0)
// 		)
// 		endpointAges := []time.Duration{}
// 		for i := 0; i < nOld; i++ {
// 			endpointAges = append(endpointAges, old)
// 		}
// 		for i := 0; i < nNew; i++ {
// 			endpointAges = append(endpointAges, new)
// 		}

// 		ctx, eps := initializeEndpoints(endpointAges, fadeInDurationHuge)

// 		a := algorithm(eps)
// 		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
// 		nReqs := map[string]int{}

// 		t.Log("test start", time.Now())
// 		// Emulate the load balancer loop, sending requests to it with random hash keys
// 		// over and over again till fadeIn period is over.
// 		for i := 0; i < numberOfReqs; i++ {
// 			ctx.Params[ConsistentHashKey] = strconv.Itoa(rnd.Intn(100000))
// 			ep := a.Apply(ctx)
// 			nReqs[ep.Host]++
// 		}

// 		expectedReqsPerOldEndpoint := numberOfReqs / nOld
// 		for idx, ep := range eps {
// 			if endpointAges[idx] == old {
// 				assert.InEpsilon(t, expectedReqsPerOldEndpoint, nReqs[ep], 0.2)
// 			}
// 			if endpointAges[idx] == new {
// 				assert.InDelta(t, 0, nReqs[ep], acceptableErrorNearZero)
// 			}
// 		}
// 	})
// }

// func TestFadeInLoadBetweenOldEps(t *testing.T) {
// 	for nOld := 1; nOld < 6; nOld++ {
// 		for nNew := 0; nNew < 6; nNew++ {
// 			testFadeInLoadBetweenOldEps(t, fmt.Sprintf("consistent-hash, %d old, %d new", nOld, nNew), newConsistentHash, nOld, nNew)
// 			testFadeInLoadBetweenOldEps(t, fmt.Sprintf("random, %d old, %d new", nOld, nNew), newRandom, nOld, nNew)
// 			testFadeInLoadBetweenOldEps(t, fmt.Sprintf("round-robin, %d old, %d new", nOld, nNew), newRoundRobin, nOld, nNew)
// 		}
// 	}
// }

func testApplyEndsWhenAllEndpointsAreFading(
	t *testing.T,
	name string,
	algorithm func([]string) routing.LBAlgorithm,
	nEndpoints int,
) {
	t.Run(name, func(t *testing.T) {
		// Initialize every endpoint with zero: every endpoint is new
		endpointAges := make([]time.Duration, nEndpoints)

		ctx, eps := initializeEndpoints(endpointAges, fadeInDurationHuge)

		a := algorithm(eps)
		ctx.Params[ConsistentHashKey] = "someConstantString"
		applied := make(chan struct{})

		go func() {
			a.Apply(ctx)
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

func TestApplyEndsWhenAllEndpointsAreFading(t *testing.T) {
	for nEndpoints := 1; nEndpoints < 10; nEndpoints++ {
		testApplyEndsWhenAllEndpointsAreFading(t, "consistent-hash", newConsistentHash, nEndpoints)
		testApplyEndsWhenAllEndpointsAreFading(t, "random", newRandom, nEndpoints)
		testApplyEndsWhenAllEndpointsAreFading(t, "round-robin", newRoundRobin, nEndpoints)
	}
}

func benchmarkFadeIn(
	b *testing.B,
	name string,
	algorithm func([]string) routing.LBAlgorithm,
	clients int,
	endpointAges ...time.Duration,
) {
	b.Run(name, func(b *testing.B) {
		var detectionTimes []time.Time
		now := time.Now()
		for _, ea := range endpointAges {
			detectionTimes = append(detectionTimes, now.Add(-ea))
		}

		var eps []string
		for i := range endpointAges {
			eps = append(eps, string('a'+rune(i)))
		}

		a := algorithm(eps)

		route := routing.NewRoute(eskip.Route{})
		route.LBFadeInDuration = fadeInDuration
		route.LBFadeInExponent = 1
		for i := range eps {
			route.LBEndpoints = append(route.LBEndpoints, routing.LBEndpoint{
				Host:     eps[i],
				Detected: detectionTimes[i],
			})
		}

		var wg sync.WaitGroup

		// Emulate the load balancer loop, sending requests to it with random hash keys
		// over and over again till fadeIn period is over.
		b.ResetTimer()
		for i := 0; i < clients; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
				ctx := &routing.LBContext{
					Params: map[string]interface{}{},
					Route:  route,
				}

				for j := 0; j < b.N/clients; j++ {
					ctx.Params[ConsistentHashKey] = strconv.Itoa(rnd.Intn(100000))
					_ = a.Apply(ctx)
				}
			}(i)
		}

		wg.Wait()
	})
}

func repeatedSlice(v time.Duration, n int) []time.Duration {
	var s []time.Duration
	for i := 0; i < n; i++ {
		s = append(s, v)
	}
	return s
}

func BenchmarkFadeIn(b *testing.B) {
	old := 2 * fadeInDuration
	clients := []int{1, 4, 16, 64, 256}
	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("random, 11, %d clients", c), newRandom, c, repeatedSlice(old, 200)...)
	}

	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("round-robin, 11, %d clients", c), newRoundRobin, c, repeatedSlice(old, 200)...)
	}

	for _, c := range clients {
		benchmarkFadeIn(b, fmt.Sprintf("consistent-hash, 11, %d clients", c), newConsistentHash, c, repeatedSlice(old, 200)...)
	}
}
