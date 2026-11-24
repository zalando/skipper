package ratelimit

import "github.com/zalando/skipper/net"

const (
	swarmPrefix    = `ratelimit.`
	swarmKeyFormat = swarmPrefix + "%s.%s"
)

// newClusterRateLimiter will return a limiter instance, that has a
// cluster wide knowledge of ongoing requests. Settings are the normal
// ratelimit settings. redisRing to configure Redis ring shard cluster
// rate limit. valkeyRing to configure a Valkey ring shard cluster rate
// limit. group is the ratelimit group that can span one or multiple
// routes.
func newClusterRateLimiter(s Settings, redisRing *net.RedisRingClient, valkeyRing *net.ValkeyRingClient, group string) limiter {
	if valkeyRing != nil {
		if l := newClusterRateLimiterValkey(s, valkeyRing, group); l != nil {
			return l
		}
	}
	if redisRing != nil {
		if l := newClusterRateLimiterRedis(s, redisRing, group); l != nil {
			return l
		}
	}
	return voidRatelimit{}
}
