package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/zalando/skipper/ratelimit"
)

const ratelimitUsage = `set global rate limit settings, e.g. -ratelimit type=local,maxHits=20,timeWindow=60
	possible ratelimit properties:
	type: local/disabled (defaults to local)
	maxHits: the number of hits a local (meaning per instance) ratelimiter can get
	timeWindow: the duration of the sliding window for the rate limiter
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
			case "disabled":
				s.Type = ratelimit.DisableRatelimit
			default:
				return errInvalidRatelimitConfig
			}
		case "maxHits":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return err
			}
			s.MaxHits = i
		case "timeWindow":
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

	if s.Type == ratelimit.NonRatelimit {
		s.Type = ratelimit.DisableRatelimit
	}

	*r = append(*r, s)
	return nil
}
