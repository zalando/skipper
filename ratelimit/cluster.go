package ratelimit

import "github.com/zalando/skipper/net"

const (
	swarmPrefix    = `ratelimit.`
	swarmKeyFormat = swarmPrefix + "%s.%s"
)

// newClusterRateLimiter will return a limiter instance, that has a
// cluster wide knowledge of ongoing requests. Settings are the normal
// ratelimit settings, Swarmer is an instance satisfying the Swarmer
// interface, which is one of swarm.Swarm or noopSwarmer,
// swarm.Options to configure a swarm.Swarm, RedisOptions to configure
// redis.Ring and group is the ratelimit group that can span one or
// multiple routes.
func newClusterRateLimiter(s Settings, sw Swarmer, ring *net.RedisRingClient, group string) limiter {
	if sw != nil {
		if l := newClusterRateLimiterSwim(s, sw, group); l != nil {
			return l
		}
	}
	if ring != nil {
		if l := newClusterRateLimiterRedis(s, ring, group); l != nil {
			return l
		}
	}
	return voidRatelimit{}
}
