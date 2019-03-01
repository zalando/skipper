package ratelimit

const swarmPrefix string = `ratelimit.`

func newClusterRateLimiter(s Settings, sw Swarmer, group string) limiter {
	if sw != nil {
		if l := newClusterRateLimiterSwim(s, sw, group); l != nil {
			return l
		}
		return voidRatelimit{}
	}
	if l := newClusterRateLimiterRedis(s, group); l != nil {
		return l
	}
	return voidRatelimit{}
}
