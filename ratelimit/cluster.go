package ratelimit

import "github.com/zalando/skipper/swarm"

const swarmPrefix string = `ratelimit.`

// newClusterRateLimiter will return a limiter instance, that has a
// cluster wide knowledge of ongoing requests. Settings are the normal
// ratelimit settings, Swarmer is an instance sattisfying the Swarmer
// interface, which is one of swarm.Swarm or noopSwarmer,
// swarm.Options to configure a swarm.Swarm, RedisOptions to configure
// redis.Ring and group is the ratelimit group that can span one or
// multiple routes.
func newClusterRateLimiter(s Settings, sw Swarmer, so *swarm.Options, ro *RedisOptions, group string) limiter {
	if so != nil {
		if l := newClusterRateLimiterSwim(s, sw, group); l != nil {
			return l
		}
	}
	if ro != nil {
		if l := newClusterRateLimiterRedis(s, ro, group); l != nil {
			return l
		}
	}
	return voidRatelimit{}
}
