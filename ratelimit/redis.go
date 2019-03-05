package ratelimit

import (
	"fmt"
	"strconv"
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
	maxHits    int64
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
	log.Infof("redis ring: %+v", ring)
	log.Infof("redis ring options: %+v", ring.Options())

	rl := &clusterLimitRedis{
		group:   group,
		maxHits: int64(s.MaxHits),
		window:  s.TimeWindow,
		ring:    ring,
	}

	var pong string
	var err error
	for _, i := range []int{1, 2, 3, 4, 5} {
		pong, err = rl.ring.Ping().Result()
		if err != nil {
			log.Infof("%d. Failed to ping redis: %v", i, err)
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}
	if err != nil {
		log.Errorf("Failed to ping redis: %v", err)
		return nil
	}

	log.Infof("pong: %v", pong)
	return rl
}

// Allow returns true if the request calculated across the cluster of
// skippers should be allowed else false. It will share it's own data
// and use the current cluster information to calculate global rates
// to decide to allow or not.
func (c *clusterLimitRedis) Allow(s string) bool {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()
	nowSec := now.UnixNano()
	clearBefore := now.Add(-c.window).UnixNano()

	count := c.allowCheckCard(key, clearBefore)
	if count > c.maxHits {
		log.Infof("redis disallow %s request: %d > %d = %v", key, count, c.maxHits, count > c.maxHits)
		return false
	}

	c.ring.ZAdd(key, redis.Z{Member: nowSec, Score: float64(nowSec)})
	return true
}

func (c *clusterLimitRedis) allowCheckCard(key string, clearBefore int64) int64 {
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
		log.Errorf("Failed to get redis cardinality for %s: %v", key, err)
		return 0
	}
	return zcardResult.Val()
}

func (c *clusterLimitRedis) Close()                       { c.ring.Close() }
func (c *clusterLimitRedis) Delta(s string) time.Duration { return 10 * c.window }

func (c *clusterLimitRedis) Oldest(s string) time.Time {
	key := swarmPrefix + c.group + "." + s
	now := time.Now()

	res := c.ring.ZRangeByScoreWithScores(key, redis.ZRangeBy{
		Min:    "0.0",
		Max:    fmt.Sprintf("%v", float64(now.UnixNano())),
		Offset: 0,
		Count:  1,
	})

	if zs := res.Val(); len(zs) > 0 {
		z := zs[0]
		if s, ok := z.Member.(string); ok {
			oldest, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				log.Errorf("Failed to convert '%v' to int64: %v", s, err)
			}
			return time.Unix(0, oldest)
		}
		log.Errorf("Failed to convert redis data to int64: %v", z.Member)
	}
	log.Errorf("redis oldest got no valid data: %v", res)
	return time.Time{}
}

func (c *clusterLimitRedis) Resize(s string, n int) {}
func (c *clusterLimitRedis) RetryAfter(s string) int {
	now := time.Now()
	oldest := c.Oldest(s)
	gab := now.Sub(oldest)
	retr := c.window - gab
	res := int(retr / time.Second)
	if res > 0 {
		return res
	}
	// Less than 1s to wait -> so set to 1
	return 1
}
