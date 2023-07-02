package loadbalancer

import (
	"math/rand"
	"sync"
)

type lockedSource struct {
	mu sync.Mutex
	r  rand.Source
}

func newLockedSource() *lockedSource {
	return &lockedSource{r: rand.NewSource(rand.Int63())} // #nosec
}

func (s *lockedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Int63()
}

func (s *lockedSource) Seed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r.Seed(seed)
}
