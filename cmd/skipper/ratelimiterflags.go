package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/zalando/skipper/ratelimit"
)

const ratelimitUsage = `set global rate limit settings, e.g. -ratelimit type=local,max-hits=20,time-window=60
	possible ratelimit properties:
	type: local/service/disabled (defaults to disabled)
	max-hits: the number of hits a ratelimiter can get
	time-window: the duration of the sliding window for the rate limiter
	(see also: https://godoc.org/github.com/zalando/skipper/ratelimit)`

const enableRatelimitUsage = `enable ratelimit`

type ratelimitFlags []ratelimit.Settings

var errInvalidRatelimitConfig = errors.New("invalid ratelimit config")

func (r *ratelimitFlags) String() string {
	s := make([]string, len(*r))
	for i, ri := range *r {
		s[i] = ri.String()
	}

	return strings.Join(s, "\n")
}

func (r *ratelimitFlags) Set(value string) error {
	var s ratelimit.Settings

	vs := strings.Split(value, ",")
	for _, vi := range vs {
		kv := strings.Split(vi, "=")
		if len(kv) != 2 {
			return errInvalidRatelimitConfig
		}

		switch kv[0] {
		case "type":
			switch kv[1] {
			case "local":
				s.Type = ratelimit.LocalRatelimit
			case "service":
				s.Type = ratelimit.ServiceRatelimit
			case "disabled":
				s.Type = ratelimit.DisableRatelimit
			default:
				return errInvalidRatelimitConfig
			}
		case "max-hits":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return err
			}
			s.MaxHits = i
		case "time-window":
			d, err := parseDurationFlag(kv[1])
			if err != nil {
				return err
			}
			s.TimeWindow = d
			s.CleanInterval = d * 10
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
