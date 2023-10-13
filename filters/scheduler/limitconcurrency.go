package scheduler

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/zalando/skipper/filters"
)

type (
	limitConcurrencySpec   struct{}
	limitConcurrencyFilter struct {
		concurrentRequests *atomic.Int64
		maxConcurrency     int64
	}
)

func NewLimitConcurrency() filters.Spec {
	return &limitConcurrencySpec{}
}

func (*limitConcurrencySpec) Name() string {
	return filters.LimitConcurrencyName
}

// CreateFilter creates a limitConcurrencyFilter, that will use an
// atomic counter to limit concurrency of a route including response
// body streaming. The first parameter is maxConcurrency.
func (s *limitConcurrencySpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	cc, err := intArg(args[0])
	if err != nil {
		return nil, err
	}
	if cc < 1 {
		return nil, fmt.Errorf("maxconcurrency requires value >0, %w", filters.ErrInvalidFilterParameters)
	}

	return &limitConcurrencyFilter{
		concurrentRequests: &atomic.Int64{},
		maxConcurrency:     int64(cc),
	}, nil
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of concurrent requests and respond to the caller,
// if the bounded queue returns an error. Status code by Error:
//
// - 503 if full
func (f *limitConcurrencyFilter) Request(ctx filters.FilterContext) {
	if f.maxConcurrency > f.concurrentRequests.Load() {
		ctx.Serve(&http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "Max concurrency reached - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
		})
		return
	}
	f.concurrentRequests.Add(1)
}

// Response will decrease the number of inflight requests to release
// the concurrency reservation for the request.
func (f *limitConcurrencyFilter) Response(ctx filters.FilterContext) {
	ctx.StateBag()[filters.LimitConcurrency] = func() {
		f.concurrentRequests.Add(-1)
	}
}
