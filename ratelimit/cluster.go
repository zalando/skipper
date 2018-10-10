package ratelimit

import (
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
				log.Debugf("resize clusterRatelimit: %v", size)
				// TODO(sszuecs): call with "go" ?
				rl.Resize(size.s, int(rl.maxHits)/size.n)
			case <-rl.quit:
				log.Debugf("quit clusterRatelimit")
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

	// t0 is the oldest entry in the local circularbuffer
	// [ t3, t4, t0, t1, t2]
	//           ^- current pointer to oldest
	// now - t0
	t0 := c.Oldest(s).UTC().UnixNano()

	_ = c.local.Allow(s) // update local rate limit

	if err := c.swarm.ShareValue(swarmPrefix+s, t0); err != nil {
		log.Errorf("clusterRatelimit failed to share value: %v", err)
	}

	swarmValues := c.swarm.Values(swarmPrefix + s)
	log.Debugf("%s: clusterRatelimit swarmValues(%d) for '%s': %v", c.name, len(swarmValues), swarmPrefix+s, swarmValues)

	c.resize <- resizeLimit{s: s, n: len(swarmValues)}

	now := time.Now().UTC().UnixNano()
	rate := c.calcTotalRequestRate(now, swarmValues)
	result := rate < c.maxHits
	log.Debugf("clusterRatelimit %s: Allow=%v, %v < %v", c.name, result, rate, c.maxHits)
	return result
}

func (c *ClusterLimit) calcTotalRequestRate(now int64, swarmValues map[string]interface{}) float64 {
	var requestRate float64
	maxNodeHits := c.maxHits / float64(len(swarmValues))

	for _, v := range swarmValues {
		t0, ok := v.(int64)
		if !ok || t0 == 0 {
			continue
		}
		delta := time.Duration(now - t0)
		adjusted := float64(delta) / float64(c.window)
		log.Debugf("%0.2f += %0.2f / %0.2f", requestRate, maxNodeHits, adjusted)
		requestRate += maxNodeHits / adjusted
	}
	log.Debugf("requestRate: %0.2f", requestRate)
	return requestRate
}

func (c *ClusterLimit) Close() {
	close(c.quit)
	c.local.Close()
}

func (c *ClusterLimit) Delta(s string) time.Duration { return c.local.Delta(s) }
func (c *ClusterLimit) Oldest(s string) time.Time    { return c.local.Oldest(s) }
func (c *ClusterLimit) Resize(s string, n int)       { c.local.Resize(s, n) }
func (c *ClusterLimit) RetryAfter(s string) int      { return c.local.RetryAfter(s) }
