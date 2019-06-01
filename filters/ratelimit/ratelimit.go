/*
Package ratelimit provides filters to control the rate limitter settings on the route level.

For detailed documentation of the ratelimit, see https://godoc.org/github.com/zalando/skipper/ratelimit.
*/
package ratelimit

import (
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

// RetryAfterKey is used as key in the context state bag
const RetryAfterKey = "#ratelimitretryafter"

// RouteSettingsKey is used as key in the context state bag
const RouteSettingsKey = "#ratelimitsettings"

type spec struct {
	typ        ratelimit.RatelimitType
	filterName string
}

type filter struct {
	settings ratelimit.Settings
}

// NewLocalRatelimit is *DEPRECATED*, use NewClientRatelimit, instead
func NewLocalRatelimit() filters.Spec {
	log.Warning("NewLocalRatelimit is deprecated, please use NewClientRatelimit")
	return &spec{typ: ratelimit.LocalRatelimit, filterName: ratelimit.LocalRatelimitName}
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
//    backendHealthcheck: Path("/healthcheck")
//    -> clientRatelimit(20, "1m")
//    -> "https://foo.backend.net";
//
// Example rate limit per Authorization Header:
//
//    login: Path("/login")
//    -> clientRatelimit(3, "1m", "Authorization")
//    -> "https://login.backend.net";
func NewClientRatelimit() filters.Spec {
	return &spec{typ: ratelimit.ClientRatelimit, filterName: ratelimit.ClientRatelimitName}
}

// NewRatelimit creates a service rate limiting, that is
// only aware of itself. If you have 5 instances with 20 req/s, then
// it would at max allow 100 req/s to the backend.
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> ratelimit(20, "1s")
//    -> "https://foo.backend.net";
func NewRatelimit() filters.Spec {
	return &spec{typ: ratelimit.ServiceRatelimit, filterName: ratelimit.ServiceRatelimitName}
}

// NewClusterRatelimit creates a rate limiting that is aware of the
// other instances. The value given here should be the combined rate
// of all instances. The ratelimit group parameter can be used to
// select the same ratelimit group across one or more routes.
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> clusterRatelimit("groupA", 200, "1m")
//    -> "https://foo.backend.net";
//
func NewClusterRateLimit() filters.Spec {
	return &spec{typ: ratelimit.ClusterServiceRatelimit, filterName: ratelimit.ClusterServiceRatelimitName}
}

// NewClusterClientRatelimit creates a rate limiting that is aware of
// the other instances. The value given here should be the combined
// rate of all instances. The ratelimit group parameter can be used to
// select the same ratelimit group across one or more routes.
//
// Example:
//
//    backendHealthcheck: Path("/login")
//    -> clusterClientRatelimit("groupB", 20, "1h")
//    -> "https://foo.backend.net";
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
//    backendHealthcheck: Path("/login")
//    -> clusterClientRatelimit("groupC", 20, "1h", "Authorization")
//    -> "https://foo.backend.net";
//
func NewClusterClientRateLimit() filters.Spec {
	return &spec{typ: ratelimit.ClusterClientRatelimit, filterName: ratelimit.ClusterClientRatelimitName}
}

// NewDisableRatelimit disables rate limiting
//
// Example:
//
//    backendHealthcheck: Path("/healthcheck")
//    -> disableRatelimit()
//    -> "https://foo.backend.net";
func NewDisableRatelimit() filters.Spec {
	return &spec{typ: ratelimit.DisableRatelimit, filterName: ratelimit.DisableRatelimitName}
}

func (s *spec) Name() string {
	return s.filterName
}

func serviceRatelimitFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
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

	return &filter{
		settings: ratelimit.Settings{
			Type:       ratelimit.ServiceRatelimit,
			MaxHits:    maxHits,
			TimeWindow: timeWindow,
			Lookuper:   ratelimit.NewSameBucketLookuper(),
		},
	}, nil
}

func clusterRatelimitFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 3 {
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
		Type:       ratelimit.ClusterServiceRatelimit,
		Group:      group,
		MaxHits:    maxHits,
		TimeWindow: timeWindow,
		Lookuper:   ratelimit.NewSameBucketLookuper(),
	}

	return &filter{settings: s}, nil
}

func clusterClientRatelimitFilter(args []interface{}) (filters.Filter, error) {
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

	return &filter{settings: s}, nil
}

func getLookuper(s string) ratelimit.Lookuper {
	headerName := http.CanonicalHeaderKey(s)
	if headerName == "X-Forwarded-For" {
		return ratelimit.NewXForwardedForLookuper()
	} else {
		return ratelimit.NewHeaderLookuper(headerName)
	}
}

func clientRatelimitFilter(args []interface{}) (filters.Filter, error) {
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
	}, nil
}

func disableFilter([]interface{}) (filters.Filter, error) {
	return &filter{
		settings: ratelimit.Settings{
			Type: ratelimit.DisableRatelimit,
		},
	}, nil
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	switch s.typ {
	case ratelimit.ServiceRatelimit:
		return serviceRatelimitFilter(args)
	case ratelimit.LocalRatelimit:
		log.Warning("ratelimit.LocalRatelimit is deprecated, please use ratelimit.ClientRatelimit")
		fallthrough
	case ratelimit.ClientRatelimit:
		return clientRatelimitFilter(args)
	case ratelimit.ClusterServiceRatelimit:
		return clusterRatelimitFilter(args)
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

// Request stores the configured ratelimit.Settings in the state bag,
// such that it can be used in the proxy to check ratelimit.
func (f *filter) Request(ctx filters.FilterContext) {
	if settings, ok := ctx.StateBag()[RouteSettingsKey].([]ratelimit.Settings); ok {
		ctx.StateBag()[RouteSettingsKey] = append(settings, f.settings)
	} else {
		ctx.StateBag()[RouteSettingsKey] = []ratelimit.Settings{f.settings}
	}
}

func (*filter) Response(filters.FilterContext) {}
