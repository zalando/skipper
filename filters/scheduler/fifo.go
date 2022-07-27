package scheduler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
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
	var f fifoFilter

	// set defaults
	f.config.MaxConcurrency = defaultMaxConcurreny
	f.config.MaxQueueSize = defaultMaxQueueSize
	f.config.Timeout = defaultTimeout

	if len(args) > 0 {
		c, err := intArg(args[0])
		if err != nil {
			return nil, err
		}
		if c >= 1 {
			f.config.MaxConcurrency = c
		}
	}

	if len(args) > 1 {
		c, err := intArg(args[1])
		if err != nil {
			return nil, err
		}
		if c >= 0 {
			f.config.MaxQueueSize = c
		}
	}

	if len(args) > 2 {
		d, err := durationArg(args[2])
		if err != nil {
			return nil, err
		}
		if d >= 1*time.Millisecond {
			f.config.Timeout = d
		}
	}

	if len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &f, nil
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
	c := ctx.Request().Context()
	done, err := f.queue.Wait(c)
	if err != nil {
		done()
		if span := opentracing.SpanFromContext(c); span != nil {
			ext.Error.Set(span, true)
			span.LogKV("fifo error", fmt.Sprintf("Failed to wait for fifo queue: %v", err))
		}
		log.Debugf("Failed to wait for fifo queue: %v", err)

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
			log.Errorf("Unknown error in fifo() please create an issue https://github.com/zalando/skipper/issues/new/choose: %v", err)
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
	last := len(pending) - 1
	if last < 0 {
		return
	}
	pending[last]()
	ctx.StateBag()[fifoKey] = pending[:last]

	if ok {
		f.queue.Release()
	}
}

const (
	fifoKey string = "fifo"
)
