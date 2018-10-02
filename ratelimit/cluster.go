package ratelimit

import (
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
)

type Swarmer interface {
	ShareValue(string, interface{}) error
	Values(string) map[string]interface{}
}

type ClusterLimit struct {
	name    string
	local   implementation
	maxHits float64
	window  time.Duration
	swarm   Swarmer
	resize  chan resizeLimit
	quit    chan struct{}
}

type resizeLimit struct {
	s string
	n int
}

func NewClusterRateLimiter(s Settings, sw Swarmer, name string) *ClusterLimit {
	rl := &ClusterLimit{
		name:    name,
		swarm:   sw,
		maxHits: float64(s.MaxHits),
		window:  s.TimeWindow,
		resize:  make(chan resizeLimit),
		quit:    make(chan struct{}),
	}
	if s.CleanInterval == 0 {
		log.Infof("new backend clusterRateLimiter")
		rl.local = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	} else {
		log.Infof("new client clusterRateLimiter")
		rl.local = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	}

	// TODO(sszuecs): we might want to have one goroutine for all of these
	go func() {
		for {
			select {
			case size := <-rl.resize:
				log.Infof("resize clusterRatelimit")
				// call with "go" ?
				rl.local.Resize(size.s, int(rl.maxHits)/size.n)
			case <-rl.quit:
				log.Infof("quit clusterRatelimit")
				close(rl.resize)
				return
			}
		}
	}()

	return rl
}

const swarmPrefix string = `ratelimit.`

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false.
func (c *ClusterLimit) Allow(s string) bool {
	_ = c.local.Allow(s) // update local rate limit

	// t0 is the oldest entry in the local circularbuffer
	// [ t3, t4, t0, t1, t2]
	//           ^- current pointer to oldest
	// now - t0
	d := time.Now().Sub(c.local.Current(s))
	dTransfer := int64(d)
	if err := c.swarm.ShareValue(swarmPrefix+s, dTransfer); err != nil {
		log.Errorf("clusterRatelimit failed to share value: %v", err)
	}

	swarmValues := c.swarm.Values(swarmPrefix + s)
	log.Infof("%s: d: %v, swarmValues: %d", c.name, d, len(swarmValues))
	log.Debugf("%s: clusterRatelimit swarmValues for '%s': %v", c.name, swarmPrefix+s, swarmValues)

	c.resize <- resizeLimit{s: s, n: len(swarmValues)}

	rate := c.calculateSharedKnowledge(swarmValues)
	result := rate < float64(c.maxHits)/float64(c.window)
	log.Infof("clusterRatelimit %s: Allow=%v, %v < %v/%v", c.name, result, rate, float64(c.maxHits), float64(c.window))
	return result
}

func (c *ClusterLimit) calculateSharedKnowledge(swarmValues map[string]interface{}) float64 {
	var rate float64 = 0
	swarmValuesSize := math.Max(1.0, float64(len(swarmValues)))
	maxNodeHits := c.maxHits / swarmValuesSize

	for _, val := range swarmValues {
		if deltaI, ok := val.(int64); ok {
			delta := time.Duration(deltaI)
			rateV := float64(c.window) / float64(delta)
			if c.window < delta {
				rateV = float64(0)
			}
			log.Debugf("rate %v, deltaI: %d, delta: %v, rateV: %v, c.window: %v", rate, deltaI, delta, rateV, c.window)
			switch {
			case delta == 0:
			case delta > 0:
				rate += rateV
			default:
				log.Errorf("Should not happen: %v, add maxNodeHits to rate", delta)
				rate += maxNodeHits
			}
		} else {
			log.Warningf("%s: val is not int64: %v", c.name, val)
		}
	}
	log.Debugf("returning rate: %0.2f/%v", rate, c.window)
	return rate
}

func (c *ClusterLimit) Close() {
	close(c.quit)
	c.local.Close()
}

func (c *ClusterLimit) Current(string) time.Time     { return time.Time{} }
func (c *ClusterLimit) Delta(s string) time.Duration { return c.local.Delta(s) }
func (c *ClusterLimit) Resize(s string, n int)       { c.local.Resize(s, n) }
func (c *ClusterLimit) RetryAfter(s string) int      { return c.local.RetryAfter(s) }
