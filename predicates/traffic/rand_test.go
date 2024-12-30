package traffic_test

import (
	"math/rand/v2"
	"sync"
)

// newTestRandFloat64 returns a function that generates fixed sequence of random float64 values for testing.
func newTestRandFloat64() func() float64 {
	return rand.New(&lockedSource{s: rand.NewPCG(0x5EED_1, 0x5EED_2)}).Float64
}

type lockedSource struct {
	mu sync.Mutex
	s  rand.Source
}

func (s *lockedSource) Uint64() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.Uint64()
}
