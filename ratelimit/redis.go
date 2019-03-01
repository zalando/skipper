package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

type RedisOptions struct {
	Addrs []string
}

// clusterLimitRedis stores all data required for the cluster ratelimit.
type clusterLimitRedis struct {
	mu         sync.Mutex
	group      string
	maxHits    int
	window     time.Duration
	client     *redis.Client
	ring       *redis.Ring
	retryAfter int
}

// newClusterRateLimiterRedis creates a new clusterLimitRedis for given
// Settings. Group is used to identify the ratelimit instance, is used
// in log messages and has to be the same in all skipper instances.
func newClusterRateLimiterRedis(s Settings, options *RedisOptions, group string) *clusterLimitRedis {
	if options == nil {
		return nil
	}

	ringOptions := &redis.RingOptions{
		Addrs: map[string]string{},
	}
	for idx, addr := range options.Addrs {
		ringOptions.Addrs[fmt.Sprintf("server%d", idx)] = addr
	}

	ring := redis.NewRing(ringOptions)
	// TODO(sszuecs): if this is good wrap with context and add deadline
	//ring = ring.WithContext(context.Background())

	rl := &clusterLimitRedis{
		group:   group,
		maxHits: s.MaxHits,
		window:  s.TimeWindow,
		ring:    ring,
	}

	pong, err := rl.ring.Ping().Result()
	if err != nil {
		log.Errorf("Failed to ping redis: %v", err)
		return nil
	}
	log.Debugf("pong: %v", pong)

	return rl
}

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimitRedis) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()
	nowSec := now.Unix()
	clearBefore := now.Add(-c.window).Unix()

	// TODO(sszuecs): https://github.com/go-redis/redis/issues/979 change to TxPipeline: MULTI exec
	// Pipeline is not a transaction
	pipe := c.ring.Pipeline()
	defer pipe.Close()
	// drop all elements of the set which occurred before one interval ago.
	pipe.ZRemRangeByScore(key, "0.0", fmt.Sprintf("%v", float64(clearBefore)))
	// get cardinality
	zcardResult := pipe.ZCard(key)
	_, err := pipe.Exec()
	if err != nil {
		log.Errorf("Failed to get cardinality: %v", err)
		return true
	}
	count := zcardResult.Val()
	if count > int64(c.maxHits) {
		return false
	}

	// TODO(sszuecs): https://github.com/go-redis/redis/issues/979 change to TxPipeline: MULTI exec
	pipe2 := c.ring.Pipeline()
	defer pipe2.Close()
	// add the current timestamp to the set
	pipe2.ZAdd(key, redis.Z{Member: nowSec, Score: float64(nowSec)})
	// increment cardinality
	count++
	// expire value if it is too old
	pipe2.Expire(key, c.window)
	_, err = pipe2.Exec()
	if err != nil {
		log.Errorf("Failed to exec pipeline: %v", err)
		// could not add, but we can use count
	}

	return count <= int64(c.maxHits)
}

func (c *clusterLimitRedis) Close()                       { c.ring.Close() }
func (c *clusterLimitRedis) Delta(s string) time.Duration { return 10 * c.window }

func (c *clusterLimitRedis) Oldest(s string) time.Time {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()

	res := c.ring.ZRangeByScoreWithScores(key, redis.ZRangeBy{
		Min:    "0.0",
		Max:    fmt.Sprintf("%v", float64(now.Unix())),
		Offset: 0,
		Count:  1,
	})
	if zs := res.Val(); len(zs) > 0 {
		z := zs[0]
		if oldest, ok := z.Member.(int64); ok {
			return time.Unix(oldest, 0)
		}
	}
	return time.Time{}
}

func (c *clusterLimitRedis) Resize(s string, n int) {}
func (c *clusterLimitRedis) RetryAfter(s string) int {
	now := time.Now()
	oldest := c.Oldest(s)
	gab := now.Sub(oldest)
	retr := gab - c.window
	if res := int(retr / time.Second); res > 0 {
		return res
	}
	return 0
}
