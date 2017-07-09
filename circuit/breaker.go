package circuit

import (
	"strconv"
	"strings"
	"time"
)

// TODO:
// - in case of the rate breaker, there are unnecessary synchronization steps due to the 3rd party gobreaker
// - introduce a TTL in the rate breaker for the stale samplers

type BreakerType int

const (
	BreakerNone BreakerType = iota
	ConsecutiveFailures
	FailureRate
	BreakerDisabled
)

type BreakerSettings struct {
	Type             BreakerType
	Host             string
	Window, Failures int
	Timeout          time.Duration
	HalfOpenRequests int
	IdleTTL          time.Duration
}

type breakerImplementation interface {
	Allow() (func(bool), bool)
}

type voidBreaker struct{}

// represents a single circuit breaker
type Breaker struct {
	settings   BreakerSettings
	ts         time.Time
	prev, next *Breaker
	impl       breakerImplementation
}

func (to BreakerSettings) mergeSettings(from BreakerSettings) BreakerSettings {
	if to.Type == BreakerNone {
		to.Type = from.Type

		if from.Type == ConsecutiveFailures {
			to.Failures = from.Failures
		}

		if from.Type == FailureRate {
			to.Window = from.Window
			to.Failures = from.Failures
		}
	}

	if to.Timeout == 0 {
		to.Timeout = from.Timeout
	}

	if to.HalfOpenRequests == 0 {
		to.HalfOpenRequests = from.HalfOpenRequests
	}

	if to.IdleTTL == 0 {
		to.IdleTTL = from.IdleTTL
	}

	return to
}

func (s BreakerSettings) String() string {
	var ss []string

	switch s.Type {
	case ConsecutiveFailures:
		ss = append(ss, "type=consecutive")
	case FailureRate:
		ss = append(ss, "type=rate")
	case BreakerDisabled:
		return "disabled"
	default:
		return "none"
	}

	if s.Host != "" {
		ss = append(ss, "host="+s.Host)
	}

	if s.Type == FailureRate && s.Window > 0 {
		ss = append(ss, "window="+strconv.Itoa(s.Window))
	}

	if s.Failures > 0 {
		ss = append(ss, "failures="+strconv.Itoa(s.Failures))
	}

	if s.Timeout > 0 {
		ss = append(ss, "timeout="+s.Timeout.String())
	}

	if s.HalfOpenRequests > 0 {
		ss = append(ss, "half-open-requests="+strconv.Itoa(s.HalfOpenRequests))
	}

	if s.IdleTTL > 0 {
		ss = append(ss, "idle-ttl="+s.IdleTTL.String())
	}

	return strings.Join(ss, ",")
}

func (b voidBreaker) Allow() (func(bool), bool) {
	return func(bool) {}, true
}

func newBreaker(s BreakerSettings) *Breaker {
	var impl breakerImplementation
	switch s.Type {
	case ConsecutiveFailures:
		impl = newConsecutive(s)
	case FailureRate:
		impl = newRate(s)
	default:
		impl = voidBreaker{}
	}

	return &Breaker{
		settings: s,
		impl:     impl,
	}
}

func (b *Breaker) Allow() (func(bool), bool) {
	return b.impl.Allow()
}

func (b *Breaker) idle(now time.Time) bool {
	return now.Sub(b.ts) > b.settings.IdleTTL
}
