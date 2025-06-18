package queuelistener

import (
	"sync"
)

const stackSize int = 10000

type naiveStack[T any] struct {
	mu sync.Mutex
	//cond  *sync.Cond
	top   int
	items [stackSize]*T
}

func NewStack() *naiveStack[external] {
	ns := &naiveStack[external]{
		top: -1,
	}
	//ns.cond = sync.NewCond(&ns.mu)
	return ns
}

func (s *naiveStack[T]) Push(data *T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == len(s.items)-1 {
		return
	}

	s.top++
	s.items[s.top] = data
	//s.mu.Unlock()
	//s.cond.Signal()
}

func (s *naiveStack[T]) Pop() *T {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == -1 {
		return nil
		//s.cond.Wait()
	} else {
		defer func() { s.top-- }()
	}

	return s.items[s.top]
}
