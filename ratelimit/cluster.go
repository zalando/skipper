package ratelimit

const swarmPrefix string = `ratelimit.`

func newClusterRateLimiter(s Settings, sw Swarmer, ro *RedisOptions, group string) limiter {
	if sw != nil {
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
