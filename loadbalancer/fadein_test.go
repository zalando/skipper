package loadbalancer

import (
	"fmt"
	"github.com/zalando/skipper/routing"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	fadeInDuration    = 100 * time.Millisecond
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

		hashKeys := findHashKeys(a, ctx)
		t.Log("test start", time.Now())
		var stats []string
		stop := time.After(fadeInDuration)
		func() {
			for {
				ctx.Params[ConsistentHashKey] = hashKeys[len(stats)%len(hashKeys)]
				ep := a.Apply(ctx)
				stats = append(stats, ep.Host)
				select {
				case <-stop:
					return
				default:
				}
			}
		}()

		t.Log("test done", time.Now())
		t.Log("CSV " + strings.Join(eps, ","))
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
					t.Error("non-monotonic change", epi)
				}
			}
		}

		for _, bucketStats := range allBuckets {
			var showStats []string
			for _, epi := range eps {
				showStats = append(showStats, fmt.Sprintf("%d", bucketStats[epi]))
			}

			t.Log("CSV " + strings.Join(showStats, ","))
		}
	})
}

// For each endpoint, return a hash key which will make the consistent hash algorithm select it.
// This allows the test to emulate round robin, useful for showing the increase in requests to each endpoint is monotonic.
func findHashKeys(a routing.LBAlgorithm, ctx *routing.LBContext) []string {
	// temporarily disable fadein
	ctx.Route.LBFadeInDuration = 0
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	var hashKeys []string
	for _, ep := range ctx.Route.LBEndpoints {
		for {
			ctx.Params[ConsistentHashKey] = strconv.Itoa(rnd.Intn(1000))
			if ep == a.Apply(ctx) {
				hashKeys = append(hashKeys, ctx.Params[ConsistentHashKey].(string))
				break
			}
		}
	}
	delete(ctx.Params, ConsistentHashKey)
	ctx.Route.LBFadeInDuration = fadeInDuration
	return hashKeys
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
