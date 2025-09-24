/*
Package ratelimit provides filters to control the rate limiter settings on the route level.

For detailed documentation of the ratelimit, see https://godoc.org/github.com/zalando/skipper/ratelimit.
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
	"github.com/zalando/skipper/ratelimitbypass"
)

const defaultStatusCode = http.StatusTooManyRequests

type spec struct {
	typ            ratelimit.RatelimitType
	provider       RatelimitProvider
	filterName     string
	maxShards      int
	globalBypasser *ratelimitbypass.BypassValidator
}

type filter struct {
	settings        ratelimit.Settings
	provider        RatelimitProvider
	statusCode      int
	maxHits         int // overrides settings.MaxHits
	bypassValidator *ratelimitbypass.BypassValidator
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

// NewClientRatelimit creates a instance based client rate limit.  If
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

// NewClientRatelimitWithGlobalBypass creates a client rate limit filter with global bypass configuration
func NewClientRatelimitWithGlobalBypass(provider RatelimitProvider, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return &spec{typ: ratelimit.ClientRatelimit, provider: provider, filterName: filters.ClientRatelimitName, globalBypasser: globalBypasser}
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

// NewRatelimitWithGlobalBypass creates a service rate limit filter with global bypass configuration
func NewRatelimitWithGlobalBypass(provider RatelimitProvider, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return &spec{typ: ratelimit.ServiceRatelimit, provider: provider, filterName: filters.RatelimitName, globalBypasser: globalBypasser}
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

// NewClusterRateLimitWithGlobalBypass creates a cluster rate limit filter with global bypass configuration
func NewClusterRateLimitWithGlobalBypass(provider RatelimitProvider, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return NewShardedClusterRateLimitWithGlobalBypass(provider, 1, globalBypasser)
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

// NewShardedClusterRateLimitWithGlobalBypass creates a sharded cluster rate limit filter with global bypass configuration
func NewShardedClusterRateLimitWithGlobalBypass(provider RatelimitProvider, maxGroupShards int, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return &spec{typ: ratelimit.ClusterServiceRatelimit, provider: provider, filterName: filters.ClusterRatelimitName, maxShards: maxGroupShards, globalBypasser: globalBypasser}
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

// NewClusterClientRateLimitWithGlobalBypass creates a cluster client rate limit filter with global bypass configuration
func NewClusterClientRateLimitWithGlobalBypass(provider RatelimitProvider, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return &spec{typ: ratelimit.ClusterClientRatelimit, provider: provider, filterName: filters.ClusterClientRatelimitName, globalBypasser: globalBypasser}
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

// NewDisableRatelimitWithGlobalBypass creates a disable rate limit filter with global bypass configuration
func NewDisableRatelimitWithGlobalBypass(provider RatelimitProvider, globalBypasser *ratelimitbypass.BypassValidator) filters.Spec {
	return &spec{typ: ratelimit.DisableRatelimit, provider: provider, filterName: filters.DisableRatelimitName, globalBypasser: globalBypasser}
}

func (s *spec) Name() string {
	return s.filterName
}

func serviceRatelimitFilter(args []interface{}) (*filter, error) {
	if !(len(args) >= 2 && len(args) <= 7) { // Support original 2-3 args + 4 bypass args
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

	// Check for bypass configuration starting after status code
	bypassStartIndex := 3
	if len(args) == 2 {
		bypassStartIndex = 2
	}

	bypassValidator := parseBypassConfig(args, bypassStartIndex)

	return &filter{
		settings: ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		statusCode:      statusCode,
		bypassValidator: bypassValidator,
	}, nil
}

func clusterRatelimitFilter(maxShards int, args []interface{}) (*filter, error) {
	if !(len(args) >= 3 && len(args) <= 8) { // Support original 3-4 args + bypass args
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

	// Check for bypass configuration starting after status code
	bypassStartIndex := 4
	if len(args) == 3 {
		bypassStartIndex = 3
	}

	bypassValidator := parseBypassConfig(args, bypassStartIndex)

	f := &filter{statusCode: statusCode, maxHits: maxHits, bypassValidator: bypassValidator}

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
	if !(len(args) >= 3 && len(args) <= 8) { // Support original 3-4 args + 4 bypass args
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

	var bypassStartIndex int
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
		bypassStartIndex = 4
	} else {
		s.Lookuper = ratelimit.NewXForwardedForLookuper()
		bypassStartIndex = 3
	}

	// Check for bypass configuration
	var bypassValidator *ratelimitbypass.BypassValidator
	if len(args) > bypassStartIndex {
		bypassValidator = parseBypassConfig(args, bypassStartIndex)
	}

	return &filter{settings: s, statusCode: defaultStatusCode, bypassValidator: bypassValidator}, nil
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
	if !(len(args) >= 2 && len(args) <= 7) { // Support original 2-3 args + 4 bypass args
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
	var bypassStartIndex int

	// Check if we have bypass parameters by looking for the expected pattern:
	// If we have 5+ args, assume: maxHits, timeWindow, [lookuper], bypassHeader, secretKey, tokenExpiry, [ipWhitelist]
	// If we have 3-4 args, could be: maxHits, timeWindow, lookuper OR maxHits, timeWindow, bypassHeader, secretKey
	// If we have 2 args, it's just: maxHits, timeWindow

	hasLookuper := false
	if len(args) >= 5 {
		// Definitely has bypass params, check if position 2 is lookuper
		if lookuperString, err := getStringArg(args[2]); err == nil {
			// Check if this looks like a bypass header name or a lookuper
			// Bypass headers typically contain "Bypass" or "Token" and are single headers
			// Lookupers can be comma-separated header names
			isLikelyBypassHeader := strings.Contains(strings.ToLower(lookuperString), "bypass") ||
				strings.Contains(strings.ToLower(lookuperString), "token") ||
				(strings.HasPrefix(lookuperString, "X-") && !strings.Contains(lookuperString, ","))

			if !isLikelyBypassHeader {
				hasLookuper = true
				if strings.Contains(lookuperString, ",") {
					var lookupers []ratelimit.Lookuper
					for _, ls := range strings.Split(lookuperString, ",") {
						lookupers = append(lookupers, getLookuper(ls))
					}
					lookuper = ratelimit.NewTupleLookuper(lookupers...)
				} else {
					lookuper = ratelimit.NewHeaderLookuper(lookuperString)
				}
				bypassStartIndex = 3
			} else {
				lookuper = ratelimit.NewXForwardedForLookuper()
				bypassStartIndex = 2
			}
		}
	} else if len(args) == 3 {
		// Could be lookuper only, no bypass
		if lookuperString, err := getStringArg(args[2]); err == nil {
			if strings.Contains(lookuperString, ",") {
				var lookupers []ratelimit.Lookuper
				for _, ls := range strings.Split(lookuperString, ",") {
					lookupers = append(lookupers, getLookuper(ls))
				}
				lookuper = ratelimit.NewTupleLookuper(lookupers...)
			} else {
				lookuper = ratelimit.NewHeaderLookuper(lookuperString)
			}
			hasLookuper = true
		}
	}

	if !hasLookuper {
		lookuper = ratelimit.NewXForwardedForLookuper()
		if bypassStartIndex == 0 {
			bypassStartIndex = 2
		}
	}

	var bypassValidator *ratelimitbypass.BypassValidator
	if len(args) > bypassStartIndex {
		bypassValidator = parseBypassConfig(args, bypassStartIndex)
	}

	return &filter{
		settings: ratelimit.Settings{
			Type:          ratelimit.ClientRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    timeWindow,
			CleanInterval: 10 * timeWindow,
			Lookuper:      lookuper,
		},
		statusCode:      defaultStatusCode,
		bypassValidator: bypassValidator,
	}, nil
}

func disableFilter(args []interface{}) (*filter, error) {
	// Even for disable filter, we might want bypass config for consistency
	var bypassValidator *ratelimitbypass.BypassValidator
	if len(args) >= 4 {
		bypassValidator = parseBypassConfig(args, 0)
	}

	return &filter{
		settings: ratelimit.Settings{
			Type: ratelimit.DisableRatelimit,
		},
		statusCode:      defaultStatusCode,
		bypassValidator: bypassValidator,
	}, nil
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f, err := s.createFilter(args)
	if f != nil {
		f.provider = s.provider
		// Apply global bypass config if no local bypass config is set
		if f.bypassValidator == nil && s.globalBypasser != nil {
			f.bypassValidator = s.globalBypasser
		}
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

// parseBypassConfig extracts bypass configuration from filter arguments
// Returns nil if no bypass config is found
func parseBypassConfig(args []interface{}, startIndex int) *ratelimitbypass.BypassValidator {
	// Check if we have enough arguments for bypass config
	// Expected: bypassHeader, secretKey, tokenExpiry, ipWhitelist
	if len(args) < startIndex+3 {
		return nil
	}

	bypassHeader, err := getStringArg(args[startIndex])
	if err != nil {
		return nil
	}

	secretKey, err := getStringArg(args[startIndex+1])
	if err != nil {
		return nil
	}

	var tokenExpiry time.Duration
	if durStr, ok := args[startIndex+2].(string); ok {
		tokenExpiry, err = time.ParseDuration(durStr)
		if err != nil {
			return nil
		}
	} else {
		return nil
	}

	var ipWhitelist []string
	if len(args) > startIndex+3 {
		if ipWhitelistStr, err := getStringArg(args[startIndex+3]); err == nil {
			ipWhitelist = ratelimitbypass.ParseIPWhitelist(ipWhitelistStr)
		}
	}

	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}

	return ratelimitbypass.NewBypassValidator(config)
}

// Request checks ratelimit using filter settings and serves `429 Too Many Requests` response if limit is reached
func (f *filter) Request(ctx filters.FilterContext) {
	// Check if request should bypass rate limiting
	if f.bypassValidator != nil && f.bypassValidator.ShouldBypass(ctx.Request()) {
		log.Debugf("Request bypassed rate limiting")
		return
	}

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
