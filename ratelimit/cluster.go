package ratelimit

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
)

const (
	swarmPrefix = `ratelimit.`
	// swarmKeyFormat defines the format for Redis keys used in cluster ratelimiting.
	// Format: ratelimit.<group>.<hashed_lookup_key>
	// Example: ratelimit.login_group.a1b2c3d4e5f6...
	swarmKeyFormat = swarmPrefix + "%s.%s"
)

// newClusterRateLimiter decides which cluster-aware rate limiter implementation to use (Swim or Redis).
// It prioritizes Swim if available, then falls back to Redis if configured.
//
// Parameters:
//   - s: Ratelimit settings.
//   - sw: Swarmer instance (e.g., swim-based implementation or nil).
//   - redisClient: The configured unified Redis client (*net.RedisClient or nil).
//   - group: The logical group name for this rate limit across the cluster.
//
// Returns:
//   - An initialized limiter implementation (Swim, Redis, or voidRatelimit if neither is available).
func newClusterRateLimiter(s Settings, sw Swarmer, redisClient *net.RedisClient, group string) limiter {
	// Prioritize Swim-based limiter if Swarmer is provided and initialization succeeds
	if sw != nil {
		if l := newClusterRateLimiterSwim(s, sw, group); l != nil {
			log.Infof("Using Swim-based cluster rate limiter for group '%s'", group)
			return l
		}
		// If Swim initialization failed but Swarmer was provided, log it
		log.Warnf("Swarmer provided for group '%s', but Swim limiter initialization failed. Checking for Redis.", group)
	}

	// Fallback to Redis-based limiter if Redis client is provided and initialization succeeds
	if redisClient != nil {
		// Pass the unified redisClient here
		if l := newClusterRateLimiterRedis(s, redisClient, group); l != nil {
			// Logging is now done inside newClusterRateLimiterRedis if successful
			return l
		}
		// If Redis initialization failed but client was provided, log it
		log.Warnf("Redis client provided for group '%s', but Redis limiter initialization failed.", group)
	}

	// If neither Swim nor Redis is available or initialized successfully, return a no-op limiter
	log.Warnf("Neither Swim nor Redis cluster rate limiter could be initialized for group '%s'. Using voidRatelimit (no limiting).", group)
	return voidRatelimit{}
}
