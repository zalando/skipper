// Package gpt provides a wait() filter that waits for a specified duration before proxying the request to the backend.
//
// Example:
//
//	wait("1s") -> <backend-url>
//
// This filter takes a single parameter, which is the duration of the wait time in Go duration format.
// The filter does not modify the request or response.
package gpt

import (
	"time"

	"github.com/zalando/skipper/filters"
)

const (
	name          = "wait"
	waitTimeParam = "time"
)

type waitSpec struct{}

func NewWait() filters.Spec {
	return &waitSpec{}
}

func (s *waitSpec) Name() string {
	return name
}

func (s *waitSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	timeParam, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	duration, err := time.ParseDuration(timeParam)
	if err != nil {
		return nil, err
	}

	return &waitFilter{duration: duration}, nil
}

type waitFilter struct {
	duration time.Duration
}

func (f *waitFilter) Request(ctx filters.FilterContext) {
	// Wait for the specified duration
	time.Sleep(f.duration)
}

func (f *waitFilter) Response(filters.FilterContext) {}

func (f *waitFilter) Name() string {
	return name
}
