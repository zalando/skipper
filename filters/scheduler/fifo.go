package scheduler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
)

const (
	fifoKey string = "fifo"
)

type (
	fifoSpec   struct{}
	fifoFilter struct {
		config scheduler.Config
		queue  *scheduler.FifoQueue
	}
)

func NewFifo() filters.Spec {
	return &fifoSpec{}
}

func (*fifoSpec) Name() string {
	return filters.FifoName
}

// CreateFilter creates a fifoFilter, that will use a semaphore based
// queue for handling requests to limit concurrency of a route. The first
// parameter is maxConcurrency the second maxQueueSize and the third
// timeout.
func (s *fifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	cc, err := intArg(args[0])
	if err != nil {
		return nil, err
	}
	if cc < 1 {
		return nil, fmt.Errorf("maxconcurrency requires value >0, %w", filters.ErrInvalidFilterParameters)
	}

	qs, err := intArg(args[1])
	if err != nil {
		return nil, err
	}
	if qs < 0 {
		return nil, fmt.Errorf("maxqueuesize requires value >=0, %w", filters.ErrInvalidFilterParameters)
	}

	d, err := durationArg(args[2])
	if err != nil {
		return nil, err
	}
	if d < 1*time.Millisecond {
		return nil, fmt.Errorf("timeout requires value >=1ms, %w", filters.ErrInvalidFilterParameters)
	}

	return &fifoFilter{
		config: scheduler.Config{
			MaxConcurrency: cc,
			MaxQueueSize:   qs,
			Timeout:        d,
		},
	}, nil
}

func (f *fifoFilter) Config() scheduler.Config {
	return f.config
}

func (f *fifoFilter) GetQueue() *scheduler.FifoQueue {
	return f.queue
}

func (f *fifoFilter) SetQueue(fq *scheduler.FifoQueue) {
	f.queue = fq
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of inflight requests and respond to the caller,
// if the bounded queue returns an error. Status code by Error:
//
// - 503 if queue full
// - 502 if queue timeout
// - 500 if error unknown
func (f *fifoFilter) Request(ctx filters.FilterContext) {
	q := f.GetQueue()
	c := ctx.Request().Context()
	done, err := q.Wait(c)
	if err != nil {
		if span := opentracing.SpanFromContext(c); span != nil {
			ext.Error.Set(span, true)
			span.LogKV("fifo error", fmt.Sprintf("Failed to wait for fifo queue: %v", err))
		}
		ctx.Logger().Debugf("Failed to wait for fifo queue: %v", err)

		switch err {
		case scheduler.ErrQueueFull:
			ctx.Serve(&http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "Queue Full - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
			return
		case scheduler.ErrQueueTimeout:
			ctx.Serve(&http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "Queue Timeout - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
			return
		case scheduler.ErrClientCanceled:
			// This case is handled in the proxy with status code 499
			return

		default:
			ctx.Logger().Errorf("Unknown error in fifo() please create an issue https://github.com/zalando/skipper/issues/new/choose: %v", err)
			ctx.Serve(&http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "Unknown error in fifo https://opensource.zalando.com/skipper/operation/operation/#scheduler, please create an issue https://github.com/zalando/skipper/issues/new/choose",
			})
			return

		}
	}

	// ok
	pending, _ := ctx.StateBag()[fifoKey].([]func())
	ctx.StateBag()[fifoKey] = append(pending, done)
}

// Response will decrease the number of inflight requests to release
// the concurrency reservation for the request.
func (f *fifoFilter) Response(ctx filters.FilterContext) {
	pending, ok := ctx.StateBag()[fifoKey].([]func())
	if !ok {
		return
	}
	last := len(pending) - 1
	if last < 0 {
		return
	}
	pending[last]()
	ctx.StateBag()[fifoKey] = pending[:last]
}
