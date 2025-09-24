package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/ratelimitbypass"
)

type bypassSpec struct {
	typ        ratelimit.RatelimitType
	provider   RatelimitProvider
	filterName string
	maxShards  int
	validator  *ratelimitbypass.BypassValidator
}

type bypassFilter struct {
	settings   ratelimit.Settings
	provider   RatelimitProvider
	statusCode int
	maxHits    int
	validator  *ratelimitbypass.BypassValidator
}

// NewClientRatelimitWithBypass creates a client rate limit filter with bypass capability
func NewClientRatelimitWithBypass(provider RatelimitProvider, bypassHeader, secretKey string, tokenExpiry time.Duration, ipWhitelist []string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &bypassSpec{
		typ:        ratelimit.ClientRatelimit,
		provider:   provider,
		filterName: "clientRatelimitWithBypass",
		validator:  validator,
	}
}

// NewRatelimitWithBypass creates a service rate limit filter with bypass capability
func NewRatelimitWithBypass(provider RatelimitProvider, bypassHeader, secretKey string, tokenExpiry time.Duration, ipWhitelist []string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &bypassSpec{
		typ:        ratelimit.ServiceRatelimit,
		provider:   provider,
		filterName: "ratelimitWithBypass",
		validator:  validator,
	}
}

// NewClusterClientRatelimitWithBypass creates a cluster client rate limit filter with bypass capability
func NewClusterClientRatelimitWithBypass(provider RatelimitProvider, bypassHeader, secretKey string, tokenExpiry time.Duration, ipWhitelist []string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &bypassSpec{
		typ:        ratelimit.ClusterClientRatelimit,
		provider:   provider,
		filterName: "clusterClientRatelimitWithBypass",
		validator:  validator,
	}
}

// NewClusterRatelimitWithBypass creates a cluster service rate limit filter with bypass capability
func NewClusterRatelimitWithBypass(provider RatelimitProvider, bypassHeader, secretKey string, tokenExpiry time.Duration, ipWhitelist []string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &bypassSpec{
		typ:        ratelimit.ClusterServiceRatelimit,
		provider:   provider,
		filterName: "clusterRatelimitWithBypass",
		maxShards:  1,
		validator:  validator,
	}
}

// NewDisableRatelimitWithBypass creates a disable rate limit filter with bypass capability
func NewDisableRatelimitWithBypass(provider RatelimitProvider, bypassHeader, secretKey string, tokenExpiry time.Duration, ipWhitelist []string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
		IPWhitelist:  ipWhitelist,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &bypassSpec{
		typ:        ratelimit.DisableRatelimit,
		provider:   provider,
		filterName: "disableRatelimitWithBypass",
		validator:  validator,
	}
}

func (s *bypassSpec) Name() string {
	return s.filterName
}

func (s *bypassSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f, err := s.createBypassFilter(args)
	if f != nil {
		f.provider = s.provider
		f.validator = s.validator
	}
	return f, err
}

func (s *bypassSpec) createBypassFilter(args []interface{}) (*bypassFilter, error) {
	switch s.typ {
	case ratelimit.ServiceRatelimit:
		return s.serviceRatelimitBypassFilter(args)
	case ratelimit.ClientRatelimit:
		return s.clientRatelimitBypassFilter(args)
	case ratelimit.ClusterServiceRatelimit:
		return s.clusterRatelimitBypassFilter(s.maxShards, args)
	case ratelimit.ClusterClientRatelimit:
		return s.clusterClientRatelimitBypassFilter(args)
	default:
		return s.disableBypassFilter(args)
	}
}

func (s *bypassSpec) serviceRatelimitBypassFilter(args []interface{}) (*bypassFilter, error) {
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

	return &bypassFilter{
		settings: ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
		statusCode: statusCode,
	}, nil
}

func (s *bypassSpec) clientRatelimitBypassFilter(args []interface{}) (*bypassFilter, error) {
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

	return &bypassFilter{
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

func (s *bypassSpec) clusterRatelimitBypassFilter(maxShards int, args []interface{}) (*bypassFilter, error) {
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

	f := &bypassFilter{statusCode: statusCode, maxHits: maxHits}

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

	return f, nil
}

func (s *bypassSpec) clusterClientRatelimitBypassFilter(args []interface{}) (*bypassFilter, error) {
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

	settings := ratelimit.Settings{
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
			settings.Lookuper = ratelimit.NewTupleLookuper(lookupers...)
		} else {
			settings.Lookuper = getLookuper(lookuperString)
		}
	} else {
		settings.Lookuper = ratelimit.NewXForwardedForLookuper()
	}

	return &bypassFilter{settings: settings, statusCode: defaultStatusCode}, nil
}

func (s *bypassSpec) disableBypassFilter([]interface{}) (*bypassFilter, error) {
	return &bypassFilter{
		settings: ratelimit.Settings{
			Type: ratelimit.DisableRatelimit,
		},
		statusCode: defaultStatusCode,
	}, nil
}

// Request checks if bypass conditions are met, otherwise applies rate limiting
func (f *bypassFilter) Request(ctx filters.FilterContext) {
	// Check if request should bypass rate limiting
	if f.validator != nil && f.validator.ShouldBypass(ctx.Request()) {
		log.Debugf("Request bypassed rate limiting")
		return
	}

	// Apply normal rate limiting
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

func (*bypassFilter) Response(filters.FilterContext) {}
