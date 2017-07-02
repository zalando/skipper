package circuit

import (
	"sync"
	"time"
)

type BreakerType int

const (
	BreakerNone BreakerType = iota
	ConsecutiveFailures
	FailureRate
)

type BreakerSettings struct {
	Type             BreakerType
	Host             string
	Window, Failures int
	Timeout          time.Duration
	HalfOpenRequests int
	disabled         bool
}

type breakerImplementation interface {
	Allow() (func(bool), bool)
	Closed() bool
}

// represents a single circuit breaker
type Breaker struct {
	settings   BreakerSettings
	ts         time.Time
	prev, next *Breaker
	impl       breakerImplementation
	mx         *sync.Mutex
	sampler    *binarySampler
}

func applySettings(to, from BreakerSettings) BreakerSettings {
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

	return to
}

func newBreaker(s BreakerSettings) *Breaker {
	b := &Breaker{
		settings: s,
		mx:       &sync.Mutex{},
	}
	b.impl = newGobreaker(s, b.readyToTrip)
	return b
}

func (b *Breaker) rateReadyToTrip() bool {
	b.mx.Lock()
	defer b.mx.Unlock()

	if b.sampler == nil {
		return false
	}

	ready := b.sampler.count >= b.settings.Failures
	if ready {
		b.sampler = nil
	}

	return ready
}

func (b *Breaker) readyToTrip(failures int) bool {
	switch b.settings.Type {
	case ConsecutiveFailures:
		return failures >= b.settings.Failures
	case FailureRate:
		return b.rateReadyToTrip()
	default:
		return false
	}
}

func (b *Breaker) tick(success bool) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if b.sampler == nil {
		if !b.impl.Closed() {
			return
		}

		b.sampler = newBinarySampler(b.settings.Window)
	}

	// count the failures in closed state
	b.sampler.tick(!success)
}

func (b *Breaker) rateAllow() (func(bool), bool) {
	done, ok := b.impl.Allow()
	if !ok {
		return nil, false
	}

	return func(success bool) {
		b.tick(success)
		done(success)
	}, true
}

func (b *Breaker) Allow() (func(bool), bool) {
	switch b.settings.Type {
	case ConsecutiveFailures:
		return b.impl.Allow()
	case FailureRate:
		return b.rateAllow()
	default:
		return nil, false
	}
}
