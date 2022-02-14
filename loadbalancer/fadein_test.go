package loadbalancer

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/routing"
)

const (
	fadeInDuration    = 100 * time.Millisecond
	bucketCount       = 20
	monotonyTolerance = 0.3 // we need to use a high tolerance for CI testing
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

		var ep []string
		for i := range endpointAges {
			ep = append(ep, string('a'+rune(i)))
		}

		a := algorithm(ep)

		ctx := &routing.LBContext{
			Route: &routing.Route{
				LBFadeInDuration: fadeInDuration,
				LBFadeInExponent: 1,
			},
		}

		for i := range ep {
			ctx.Route.LBEndpoints = append(ctx.Route.LBEndpoints, routing.LBEndpoint{
				Host:     ep[i],
				Detected: detectionTimes[i],
			})
		}

		t.Log("test start", time.Now())
		var stats []string
		stop := time.After(fadeInDuration)
		func() {
			for {
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
		t.Log("CSV " + strings.Join(ep, ","))
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
		for _, epi := range ep {
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

			for _, epi := range ep {
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
			for _, epi := range ep {
				showStats = append(showStats, fmt.Sprintf("%d", bucketStats[epi]))
			}

			t.Log("CSV " + strings.Join(showStats, ","))
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
}
