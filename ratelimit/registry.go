package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
)

const (
	DefaultMaxhits       = 20
	DefaultTimeWindow    = 1 * time.Second
	DefaultCleanInterval = 60 * time.Second
)

// Registry objects hold the active ratelimiters, ensure synchronized
// access to them, apply default settings and recycle the idle
// ratelimiters.
type Registry struct {
	sync.Mutex
	once        sync.Once
	global      Settings
	lookup      map[Settings]*Ratelimit
	swarm       Swarmer
	redisClient *net.RedisClient
}

// NewRegistry initializes a registry with the provided default settings.
// Deprecated: Use NewSwarmRegistry to potentially configure cluster rate limiting.
func NewRegistry(settings ...Settings) *Registry {
	log.Warn("ratelimit.NewRegistry is deprecated, use NewSwarmRegistry for cluster support.")
	return NewSwarmRegistry(nil, nil, settings...)
}

// NewSwarmRegistry initializes a registry with an optional swarm and
// the provided default settings. If swarm is nil, clusterRatelimits
// will be replaced by voidRatelimit, which is a noop limiter implementation.
func NewSwarmRegistry(swarm Swarmer, ro *net.RedisOptions, settings ...Settings) *Registry {
	defaults := Settings{
		Type:          DisableRatelimit,
		MaxHits:       DefaultMaxhits,
		TimeWindow:    DefaultTimeWindow,
		CleanInterval: DefaultCleanInterval,
	}

	// Ensure Redis metrics prefix is set if Redis options are provided
	if ro != nil && ro.MetricsPrefix == "" {
		ro.MetricsPrefix = redisMetricsPrefix
	}

	// Initialize the registry
	r := &Registry{
		once:        sync.Once{},
		global:      defaults,
		lookup:      make(map[Settings]*Ratelimit),
		swarm:       swarm,
		redisClient: nil,
	}

	// Create Redis client only if RedisOptions are provided
	if ro != nil {
		client := net.NewRedisClient(ro)
		if client != nil {
			r.redisClient = client
			log.Info("Initialized Redis client for cluster rate limiting.")
		} else {
			log.Error("Failed to initialize Redis client despite RedisOptions being provided.")
		}
	} else {
		log.Info("No RedisOptions provided, Redis-based cluster rate limiting disabled.")
	}

	if len(settings) > 0 {
		r.global = settings[0]
		log.Infof("Applied global default ratelimit settings: %v", r.global)
	} else {
		log.Infof("Using default global ratelimit settings: %v", r.global)
	}

	return r
}

// Close teardown Registry and dependent resources
func (r *Registry) Close() {
	r.once.Do(func() {
		log.Info("Closing ratelimit Registry...")
		if r.redisClient != nil {
			r.redisClient.Close()
			log.Info("Closed Redis client connection.")
		}

		r.Lock()
		for s, rl := range r.lookup {
			if rl != nil {
				rl.Close()
			}
			delete(r.lookup, s)
		}
		r.Unlock()
		log.Info("Closed all active ratelimiter instances.")
		log.Info("Ratelimit Registry closed.")
	})
}

func (r *Registry) get(s Settings) *Ratelimit {
	rl, ok := r.lookup[s]
	if !ok {
		rl = newRatelimit(s, r.swarm, r.redisClient)
		if rl == nil {
			log.Errorf("newRatelimit returned nil for settings: %v. This should not happen.", s)
			return &Ratelimit{settings: s, impl: voidRatelimit{}}
		}
		r.lookup[s] = rl
		log.Debugf("Created new ratelimiter instance for settings: %v", s)
	}

	return rl
}

// Get returns a Ratelimit instance for provided Settings
func (r *Registry) Get(s Settings) *Ratelimit {
	if s.Type == DisableRatelimit || s.Type == NoRatelimit {
		return nil
	}

	r.Lock()
	defer r.Unlock()

	return r.get(s)
}

// Check returns Settings used and the retry-after duration in case of
// request is ratelimitted. Otherwise return the Settings and 0. It is
// only used in the global ratelimit facility.
// Note: This method only considers the *global* setting, not route-specific ones.
func (r *Registry) Check(req *http.Request) (Settings, int) {
	if r == nil {
		// Registry not initialized
		return Settings{}, 0
	}

	s := r.global

	if s.Type == DisableRatelimit || s.Type == NoRatelimit {
		return Settings{}, 0
	}

	rlimit := r.Get(s)
	if rlimit == nil {
		log.Warn("r.Get returned nil unexpectedly in Check method.")
		return Settings{}, 0
	}

	var lookupKey string
	switch s.Type {
	case ClusterServiceRatelimit, ServiceRatelimit:
		lookupKey = ""
	case LocalRatelimit:
		log.Warning("LocalRatelimit is deprecated, please use ClientRatelimit instead")
		fallthrough
	case ClusterClientRatelimit, ClientRatelimit:
		lookuper := s.Lookuper
		if lookuper == nil {
			lookuper = XForwardedForLookuper{}
		}
		lookupKey = lookuper.Lookup(req)
		if lookupKey == "" {
			// If lookup key is empty for client-based limiting, treat as unidentifiable client.
			// Depending on policy, might allow or deny. Current default is allow (return 0).
			log.Debugf("Lookup key for global client-based ratelimit check is empty for request %s %s", req.Method, req.URL.Path)
		}

	default:
		log.Errorf("Unknown global ratelimit type encountered in Check: %v", s.Type)
		return Settings{}, 0
	}

	if !rlimit.Allow(context.Background(), lookupKey) {
		// Only log rate limit denial if lookup key was valid.
		// Avoid flooding logs for requests without identifiable keys if those are allowed.
		if lookupKey != "" || s.Type == ClusterServiceRatelimit || s.Type == ServiceRatelimit {
			log.Debugf("Global rate limit check denied, type %s.", s.Type)
		}
		retryAfter := rlimit.RetryAfter(lookupKey)
		log.Debugf("Global rate limit check denied, type %s. RetryAfter: %d", s.Type, retryAfter)
		return s, retryAfter
	}

	return Settings{}, 0
}
