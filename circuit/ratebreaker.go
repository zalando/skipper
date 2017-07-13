package circuit

import (
	log "github.com/sirupsen/logrus"
	"sync"

	"github.com/sony/gobreaker"
)

// TODO:
// in case of the rate breaker, there are unnecessary synchronization steps due to the 3rd party gobreaker. If
// the sliding window was part of the implementation of the individual breakers, this additional syncrhonization
// would not be required.

type rateBreaker struct {
	settings BreakerSettings
	open     bool
	mx       *sync.Mutex
	sampler  *binarySampler
	gb       *gobreaker.TwoStepCircuitBreaker
}

func newRate(s BreakerSettings) *rateBreaker {
	b := &rateBreaker{
		settings: s,
		mx:       &sync.Mutex{},
	}

	b.gb = gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:        s.Host,
		MaxRequests: uint32(s.HalfOpenRequests),
		Timeout:     s.Timeout,
		ReadyToTrip: func(gobreaker.Counts) bool { return b.readyToTrip() },
	})

	return b
}

func (b *rateBreaker) readyToTrip() bool {
	b.mx.Lock()
	defer b.mx.Unlock()

	if b.sampler == nil {
		return false
	}

	b.open = b.sampler.count >= b.settings.Failures
	if b.open {
		log.Infof("circuit breaker open: %v", b.settings)
		b.sampler = nil
	}

	return b.open
}

// count the failures in closed and half-open state
func (b *rateBreaker) countRate(success bool) {
	b.mx.Lock()
	defer b.mx.Unlock()

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

	if b.open {
		b.open = false
		log.Infof("circuit breaker closed: %v", b.settings)
	}

	return func(success bool) {
		b.countRate(success)
		done(success)
	}, true
}
