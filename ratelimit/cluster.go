package ratelimit

import (
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/go-redis/redis_rate"
	log "github.com/sirupsen/logrus"
)

// clusterLimit stores all data required for the cluster ratelimit.
type clusterLimit struct {
	mu         sync.Mutex
	group      string
	maxHits    int
	window     time.Duration
	ring       *redis.Ring
	limiter    *redis_rate.Limiter
	retryAfter int
}

type resizeLimit struct {
	s string
	n int
}

// newClusterRateLimiter creates a new clusterLimit for given Settings
// and use the given Swarmer. Group is used in log messages to identify
// the ratelimit instance and has to be the same in all skipper instances.
func newClusterRateLimiter(s Settings, group string) *clusterLimit {
	log.Infof("creating clusterLimiter")
	ring := redis.NewRing(&redis.RingOptions{
		Addrs: map[string]string{
			"server1": "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
			"server2": "skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379",
		},
	})
	limiter := redis_rate.NewLimiter(ring)
	// Optional.
	//limiter.Fallback = rate.NewLimiter(rate.Every(s.TimeWindow), s.MaxHits)

	rl := &clusterLimit{
		group:   group,
		maxHits: s.MaxHits,
		window:  s.TimeWindow,
		ring:    ring,
		limiter: limiter,
	}

	pong, err := rl.ring.Ping().Result()
	if err != nil {
		log.Errorf("Failed to ping redis: %v", err)
		return nil
	}
	log.Debugf("pong: %v", pong)

	return rl
}

const swarmPrefix string = `ratelimit.`

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimit) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	rate, delay, allowed := c.limiter.AllowN(key, int64(c.maxHits), c.window, 1)
	log.Infof("rate: %v, delay: %v, allow: %v", rate, delay, allowed)
	if rate == 0 { // if redis is not reachable allow
		log.Infof("allow rate is 0")
		return true
	}
	retr := (c.window - delay) / time.Second
	c.mu.Lock()
	c.retryAfter = int(retr)
	c.mu.Unlock()
	return allowed
}

func (c *clusterLimit) Close()                       {}
func (c *clusterLimit) Delta(s string) time.Duration { return 10 * c.window }
func (c *clusterLimit) Oldest(s string) time.Time    { return time.Now().Add(-10 * c.window) }
func (c *clusterLimit) Resize(s string, n int)       {}
func (c *clusterLimit) RetryAfter(s string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.retryAfter
}
