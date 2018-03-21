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

func NewClusterRateLimiter(s Settings, sw Swarmer) implementation {
	rl := &ClusterLimit{
		swarm:   sw,
		maxHits: float64(s.MaxHits),
		window:  s.TimeWindow,
		resize:  make(chan resizeLimit),
		quit:    make(chan struct{}),
	}
	if s.CleanInterval == 0 {
		rl.local = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	} else {
		rl.local = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	}

	go func() {
		for {
			select {
			case size := <-rl.resize:
				// call with "go" ?
				rl.local.Resize(size.s, int(rl.maxHits)/size.n)
			case <-rl.quit:
				return
			}
		}
	}()

	return rl
}

const swarmPrefix string = `ratelimit.`

func (c *ClusterLimit) Allow(s string) bool {
	_ = c.local.Allow(s) // update local rate limit
	if err := c.swarm.ShareValue(swarmPrefix+s, c.local.Delta(s)); err != nil {
		log.Errorf("SWARM failed to share value: %s\n", err)
	}

	var rate float64
	swarmValues := c.swarm.Values(swarmPrefix + s)
	c.resize <- resizeLimit{s: s, n: len(swarmValues)}

	nodeHits := c.maxHits / float64(len(swarmValues)) // hits per node within the window from the global rate limit
	for _, val := range swarmValues {
		if delta, ok := val.(time.Duration); ok {
			switch {
			case delta == 0:
				// initially all are set to time.Time{}, so we get 0 delta
			case delta < 0:
				// should not happen... but anyway, we set to global rate
				rate += c.maxHits / float64(c.window)
			default:
				rate += nodeHits / float64(delta)
			}
		}
	}
	log.Debugf("SWARM clusterRatelimit: %d values: %d, rate: %0.2f", len(swarmValues), rate)
	return rate < float64(c.maxHits)/float64(c.window)
}

func (c *ClusterLimit) Close() {
	close(c.quit)
	c.local.Close()
}

func (c *ClusterLimit) Delta(s string) time.Duration {
	return c.local.Delta(s)
}

func (c *ClusterLimit) Resize(s string, n int) {
	c.local.Resize(s, n)
}
