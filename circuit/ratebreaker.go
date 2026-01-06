package circuit

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/sony/gobreaker"
)

// TODO:
// in case of the rate breaker, there are unnecessary synchronization steps due to the 3rd party gobreaker. If
// the sliding window was part of the implementation of the individual breakers, this additional synchronization
// would not be required.

type rateBreaker struct {
	settings BreakerSettings
	mu       sync.Mutex
	sampler  *binarySampler
	gb       *gobreaker.TwoStepCircuitBreaker
}

func newRate(s BreakerSettings) *rateBreaker {
	b := &rateBreaker{
		settings: s,
	}

	b.gb = gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:        s.Host,
		MaxRequests: uint32(s.HalfOpenRequests),
		Timeout:     s.Timeout,
		ReadyToTrip: func(gobreaker.Counts) bool { return b.readyToTrip() },
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Infof("circuit breaker %v went from %v to %v", name, from.String(), to.String())
		},
	})

	return b
}

func (b *rateBreaker) readyToTrip() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.sampler == nil {
		return false
	}

	return b.sampler.count >= b.settings.Failures
}

// count the failures in closed and half-open state
func (b *rateBreaker) countRate(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.sampler == nil {
		b.sampler = newBinarySampler(b.settings.Window)
	}

	b.sampler.tick(!success)
}

func (b *rateBreaker) Allow() (func(bool), bool) {
	done, err := b.gb.Allow()

	// this error can only indicate that the breaker is not closed
	closed := err == nil

	if !closed {
		return nil, false
	}

	return func(success bool) {
		b.countRate(success)
		done(success)
	}, true
}
