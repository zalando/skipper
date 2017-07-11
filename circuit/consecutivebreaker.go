package circuit

import (
	log "github.com/sirupsen/logrus"
	"github.com/sony/gobreaker"
)

type consecutiveBreaker struct {
	settings BreakerSettings
	open     bool
	gb       *gobreaker.TwoStepCircuitBreaker
}

func newConsecutive(s BreakerSettings) *consecutiveBreaker {
	b := &consecutiveBreaker{
		settings: s,
	}

	b.gb = gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:        s.Host,
		MaxRequests: uint32(s.HalfOpenRequests),
		Timeout:     s.Timeout,
		ReadyToTrip: b.readyToTrip,
	})

	return b
}

func (b *consecutiveBreaker) readyToTrip(c gobreaker.Counts) bool {
	b.open = int(c.ConsecutiveFailures) >= b.settings.Failures
	if b.open {
		log.Infof("circuit breaker open: %v", b.settings)
	}

	return b.open
}

func (b *consecutiveBreaker) Allow() (func(bool), bool) {
	done, err := b.gb.Allow()

	// this error can only indicate that the breaker is not closed
	closed := err == nil

	if !closed {
		return nil, false
	}

	if b.open {
		b.open = false
		log.Infof("circuit breaker closed: %v", b.settings)
	}

	return done, true
}
