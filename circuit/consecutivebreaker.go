package circuit

import (
	log "github.com/sirupsen/logrus"
	"github.com/sony/gobreaker"
)

type consecutiveBreaker struct {
	settings BreakerSettings
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
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Infof("circuit breaker %v went from %v to %v", name, from.String(), to.String())
		},
	})

	return b
}

func (b *consecutiveBreaker) readyToTrip(c gobreaker.Counts) bool {
	return int(c.ConsecutiveFailures) >= b.settings.Failures
}

func (b *consecutiveBreaker) Allow() (func(bool), bool) {
	done, err := b.gb.Allow()

	// this error can only indicate that the breaker is not closed
	closed := err == nil

	if !closed {
		return nil, false
	}
	return done, true
}
