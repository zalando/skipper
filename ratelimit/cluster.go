package ratelimit

import (
	"fmt"
	"os"
	"time"

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
}

func NewClusterRateLimiter(s Settings, sw Swarmer) implementation {
	rl := &ClusterLimit{
		swarm:   sw,
		maxHits: float64(s.MaxHits),
		window:  s.TimeWindow,
	}
	if s.CleanInterval == 0 {
		rl.local = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	} else {
		rl.local = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	}
	return rl
}

const swarmPrefix string = `ratelimit.`

func (c *ClusterLimit) Allow(s string) bool {
	_ = c.local.Allow(s) // update local rate limit
	if err := c.swarm.ShareValue(swarmPrefix+s, c.local.Delta(s)); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to share value: %s\n", err)
	}
	var rate float64
	swarmValues := c.swarm.Values(swarmPrefix + s)
	nodeHits := c.maxHits / float64(len(swarmValues)) // hits per node within the window from the global rate limit
	for _, val := range swarmValues {
		if delta, ok := val.(time.Duration); ok {
			if delta <= 0 { // should not happen... but anyway, we set to global rate
				rate += c.maxHits / float64(c.window)
			} else {
				rate += nodeHits / float64(delta)
			}
		}
	}
	return rate < float64(c.maxHits)/float64(c.window)
}

func (c *ClusterLimit) Close() {
	c.local.Close()
}

func (c *ClusterLimit) Delta(s string) time.Duration {
	return c.local.Delta(s)
}
