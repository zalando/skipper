/*
Package ratelimit provides filters to control the rate limiter settings on the route level.

For detailed documentation of the ratelimit, see https://pkg.go.dev/github.com/zalando/skipper/ratelimit.
*/
package ratelimit

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

const defaultStatusCode = http.StatusTooManyRequests

type spec struct {
	typ        ratelimit.RatelimitType
	provider   RatelimitProvider
	filterName string
	maxShards  int
}

type filter struct {
	settings   ratelimit.Settings
	provider   RatelimitProvider
	statusCode int
	maxHits    int // overrides settings.MaxHits
}

// RatelimitProvider returns a limit instance for provided Settings
type RatelimitProvider interface {
	get(s ratelimit.Settings) limit
}

type limit interface {
	// Allow is used to decide if call with context is allowed to pass
	Allow(context.Context, string) bool

	// RetryAfter is used to inform the client how many seconds it
	// should wait before making a new request
	RetryAfter(string) int
}

// RegistryAdapter adapts ratelimit.Registry to RateLimitProvider interface.
// ratelimit.Registry is not an interface and its Get method returns
// ratelimit.Ratelimit which is not an interface either
// RegistryAdapter narrows ratelimit interfaces to necessary minimum
// and enables easier test stubbing
type registryAdapter struct {
	registry *ratelimit.Registry
}

func (a *registryAdapter) get(s ratelimit.Settings) limit {
	return a.registry.Get(s)
}

func NewRatelimitProvider(registry *ratelimit.Registry) RatelimitProvider {
	return &registryAdapter{registry}
}

// NewLocalRatelimit is *DEPRECATED*, use NewClientRatelimit, instead
func NewLocalRatelimit(provider RatelimitProvider) filters.Spec {
	return &spec{typ: ratelimit.LocalRatelimit, provider: provider, filterName: ratelimit.LocalRatelimitName}
}

// NewClientRatelimit creates an instance based client rate limit.  If
// you have 5 instances with 20 req/s, then it would allow 100 req/s
// to the backend from the same client. A third argument can be used to
// set which HTTP header of the request should be used to find the
// same user. Third argument defaults to XForwardedForLookuper,
// meaning X-Forwarded-For Header.
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> clientRatelimit(20, "1m")
//	-> "https://foo.backend.net";
//
// Example rate limit per Authorization Header:
//
//	login: Path("/login")
//	-> clientRatelimit(3, "1m", "Authorization")
//	-> "https://login.backend.net";
func NewClientRatelimit(provider RatelimitProvider) filters.Spec {
	return &spec{typ: ratelimit.ClientRatelimit, provider: provider, filterName: filters.ClientRatelimitName}
}

// NewRatelimit creates a service rate limiting, that is
// only aware of itself. If you have 5 instances with 20 req/s, then
// it would at max allow 100 req/s to the backend.
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> ratelimit(20, "1s")
//	-> "https://foo.backend.net";
//
// Optionally a custom response status code can be provided as an argument (default is 429).
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> ratelimit(20, "1s", 503)
//	-> "https://foo.backend.net";
func NewRatelimit(provider RatelimitProvider) filters.Spec {
	return &spec{typ: ratelimit.ServiceRatelimit, provider: provider, filterName: filters.RatelimitName}
}

// NewClusterRateLimit creates a rate limiting that is aware of the
// other instances. The value given here should be the combined rate
// of all instances. The ratelimit group parameter can be used to
// select the same ratelimit group across one or more routes.
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> clusterRatelimit("groupA", 200, "1m")
//	-> "https://foo.backend.net";
//
// Optionally a custom response status code can be provided as an argument (default is 429).
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> clusterRatelimit("groupA", 200, "1m", 503)
//	-> "https://foo.backend.net";
func NewClusterRateLimit(provider RatelimitProvider) filters.Spec {
	return NewShardedClusterRateLimit(provider, 1)
}

// NewShardedClusterRateLimit creates a cluster rate limiter that uses multiple group shards to count hits.
// Based on the configured group and maxHits each filter instance selects N distinct group shards from [1, maxGroupShards].
// For every subsequent request it uniformly picks one of N group shards and limits number of allowed requests per group shard to maxHits/N.
//
// For example if maxGroupShards = 10, clusterRatelimit("groupA", 200, "1m") will use 10 distinct groups to count hits and
// will allow up to 20 hits per each group and thus up to configured 200 hits in total.
func NewShardedClusterRateLimit(provider RatelimitProvider, maxGroupShards int) filters.Spec {
	return &spec{typ: ratelimit.ClusterServiceRatelimit, provider: provider, filterName: filters.ClusterRatelimitName, maxShards: maxGroupShards}
}

// NewClusterClientRateLimit creates a rate limiting that is aware of
// the other instances. The value given here should be the combined
// rate of all instances. The ratelimit group parameter can be used to
// select the same ratelimit group across one or more routes.
//
// Example:
//
//	backendHealthcheck: Path("/login")
//	-> clusterClientRatelimit("groupB", 20, "1h")
//	-> "https://foo.backend.net";
//
// The above example would limit access to "/login" if, the client did
// more than 20 requests within the last hour to this route across all
// running skippers in the cluster.  A single client can be detected
// by different data from the http request and defaults to client IP
// or X-Forwarded-For header, if exists. The optional third parameter
// chooses the HTTP header to choose a client is
// counted as the same.
//
// Example:
//
//	backendHealthcheck: Path("/login")
//	-> clusterClientRatelimit("groupC", 20, "1h", "Authorization")
//	-> "https://foo.backend.net";
func NewClusterClientRateLimit(provider RatelimitProvider) filters.Spec {
	return &spec{typ: ratelimit.ClusterClientRatelimit, provider: provider, filterName: filters.ClusterClientRatelimitName}
}

// NewDisableRatelimit disables rate limiting
//
// Example:
//
//	backendHealthcheck: Path("/healthcheck")
//	-> disableRatelimit()
//	-> "https://foo.backend.net";
func NewDisableRatelimit(provider RatelimitProvider) filters.Spec {
	return &spec{typ: ratelimit.DisableRatelimit, provider: provider, filterName: filters.DisableRatelimitName}
}

func (s *spec) Name() string {
	return s.filterName
}

func serviceRatelimitFilter(args []interface{}) (*filter, error) {
	if !(len(args) == 2 || len(args) == 3) {
		return nil, filters.ErrInvalidFilterParameters
	}

	maxHits, err := getIntArg(args[0])
	if err != nil {
		return nil, err
	}

	timeWindow, err := getDurationArg(args[1])
	if err != nil {
		return nil, err
	}

	statusCode, err := getStatusCodeArg(args, 2)
	if err != nil {
		return nil, err
	}

	return &filter{
		settings: ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		statusCode: statusCode,
	}, nil
}

func clusterRatelimitFilter(maxShards int, args []interface{}) (*filter, error) {
	if !(len(args) == 3 || len(args) == 4) {
		return nil, filters.ErrInvalidFilterParameters
	}

	group, err := getStringArg(args[0])
	if err != nil {
		return nil, err
	}

	maxHits, err := getIntArg(args[1])
	if err != nil {
		return nil, err
	}

	timeWindow, err := getDurationArg(args[2])
	if err != nil {
		return nil, err
	}

	statusCode, err := getStatusCodeArg(args, 3)
	if err != nil {
		return nil, err
	}

	f := &filter{statusCode: statusCode, maxHits: maxHits}

	keyShards := getKeyShards(maxHits, maxShards)
	if keyShards > 1 {
		f.settings = ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			Group:      group + "." + strconv.Itoa(keyShards),
			MaxHits:    maxHits / keyShards,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewRoundRobinLookuper(uint64(keyShards)),
		}
	} else {
		f.settings = ratelimit.Settings{
			Type:       ratelimit.ClusterServiceRatelimit,
			Group:      group,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		}
	}
	log.Debugf("maxHits: %d, keyShards: %d", maxHits, keyShards)

	return f, nil
}

// getKeyShards returns number of key shards based on max hits and max allowed shards.
// Number of key shards k is the largest number from `[1, maxShards]` interval such that `maxHits % k == 0`
func getKeyShards(maxHits, maxShards int) int {
	for k := maxShards; k > 1; k-- {
		if maxHits%k == 0 {
			return k
		}
	}
	return 1
}

func clusterClientRatelimitFilter(args []interface{}) (*filter, error) {
	if !(len(args) == 3 || len(args) == 4) {
		return nil, filters.ErrInvalidFilterParameters
	}

	group, err := getStringArg(args[0])
	if err != nil {
		return nil, err
	}

	maxHits, err := getIntArg(args[1])
	if err != nil {
		return nil, err
	}

	timeWindow, err := getDurationArg(args[2])
	if err != nil {
		return nil, err
	}

	s := ratelimit.Settings{
		Type:          ratelimit.ClusterClientRatelimit,
		Group:         group,
		MaxHits:       maxHits,
		TimeWindow:    timeWindow,
		CleanInterval: 10 * timeWindow,
	}

	if len(args) > 3 {
		lookuperString, err := getStringArg(args[3])
		if err != nil {
			return nil, err
		}
		if strings.Contains(lookuperString, ",") {
			var lookupers []ratelimit.Lookuper
			for _, ls := range strings.Split(lookuperString, ",") {
				lookupers = append(lookupers, getLookuper(ls))
			}
			s.Lookuper = ratelimit.NewTupleLookuper(lookupers...)
		} else {
			s.Lookuper = getLookuper(lookuperString)
		}
	} else {
		s.Lookuper = ratelimit.NewXForwardedForLookuper()
	}

	return &filter{settings: s, statusCode: defaultStatusCode}, nil
}

func getLookuper(s string) ratelimit.Lookuper {
	headerName := http.CanonicalHeaderKey(s)
	if headerName == "X-Forwarded-For" {
		return ratelimit.NewXForwardedForLookuper()
	} else {
		return ratelimit.NewHeaderLookuper(headerName)
	}
}

func clientRatelimitFilter(args []interface{}) (*filter, error) {
	if !(len(args) == 2 || len(args) == 3) {
		return nil, filters.ErrInvalidFilterParameters
	}

	maxHits, err := getIntArg(args[0])
	if err != nil {
		return nil, err
	}

	timeWindow, err := getDurationArg(args[1])
	if err != nil {
		return nil, err
	}

	var lookuper ratelimit.Lookuper
	if len(args) > 2 {
		lookuperString, err := getStringArg(args[2])
		if err != nil {
			return nil, err
		}
		if strings.Contains(lookuperString, ",") {
			var lookupers []ratelimit.Lookuper
			for _, ls := range strings.Split(lookuperString, ",") {
				lookupers = append(lookupers, getLookuper(ls))
			}
			lookuper = ratelimit.NewTupleLookuper(lookupers...)
		} else {
			lookuper = ratelimit.NewHeaderLookuper(lookuperString)
		}
	} else {
		lookuper = ratelimit.NewXForwardedForLookuper()
	}

	return &filter{
		settings: ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    timeWindow,
			CleanInterval: 10 * timeWindow,
			Lookuper:      lookuper,
		},
		statusCode: defaultStatusCode,
	}, nil
}

func disableFilter([]interface{}) (*filter, error) {
	return &filter{
		settings: ratelimit.Settings{
			Type: ratelimit.DisableRatelimit,
		},
		statusCode: defaultStatusCode,
	}, nil
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f, err := s.createFilter(args)
	if f != nil {
		f.provider = s.provider
	}
	return f, err
}

func (s *spec) createFilter(args []interface{}) (*filter, error) {
	switch s.typ {
	case ratelimit.ServiceRatelimit:
		return serviceRatelimitFilter(args)
	case ratelimit.LocalRatelimit:
		log.Warning("ratelimit.LocalRatelimit is deprecated, please use ratelimit.ClientRatelimit")
		fallthrough
	case ratelimit.ClientRatelimit:
		return clientRatelimitFilter(args)
	case ratelimit.ClusterServiceRatelimit:
		return clusterRatelimitFilter(s.maxShards, args)
	case ratelimit.ClusterClientRatelimit:
		return clusterClientRatelimitFilter(args)
	default:
		return disableFilter(args)
	}
}

func getIntArg(a interface{}) (int, error) {
	if i, ok := a.(int); ok {
		return i, nil
	}

	if f, ok := a.(float64); ok {
		return int(f), nil
	}

	return 0, filters.ErrInvalidFilterParameters
}

func getStringArg(a interface{}) (string, error) {
	if s, ok := a.(string); ok {
		return s, nil
	}

	return "", filters.ErrInvalidFilterParameters
}

func getDurationArg(a interface{}) (time.Duration, error) {
	if s, ok := a.(string); ok {
		return time.ParseDuration(s)
	}

	i, err := getIntArg(a)
	return time.Duration(i) * time.Second, err
}

func getStatusCodeArg(args []interface{}, index int) (int, error) {
	// status code arg is optional so we return default status code but no error
	if len(args) <= index {
		return defaultStatusCode, nil
	}

	return getIntArg(args[index])
}

// Request checks ratelimit using filter settings and serves `429 Too Many Requests` response if limit is reached
func (f *filter) Request(ctx filters.FilterContext) {
	rateLimiter := f.provider.get(f.settings)
	if rateLimiter == nil {
		ctx.Logger().Errorf("RateLimiter is nil for settings: %s", f.settings)
		return
	}

	if f.settings.Lookuper == nil {
		ctx.Logger().Errorf("Lookuper is nil for settings: %s", f.settings)
		return
	}

	s := f.settings.Lookuper.Lookup(ctx.Request())
	if s == "" {
		ctx.Logger().Debugf("Lookuper found no data in request for settings: %s and request: %v", f.settings, ctx.Request())
		return
	}

	if !rateLimiter.Allow(ctx.Request().Context(), s) {
		maxHits := f.settings.MaxHits
		if f.maxHits != 0 {
			maxHits = f.maxHits
		}
		ctx.Serve(&http.Response{
			StatusCode: f.statusCode,
			Header:     ratelimit.Headers(maxHits, f.settings.TimeWindow, rateLimiter.RetryAfter(s)),
		})
	}
}

func (*filter) Response(filters.FilterContext) {}
