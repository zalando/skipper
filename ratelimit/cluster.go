package ratelimit

import (
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
	"github.com/zalando/skipper/metrics"
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
	// Members returns the number of known members
	Members() int
}

// clusterLimit stores all data required for the cluster ratelimit.
type clusterLimit struct {
	group   string
	local   limiter
	maxHits int
	window  time.Duration
	swarm   Swarmer
	metrics metrics.Metrics
	resize  chan resizeLimit
	quit    chan struct{}
}

type resizeLimit struct {
	s string
	n int
}

// newClusterRateLimiter creates a new clusterLimit for given Settings
// and use the given Swarmer. Group is used in log messages to identify
// the ratelimit instance and has to be the same in all skipper instances.
func newClusterRateLimiter(s Settings, sw Swarmer, group string) *clusterLimit {
	rl := &clusterLimit{
		group:   group,
		swarm:   sw,
		maxHits: s.MaxHits,
		window:  s.TimeWindow,
		metrics: metrics.Default,
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
				// TODO(sszuecs): resize is only good if maxHits >> size.n , which means not have to many skipper instances
				instSize := rl.maxHits / size.n
				log.Debugf("%s resize clusterRatelimit: size=%v, rl.maxHits=%d, instance to: %v", group, size, rl.maxHits, instSize)
				rl.Resize(size.s, instSize)
			case <-rl.quit:
				log.Debugf("%s: quit clusterRatelimit", group)
				close(rl.resize)
				return
			}
		}
	}()

	return rl
}

const swarmPrefix string = `ratelimit.`

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimit) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	numInstances := c.swarm.Members()
	c.resize <- resizeLimit{s: s, n: numInstances}

	// t0 is the oldest entry in the local circularbuffer
	// [ t3, t4, t0, t1, t2]
	//           ^- current pointer to oldest
	// now - t0
	t0 := c.Oldest(s).UTC().UnixNano()
	c.metrics.UpdateGauge("swarm."+key+".t0", float64(t0))

	_ = c.local.Allow(s) // update local rate limit

	// if t0 < 0 { // circularbuffer not filled yet
	// 	log.Debugf("clusterLimit not filled: allow")
	// 	return true
	// }
	nowt := time.Now().UTC()
	// if t0 < nowt.Add(-c.window*2).UnixNano() { // TEST: do not populate outdated values
	// 	log.Debugf("clusterLimit t0 too old to share: allow")
	// 	return true
	// }

	if err := c.swarm.ShareValue(key, t0); err != nil {
		log.Errorf("%s clusterRatelimit failed to share value: %v", c.group, err)
	}

	swarmValues := c.swarm.Values(key)
	now := nowt.UnixNano()
	rate := c.calcTotalRequestRate(now, numInstances, swarmValues)
	log.Debugf("clusterRatelimit(%s, %d/%s)=%v numInstances(%d) requestrate=%0.2f t0=%d swarmValues(%d) for '%s': %v",
		c.group, c.maxHits, c.window, rate < float64(c.maxHits), numInstances, rate, t0, len(swarmValues), swarmPrefix+s, swarmValues)
	c.metrics.UpdateGauge("swarm."+key+".rate", rate)
	return rate < float64(c.maxHits)
}

func (c *clusterLimit) calcTotalRequestRate(now int64, numInstances int, swarmValues map[string]interface{}) float64 {
	var requestRate float64
	// len(swarmValues) instead of numInstances would create harm, because of stale data
	maxNodeHits := math.Max(1.0, float64(c.maxHits)/(float64(numInstances)))

	for k, v := range swarmValues {
		t0, ok := v.(int64)
		if !ok || t0 == 0 {
			continue
		}
		delta := now - t0
		log.Debugf("clusterRatelimit delta: %s %d", time.Duration(delta), delta)
		if delta < 0 {
			log.Warningf("Clock skew, should not happen: %v - %v = %v", now, t0, delta)
			continue
		}

		// TODO(sszuecs) would be nice if we can drop this
		if delta > int64(10*c.window) { // 1m40s     or 100000000000
			//if delta > int64(100*c.window) { // 16m40s or 1000000000000
			continue
		}

		// cap to max 1.0 seems to be required (also tested 1.1), otherwise we spike and disallow too many
		// adjusted := float64(delta) / float64(c.window)
		adjusted := math.Max(1.0, float64(delta)/float64(c.window))

		requestRate += maxNodeHits / adjusted
		log.Debugf("clusterRatelimit %s (%s): %0.2f += %0.2f / %0.2f", c.group, k, requestRate, maxNodeHits, adjusted)
	}
	return requestRate
}

// Close should be called to teardown the clusterLimit.
func (c *clusterLimit) Close() {
	close(c.quit)
	c.local.Close()
}

func (c *clusterLimit) Delta(s string) time.Duration { return c.local.Delta(s) }
func (c *clusterLimit) Oldest(s string) time.Time    { return c.local.Oldest(s) }
func (c *clusterLimit) Resize(s string, n int)       { c.local.Resize(s, n) }
func (c *clusterLimit) RetryAfter(s string) int      { return c.local.RetryAfter(s) }
