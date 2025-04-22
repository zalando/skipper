package ratelimit

import (
	"context"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
)

// Swarmer interface defines the requirement for a Swarm, for use as
// an exchange method for cluster ratelimits:
// ratelimit.ClusterServiceRatelimit and
// ratelimit.ClusterClientRatelimit.
type Swarmer interface {
	// ShareValue is used to share the local information with its peers.
	ShareValue(string, interface{}) error
	// Values is used to get global information about current rates.
	Values(string) map[string]interface{}
}

// clusterLimitSwim stores all data required for the cluster ratelimit.
type clusterLimitSwim struct {
	group   string
	local   limiter
	maxHits int
	window  time.Duration
	swarm   Swarmer
	resize  chan resizeLimit
	quit    chan struct{}
}

type resizeLimit struct {
	s string
	n int
}

// newClusterRateLimiter creates a new clusterLimitSwim for given Settings
// and use the given Swarmer. Group is used in log messages to identify
// the ratelimit instance and has to be the same in all skipper instances.
func newClusterRateLimiterSwim(s Settings, sw Swarmer, group string) *clusterLimitSwim {
	rl := &clusterLimitSwim{
		group:   group,
		swarm:   sw,
		maxHits: s.MaxHits,
		window:  s.TimeWindow,
		resize:  make(chan resizeLimit),
		quit:    make(chan struct{}),
	}
	switch s.Type {
	case ClusterServiceRatelimit:
		log.Infof("new backend clusterRateLimiter")
		rl.local = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	case ClusterClientRatelimit:
		log.Infof("new client clusterRateLimiter")
		rl.local = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	default:
		log.Errorf("Unknown ratelimit type: %s", s.Type)
		return nil
	}

	// TODO(sszuecs): we might want to have one goroutine for all of these
	go func() {
		for {
			select {
			case size := <-rl.resize:
				log.Debugf("%s resize clusterRatelimit: %v", group, size)
				// TODO(sszuecs): call with "go" ?
				rl.Resize(size.s, rl.maxHits/size.n)
			case <-rl.quit:
				log.Debugf("%s: received quit signal, closing resize channel", group)
				close(rl.resize)
				log.Debugf("%s: quit clusterRatelimit, resize channel closed", group)
				return
			}
		}
	}()

	return rl
}

// Allow returns true if the request with context calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimitSwim) Allow(ctx context.Context, clearText string) bool {
	s := getHashedKey(clearText)
	key := swarmPrefix + c.group + "." + s

	// t0 is the oldest entry in the local circularbuffer
	// [ t3, t4, t0, t1, t2]
	//           ^- current pointer to oldest
	// now - t0
	t0 := c.Oldest(s).UTC().UnixNano()

	_ = c.local.Allow(ctx, s) // update local rate limit

	if err := c.swarm.ShareValue(key, t0); err != nil {
		log.Errorf("clusterRatelimit '%s' disabled, failed to share value: %v", c.group, err)
		return true // unsafe to continue otherwise
	}

	swarmValues := c.swarm.Values(key)
	log.Debugf("%s: clusterRatelimit swarmValues(%d) for '%s': %v", c.group, len(swarmValues), swarmPrefix+s, swarmValues)

	select {
	case c.resize <- resizeLimit{s: s, n: len(swarmValues)}:
	default:
		log.Warnf("%s: could not send resize message, resize channel blocked or closed", c.group)
	}

	now := time.Now().UTC().UnixNano()
	rate := c.calcTotalRequestRate(now, swarmValues)
	result := rate < float64(c.maxHits)
	log.Debugf("%s clusterRatelimit: Allow=%v, %v < %d", c.group, result, rate, c.maxHits)
	return result
}

func (c *clusterLimitSwim) calcTotalRequestRate(now int64, swarmValues map[string]interface{}) float64 {
	var requestRate float64
	// Avoid division by zero if swarmValues is empty
	nodeCount := len(swarmValues)
	if nodeCount == 0 {
		log.Warnf("%s: calcTotalRequestRate called with zero swarm values", c.group)
		return 0
	}

	maxNodeHits := math.Max(1.0, float64(c.maxHits)/float64(nodeCount))

	for _, v := range swarmValues {
		t0, ok := v.(int64)
		if !ok || t0 == 0 {
			continue
		}
		delta := time.Duration(now - t0)
		// Avoid division by zero or negative delta if time issues occur
		if delta <= 0 {
			log.Warnf("%s: Invalid time delta %v detected", c.group, delta)
			requestRate += maxNodeHits
			continue
		}
		// Avoid division by zero if c.window is zero (shouldn't happen with proper config)
		if c.window <= 0 {
			log.Errorf("%s: Invalid zero ratelimit window", c.group)
			requestRate += maxNodeHits
			continue
		}

		adjusted := float64(delta) / float64(c.window)
		// Avoid division by zero if adjusted time is zero (delta < window resolution?)
		if adjusted <= 0 {
			log.Warnf("%s: Adjusted time window is zero or negative (%v), delta=%v, window=%v", c.group, adjusted, delta, c.window)
			requestRate += maxNodeHits
			continue
		}

		log.Debugf("%s: %0.2f += %0.2f / %0.2f", c.group, requestRate, maxNodeHits, adjusted)
		requestRate += maxNodeHits / adjusted
	}
	log.Debugf("%s requestRate: %0.2f", c.group, requestRate)
	return requestRate
}

// Close should be called to teardown the clusterLimitSwim.
func (c *clusterLimitSwim) Close() {
	// Signal the goroutine to quit
	close(c.quit)

	// Close the underlying local limiter
	c.local.Close()
	log.Infof("%s: clusterLimitSwim closed", c.group)
}

func (c *clusterLimitSwim) Delta(s string) time.Duration { return c.local.Delta(s) }
func (c *clusterLimitSwim) Oldest(s string) time.Time    { return c.local.Oldest(s) }
func (c *clusterLimitSwim) Resize(s string, n int)       { c.local.Resize(s, n) }
func (c *clusterLimitSwim) RetryAfter(s string) int      { return c.local.RetryAfter(s) }
