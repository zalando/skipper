package ratelimit

import (
	"github.com/hashicorp/golang-lru"
	"github.com/zalando/skipper/net"
)

const (
	swarmPrefix          = `ratelimit.`
	swarmKeyFormat       = swarmPrefix + "%s.%s"
	swarmKeyFormatCached = swarmPrefix + "cached.%s.%s"
)

// newClusterRateLimiter will return a limiter instance, that has a
// cluster wide knowledge of ongoing requests. Settings are the normal
// ratelimit settings, Swarmer is an instance satisfying the Swarmer
// interface, which is one of swarm.Swarm or noopSwarmer,
// swarm.Options to configure a swarm.Swarm, RedisOptions to configure
// redis.Ring and group is the ratelimit group that can span one or
// multiple routes.
//
// If a non-nil cache is provided, the redis based cluster rate limiter
// will use the cache to limit the calls to the Redis instances based
// the time window the cache period factor.
//
func newClusterRateLimiter(
	s Settings,
	sw Swarmer,
	ring *net.RedisRingClient,
	c *lru.Cache,
	group string,
	cachePeriodFactor int,
) limiter {
	if sw != nil {
		if l := newClusterRateLimiterSwim(s, sw, group); l != nil {
			return l
		}
	}

	if ring != nil && c != nil {
		return newClusterLimitRedisCached(s, ring, c, group, cachePeriodFactor)
	}

	if ring != nil {
		if l := newClusterRateLimiterRedis(s, ring, group); l != nil {
			return l
		}
	}

	return voidRatelimit{}
}
