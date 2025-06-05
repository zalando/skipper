package proxy

import "time"

type stopWatch struct {
	started time.Time
	elapsed time.Duration
}

func NewStopWatch() *stopWatch {
	return &stopWatch{}
}

func (s *stopWatch) Start() {
	if s.started.IsZero() {
		s.started = time.Now()
	}
}

func (s *stopWatch) Stop() {
	if !s.started.IsZero() {
		s.elapsed += time.Since(s.started)
		s.started = time.Time{}
	}
}

func (s *stopWatch) Reset() {
	s.started = time.Time{}
	s.elapsed = 0
}

func (s *stopWatch) Elapsed() time.Duration {
	return s.elapsed
}
