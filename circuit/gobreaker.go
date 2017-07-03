package circuit

import "github.com/sony/gobreaker"

// wrapper for changing interface:
type gobreakerWrap struct {
	gb *gobreaker.TwoStepCircuitBreaker
}

func newGobreaker(s BreakerSettings, readyToTrip func(int) bool) gobreakerWrap {
	return gobreakerWrap{gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:        s.Host,
		MaxRequests: uint32(s.HalfOpenRequests),
		Timeout:     s.Timeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return readyToTrip(int(c.ConsecutiveFailures))
		},
	})}
}

func (w gobreakerWrap) Allow() (func(bool), bool) {
	done, err := w.gb.Allow()

	// this error can only indicate that the breaker is not closed
	if err != nil {
		return nil, false
	}

	return done, true
}

func (w gobreakerWrap) Closed() bool {
	return w.gb.State() == gobreaker.StateClosed
}
