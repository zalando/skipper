package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/ratelimit"
)

const ratelimitsUsage = `set global rate limit settings, e.g. -ratelimits type=client,max-hits=20,time-window=60s
	possible ratelimit properties:
	type: client/service/clusterClient/clusterService/disabled (defaults to disabled)
	max-hits: the number of hits a ratelimiter can get
	time-window: the duration of the sliding window for the rate limiter
	group: defines the ratelimit group, which can be the same for different routes.
	(see also: https://pkg.go.dev/github.com/zalando/skipper/ratelimit)`

const enableRatelimitsUsage = `enable ratelimits`

type ratelimitFlags []ratelimit.Settings

var errInvalidRatelimitConfig = errors.New("invalid ratelimit config (allowed values are: client, service or disabled)")

func (r ratelimitFlags) String() string {
	s := make([]string, len(r))
	for i, ri := range r {
		s[i] = ri.String()
	}

	return strings.Join(s, "\n")
}

func (r *ratelimitFlags) Set(value string) error {
	var s ratelimit.Settings

	vs := strings.SplitSeq(value, ",")
	for vi := range vs {
		k, v, found := strings.Cut(vi, "=")
		if !found {
			return errInvalidRatelimitConfig
		}

		switch k {
		case "type":
			switch v {
			case "local":
				log.Warning("LocalRatelimit is deprecated, please use ClientRatelimit instead")
				fallthrough
			case "client":
				s.Type = ratelimit.ClientRatelimit
			case "service":
				s.Type = ratelimit.ServiceRatelimit
			case "clusterClient":
				s.Type = ratelimit.ClusterClientRatelimit
			case "clusterService":
				s.Type = ratelimit.ClusterServiceRatelimit
			case "disabled":
				s.Type = ratelimit.DisableRatelimit
			default:
				return errInvalidRatelimitConfig
			}
		case "max-hits":
			i, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			s.MaxHits = i
		case "time-window":
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}
			s.TimeWindow = d
			s.CleanInterval = d * 10
		case "group":
			s.Group = v
		default:
			return errInvalidRatelimitConfig
		}
	}

	if s.Type == ratelimit.NoRatelimit {
		s.Type = ratelimit.DisableRatelimit
	}

	*r = append(*r, s)
	return nil
}

func (r *ratelimitFlags) UnmarshalYAML(unmarshal func(any) error) error {
	var rateLimitSettings ratelimit.Settings
	if err := unmarshal(&rateLimitSettings); err != nil {
		return err
	}

	rateLimitSettings.CleanInterval = rateLimitSettings.TimeWindow * 10

	*r = append(*r, rateLimitSettings)
	return nil
}
