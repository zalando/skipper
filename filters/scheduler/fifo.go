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

type (
	fifoSpec struct {
		typ string
	}
	fifoFilter struct {
		config scheduler.Config
		queue  *scheduler.FifoQueue
		typ    string
	}
)

func NewFifo() filters.Spec {
	return &fifoSpec{
		typ: filters.FifoName,
	}
}

func NewFifoWithBody() filters.Spec {
	return &fifoSpec{
		typ: filters.FifoWithBodyName,
	}
}

func (s *fifoSpec) Name() string {
	return s.typ
}

// CreateFilter creates a fifoFilter, that will use a semaphore based
// queue for handling requests to limit concurrency of a route. The first
// parameter is maxConcurrency the second maxQueueSize and the third
// timeout.
func (s *fifoSpec) CreateFilter(args []any) (filters.Filter, error) {
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
		typ: s.typ,
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
	pending, _ := ctx.StateBag()[f.typ].([]func())
	ctx.StateBag()[f.typ] = append(pending, done)
}

// Response will decrease the number of inflight requests to release
// the concurrency reservation for the request.
func (f *fifoFilter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case filters.FifoName:
		pending, ok := ctx.StateBag()[f.typ].([]func())
		if !ok {
			return
		}
		last := len(pending) - 1
		if last < 0 {
			return
		}
		pending[last]()
		ctx.StateBag()[f.typ] = pending[:last]

	case filters.FifoWithBodyName:
		// nothing to do here, handled in the proxy after copyStream()
	}
}

// HandleErrorResponse is to opt-in for filters to get called
// Response(ctx) in case of errors via proxy. It has to return true to opt-in.
func (f *fifoFilter) HandleErrorResponse() bool {
	return true
}
