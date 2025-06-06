package proxy

import "time"

type stopWatch struct {
	now     func() time.Time
	started time.Time
	elapsed time.Duration
}

func newStopWatch() *stopWatch {
	return &stopWatch{
		now: time.Now,
	}
}

func (s *stopWatch) Start() {
	if s.started.IsZero() {
		s.started = s.now()
	}
}

func (s *stopWatch) Stop() {
	if !s.started.IsZero() {
		now := s.now()
		s.elapsed += now.Sub(s.started)
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
