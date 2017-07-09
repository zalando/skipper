package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/zalando/skipper/circuit"
)

const breakerUsage = `set global or host specific circuit breakers, e.g. -breaker type=rate,host=www.example.org,window=300,failures=30
	possible breaker properties:
	type: consecutive/rate/disabled (defaults to consecutive)
	host: a host name that overrides the global for a host
	failures: the number of failures for consecutive or rate breakers
	window: the size of the sliding window for the rate breaker
	timeout: duration string or milliseconds while the breaker stays open
	half-open-requests: the number of requests in half-open state to succeed before getting closed again
	idle-ttl: duration string or milliseconds after the breaker is considered idle and reset
	(see also: https://godoc.org/github.com/zalando/skipper/circuit)`

type breakerFlags []circuit.BreakerSettings

var errInvalidBreakerConfig = errors.New("invalid breaker config")

func (b *breakerFlags) String() string {
	s := make([]string, len(*b))
	for i, bi := range *b {
		s[i] = bi.String()
	}

	return strings.Join(s, "\n")
}

func (b *breakerFlags) Set(value string) error {
	var s circuit.BreakerSettings

	vs := strings.Split(value, ",")
	for _, vi := range vs {
		kv := strings.Split(vi, "=")
		if len(kv) != 2 {
			return errInvalidBreakerConfig
		}

		switch kv[0] {
		case "type":
			switch kv[1] {
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
			s.Host = kv[1]
		case "window":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return err
			}

			s.Window = i
		case "failures":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return err
			}

			s.Failures = i
		case "timeout":
			d, err := parseDurationFlag(kv[1])
			if err != nil {
				return err
			}

			s.Timeout = d
		case "half-open-requests":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return err
			}

			s.HalfOpenRequests = i
		case "idle-ttl":
			d, err := parseDurationFlag(kv[1])
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
