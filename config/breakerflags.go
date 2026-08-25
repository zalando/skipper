package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/zalando/skipper/circuit"
)

const breakerUsage = `set global or host specific circuit breakers, e.g. -breaker type=rate,host=www.example.org,window=300s,failures=30
	possible breaker properties:
	type: consecutive/rate/disabled (defaults to consecutive)
	host: a host name that overrides the global for a host
	failures: the number of failures for consecutive or rate breakers
	window: the size of the sliding window for the rate breaker
	timeout: duration string or milliseconds while the breaker stays open
	half-open-requests: the number of requests in half-open state to succeed before getting closed again
	idle-ttl: duration string or milliseconds after the breaker is considered idle and reset
	(see also: https://pkg.go.dev/github.com/zalando/skipper/circuit)`

const enableBreakersUsage = `enable breakers to be set from filters without providing global or host settings (equivalent to: -breaker type=disabled)`

type breakerFlags []circuit.BreakerSettings

var errInvalidBreakerConfig = errors.New("invalid breaker config (allowed values are: consecutive, rate or disabled)")

func (b breakerFlags) String() string {
	s := make([]string, len(b))
	for i, bi := range b {
		s[i] = bi.String()
	}

	return strings.Join(s, "\n")
}

func (b *breakerFlags) Set(value string) error {
	var s circuit.BreakerSettings

	vs := strings.SplitSeq(value, ",")
	for vi := range vs {
		k, v, found := strings.Cut(vi, "=")
		if !found {
			return errInvalidBreakerConfig
		}

		switch k {
		case "type":
			switch v {
			case "consecutive":
				s.Type = circuit.ConsecutiveFailures
			case "rate":
				s.Type = circuit.FailureRate
			case "disabled":
				s.Type = circuit.BreakerDisabled
			default:
				return errInvalidBreakerConfig
			}
		case "host":
			s.Host = v
		case "window":
			i, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			s.Window = i
		case "failures":
			i, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			s.Failures = i
		case "timeout":
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}

			s.Timeout = d
		case "half-open-requests":
			i, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			s.HalfOpenRequests = i
		case "idle-ttl":
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}

			s.IdleTTL = d
		default:
			return errInvalidBreakerConfig
		}
	}

	if s.Type == circuit.BreakerNone {
		s.Type = circuit.ConsecutiveFailures
	}

	*b = append(*b, s)
	return nil
}

func (b *breakerFlags) UnmarshalYAML(unmarshal func(any) error) error {
	var breakerSettings circuit.BreakerSettings
	if err := unmarshal(&breakerSettings); err != nil {
		return err
	}

	*b = append(*b, breakerSettings)
	return nil
}
