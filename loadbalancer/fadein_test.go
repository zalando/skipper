package loadbalancer

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/routing"
)

const (
	fadeInDuration    = 500 * time.Millisecond
	bucketCount       = 20
	monotonyTolerance = 0.4 // we need to use a high tolerance for CI testing
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

func testFadeIn(
	t *testing.T,
	name string,
	algorithm func([]string) routing.LBAlgorithm,
	endpointAges ...time.Duration,
) {
	t.Run(name, func(t *testing.T) {
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

		ctx := &routing.LBContext{
			Params: map[string]interface{}{},
			Route: &routing.Route{
				LBFadeInDuration: fadeInDuration,
				LBFadeInExponent: 1,
			},
		}

		for i := range eps {
			ctx.Route.LBEndpoints = append(ctx.Route.LBEndpoints, routing.LBEndpoint{
				Host:     eps[i],
				Detected: detectionTimes[i],
			})
		}

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

	testFadeIn(t, "consistent-hash, 0", newConsistentHash, old, old)
	testFadeIn(t, "consistent-hash, 1", newConsistentHash, 0, old)
	testFadeIn(t, "consistent-hash, 2", newConsistentHash, 0, 0)
	testFadeIn(t, "consistent-hash, 3", newConsistentHash, old, 0)
	testFadeIn(t, "consistent-hash, 4", newConsistentHash, old, old, old, 0)
	testFadeIn(t, "consistent-hash, 5", newConsistentHash, old, old, old, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 6", newConsistentHash, old, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 7", newConsistentHash, old, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 8", newConsistentHash, 0, 0, 0, 0, 0, 0)
	testFadeIn(t, "consistent-hash, 9", newConsistentHash, fadeInDuration/2, fadeInDuration/3, fadeInDuration/4)
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

		route := &routing.Route{
			LBFadeInDuration: fadeInDuration,
			LBFadeInExponent: 1,
		}
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
