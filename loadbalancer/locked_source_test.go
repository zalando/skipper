package loadbalancer

import (
	"sync"
	"testing"
)

func loadTestLockedSource(s *lockedSource, n int) {
	for range n {
		s.Int63()
	}
}

func TestLockedSourceForConcurrentUse(t *testing.T) {
	s := NewLockedSource()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			loadTestLockedSource(s, 100000)
		})
	}
	wg.Wait()
}
