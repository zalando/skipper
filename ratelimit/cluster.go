package ratelimit

import "github.com/zalando/skipper/swarm"

const swarmPrefix string = `ratelimit.`

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
