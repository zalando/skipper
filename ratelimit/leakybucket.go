package ratelimit

import (
	"context"
	_ "embed"
	"time"
)

// Implements leaky bucket algorithm as a Redis lua script.
// Redis guarantees that a script is executed in an atomic way:
// no other script or Redis command will be executed while a script is being executed.
//
// Possible optimization: substitute capacity and emission in script source code
// on script creation in order not to send them over the wire on each call.
// This way every distinct bucket configuration will get its own script.
//
// See https://redis.io/commands/eval
//
//go:embed leakybucket.lua
var leakyBucketScript string

// LeakyBucketLimiter is the interface for cluster leaky bucket implementations.
type LeakyBucketLimiter interface {
	Add(ctx context.Context, label string, increment int) (added bool, retry time.Duration, err error)
}

// NewClusterLeakyBucket creates a class of leaky buckets of a given capacity and emission.
// Emission is the reciprocal of the leak rate and equals the time to leak one unit.
// Prefers Valkey over Redis when both are configured.
//
// The leaky bucket is an algorithm based on an analogy of how a bucket with a constant leak will overflow if either
// the average rate at which water is poured in exceeds the rate at which the bucket leaks or if more water than
// the capacity of the bucket is poured in all at once.
// See https://en.wikipedia.org/wiki/Leaky_bucket
func NewClusterLeakyBucket(r *Registry, capacity int, emission time.Duration) LeakyBucketLimiter {
	if r.valkeyRing != nil {
		return newClusterLeakyBucketValkey(r.valkeyRing, capacity, emission, time.Now)
	}
	return newClusterLeakyBucketRedis(r.redisRing, capacity, emission, time.Now)
}
