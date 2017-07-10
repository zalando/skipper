package circuit

import "github.com/sony/gobreaker"

type consecutiveBreaker struct {
	gb *gobreaker.TwoStepCircuitBreaker
}

func newConsecutive(s BreakerSettings) *consecutiveBreaker {
	return &consecutiveBreaker{
		gb: gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
			Name:        s.Host,
			MaxRequests: uint32(s.HalfOpenRequests),
			Timeout:     s.Timeout,
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return int(c.ConsecutiveFailures) >= s.Failures
			},
		}),
	}
}

func (b *consecutiveBreaker) Allow() (func(bool), bool) {
	done, err := b.gb.Allow()

	// this error can only indicate that the breaker is not closed
	if err != nil {
		return nil, false
	}

	return done, true
}
