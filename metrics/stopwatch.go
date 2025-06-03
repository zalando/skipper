package metrics

import "time"

type StopWatch struct {
	started time.Time
	elapsed time.Duration
}

func NewStopWatch() *StopWatch {
	return &StopWatch{}
}

func (s *StopWatch) Start() {
	if s.started.IsZero() {
		s.started = time.Now()
	}
}

func (s *StopWatch) Stop() {
	if !s.started.IsZero() {
		s.elapsed += time.Since(s.started)
		s.started = time.Time{}
	}
}

func (s *StopWatch) Reset() {
	s.started = time.Time{}
	s.elapsed = 0
}

func (s *StopWatch) Elapsed() time.Duration {
	return s.elapsed
}
