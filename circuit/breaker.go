package circuit

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BreakerType defines the type of the used breaker: consecutive, rate or disabled.
type BreakerType int

func (b *BreakerType) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}

	switch value {
	case "consecutive":
		*b = ConsecutiveFailures
	case "rate":
		*b = FailureRate
	case "disabled":
		*b = BreakerDisabled
	default:
		return fmt.Errorf("invalid breaker type %v (allowed values are: consecutive, rate or disabled)", value)
	}

	return nil
}

const (
	BreakerNone BreakerType = iota
	ConsecutiveFailures
	FailureRate
	BreakerDisabled
)

// BreakerSettings contains the settings for individual circuit breakers.
//
// See the package overview for the detailed merging/overriding rules of the settings and for the meaning of the
// individual fields.
type BreakerSettings struct {
	Type             BreakerType   `yaml:"type"`
	Host             string        `yaml:"host"`
	Window           int           `yaml:"window"`
	Failures         int           `yaml:"failures"`
	Timeout          time.Duration `yaml:"timeout"`
	HalfOpenRequests int           `yaml:"half-open-requests"`
	IdleTTL          time.Duration `yaml:"idle-ttl"`
}

type breakerImplementation interface {
	Allow() (func(bool), bool)
}

type voidBreaker struct{}

// Breaker represents a single circuit breaker for a particular set of settings.
//
// Use the Get() method of the Registry to request fully initialized breakers.
type Breaker struct {
	settings BreakerSettings
	ts       time.Time
	impl     breakerImplementation
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

// String returns the string representation of a particular set of settings.
//
//lint:ignore ST1016 "s" makes sense here and mergeSettings has "to"
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

// Allow returns true if the breaker is in the closed state and a callback function for reporting the outcome of
// the operation. The callback expects true values if the outcome of the request was successful. Allow may not
// return a callback function when the state is open.
func (b *Breaker) Allow() (func(bool), bool) {
	return b.impl.Allow()
}

func (b *Breaker) idle(now time.Time) bool {
	return now.Sub(b.ts) > b.settings.IdleTTL
}
