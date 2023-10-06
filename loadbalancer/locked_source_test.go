package loadbalancer

import (
	"sync"
	"testing"
)

func loadTestLockedSource(s *lockedSource, n int) {
	for i := 0; i < n; i++ {
		s.Int63()
	}
}

func TestLockedSourceForConcurrentUse(t *testing.T) {
	s := NewLockedSource()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			loadTestLockedSource(s, 100000)
			wg.Done()
		}()
	}
	wg.Wait()
}
