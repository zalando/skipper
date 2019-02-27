package ratelimit

import (
	"fmt"
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
	client     *redis.Client
	ring       *redis.Ring
	limiter    *redis_rate.Limiter
	retryAfter int
}

// newClusterRateLimiter creates a new clusterLimit for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiter(s Settings, group string) *clusterLimit {
	log.Infof("creating clusterLimiter")

	// ring := redis.NewRing(&redis.RingOptions{
	// 	Addrs: map[string]string{
	// 		"server1": "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
	// 		"server2": "skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379",
	// 	},
	// })
	// limiter := redis_rate.NewLimiter(ring)
	// // Optional.
	// //limiter.Fallback = rate.NewLimiter(rate.Every(s.TimeWindow), s.MaxHits)

	// TODO(sszuecs):
	client := redis.NewClient(&redis.Options{
		Addr: "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
		//Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})
	// TODO(sszuecs): if this is good wrap with context and add deadline
	//client = client.WithContext(context.Background())

	rl := &clusterLimit{
		group:   group,
		maxHits: s.MaxHits,
		window:  s.TimeWindow,
		client:  client,
		// ring:    ring,
		// limiter: limiter,
	}

	pong, err := rl.client.Ping().Result()
	if err != nil {
		log.Errorf("Failed to ping redis: %v", err)
		return nil
	}
	log.Debugf("pong: %v", pong)

	// pong, err = rl.ring.Ping().Result()
	// if err != nil {
	// 	log.Errorf("Failed to ping redis: %v", err)
	// 	return nil
	// }
	// log.Debugf("pong: %v", pong)

	return rl
}

const swarmPrefix string = `ratelimit.`

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimit) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()
	//clearBefore := now.Add(c.window)

	// run MULTI exec
	pipe := c.client.TxPipeline()
	defer pipe.Close()

	// drop all elements of the set which occurred before one interval ago.
	pipe.ZRemRangeByScore(key, "0.0", fmt.Sprintf("%v", float64(now.Add(-c.window).UnixNano())))
	//c.client.ZRemRangeByScore(key, "0.0", fmt.Sprintf("%v", float64(now.Add(-c.window).UnixNano())))

	// fetch all elements of the set
	//zrangeResult := pipe.ZRange(key, 0, -1)
	//c.client.ZRange(key, 0, -1)

	// add the current timestamp to the set
	pipe.ZAdd(key, redis.Z{Member: now.UnixNano(), Score: float64(now.UnixNano())})
	//c.client.ZAdd(key, redis.Z{Member: now.UnixNano(), Score: float64(now.UnixNano())})

	zcardResult := pipe.ZCard(key)

	pipe.Expire(key, c.window)
	_, err := pipe.Exec()
	if err != nil {
		log.Errorf("Failed to exec pipeline: %v", err)
		return true
	}

	log.Debugf("number of requests from %s: %v", key, zcardResult.Val())

	// After all operations are completed, we count the number of fetched elements. If it exceeds the limit, we don’t allow the action.
	//count := c.client.ZCard(key).Val()
	count := zcardResult.Val()
	return count < int64(c.maxHits)
}

// no TxPipeline possible with the library https://github.com/go-redis/redis/blob/master/ring.go#L640
func (c *clusterLimit) Allow2(s string) bool {
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
	return 1
}
func (c *clusterLimit) RetryAfter2(s string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.retryAfter
}
