package loadbalancer

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/routing"
)

/*
algorithm
single endpoint
has fade-in duration
stabilizes after fade-in
*/

const (
	fadeInDuration = 100 * time.Millisecond
	bucketCount    = 20
)

func testFadeIn(
	t *testing.T,
	algorithm func([]string) routing.LBAlgorithm,
	endpointAges ...time.Duration,
) {
	var detectionTimes []time.Time
	now := time.Now()
	for _, ea := range endpointAges {
		detectionTimes = append(detectionTimes, now.Add(-ea))
	}

	var ep []string
	for i := range endpointAges {
		ep = append(ep, string('a'+i))
	}

	a := algorithm(ep)

	ctx := &routing.LBContext{
		Route: &routing.Route{
			LBFadeInDuration: fadeInDuration,
			LBFadeInEase:     1,
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
			// try with copy-on-write
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
	bucketSize := len(stats) / bucketCount
	for i := 0; i < bucketCount; i++ {
		bucketStats := make(map[string]int)
		for j := i * bucketSize; j < (i+1)*bucketSize; j++ {
			bucketStats[stats[j]]++
		}

		var showStats []string
		for _, epi := range ep {
			showStats = append(showStats, fmt.Sprintf("%s=%d", epi, bucketStats[epi]))
		}

		t.Log(strings.Join(showStats, ", "))
		// TODO: verify results
	}
}

func TestFadeIn(t *testing.T) {
	old := 2 * fadeInDuration
	// testFadeIn(t, newRoundRobin, , old)
	// testFadeIn(t, newRoundRobin, 0, 0)
	testFadeIn(t, newRoundRobin, old, 0)
	testFadeIn(t, newRoundRobin, old, old, old, 0)
	testFadeIn(t, newRoundRobin, old, old, old, 0, 0, 0)
	testFadeIn(t, newRoundRobin, old, 0, 0, 0)

	// testFadeIn(t, newRandom, old, old)
	// testFadeIn(t, newRandom, 0, 0)
	// testFadeIn(t, newRandom, old, 0)
	// testFadeIn(t, newRandom, old, 0, 0, 0)
	// testFadeIn(t, newRandom, old, 0, 0, 0, 0, 0, 0)
	// testFadeIn(t, newRandom, 0, 0, 0, 0, 0, 0)
}

// TODO: test consistent hash separately
