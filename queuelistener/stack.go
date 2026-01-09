package queuelistener

import (
	"sync"
)

const stackSize int = 10000

type naiveStack[T any] struct {
	mu    sync.Mutex
	top   int
	items [stackSize]*T
}

func NewStack() *naiveStack[external] {
	return &naiveStack[external]{
		top: -1,
	}
}

func (s *naiveStack[T]) Push(data *T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == len(s.items)-1 {
		return
	}

	s.top++
	s.items[s.top] = data
}

func (s *naiveStack[T]) Pop() *T {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == -1 {
		return nil
	} else {
		defer func() { s.top-- }()
	}

	return s.items[s.top]
}
