package scheduler

import (
	"net/http"
	"time"

	"github.com/aryszka/jobqueue"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
)

type (
	lifoSpec      struct{}
	lifoGroupSpec struct{}

	lifoFilter struct {
		config scheduler.Config
		queue  *scheduler.Queue
	}

	lifoGroupFilter struct {
		name      string
		hasConfig bool
		config    scheduler.Config
		queue     *scheduler.Queue
	}
)

const (
	// Deprecated, use filters.LifoName instead
	LIFOName = filters.LifoName
	// Deprecated, use filters.LifoGroupName instead
	LIFOGroupName = filters.LifoGroupName

	defaultMaxConcurrency = 100
	defaultMaxQueueSize   = 100
	defaultTimeout        = 10 * time.Second
)

func NewLIFO() filters.Spec {
	return &lifoSpec{}
}

func NewLIFOGroup() filters.Spec {
	return &lifoGroupSpec{}
}

func intArg(a interface{}) (int, error) {
	switch v := a.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	default:
		return 0, filters.ErrInvalidFilterParameters
	}
}

func durationArg(a interface{}) (time.Duration, error) {
	switch v := a.(type) {
	case string:
		return time.ParseDuration(v)
	default:
		return 0, filters.ErrInvalidFilterParameters
	}
}

func (s *lifoSpec) Name() string { return filters.LifoName }

// CreateFilter creates a lifoFilter, that will use a queue based
// queue for handling requests instead of the fifo queue. The first
// parameter is MaxConcurrency the second MaxQueueSize and the third
// Timeout.
//
// The implementation is based on
// https://pkg.go.dev/github.com/aryszka/jobqueue, which provides more
// detailed documentation.
//
// All parameters are optional and defaults to
// MaxConcurrency 100, MaxQueueSize 100, Timeout 10s.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxQueueSize: total max = MaxConcurrency + MaxQueueSize
//
// Min values are 1 for MaxConcurrency and MaxQueueSize, and 1ms for
// Timeout. All configuration that is below will be set to these min
// values.
func (s *lifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var l lifoFilter

	// set defaults
	l.config.MaxConcurrency = defaultMaxConcurrency
	l.config.MaxQueueSize = defaultMaxQueueSize
	l.config.Timeout = defaultTimeout

	if len(args) > 0 {
		c, err := intArg(args[0])
		if err != nil {
			return nil, err
		}
		if c >= 1 {
			l.config.MaxConcurrency = c
		}
	}

	if len(args) > 1 {
		c, err := intArg(args[1])
		if err != nil {
			return nil, err
		}
		if c >= 0 {
			l.config.MaxQueueSize = c
		}
	}

	if len(args) > 2 {
		d, err := durationArg(args[2])
		if err != nil {
			return nil, err
		}
		if d >= 1*time.Millisecond {
			l.config.Timeout = d
		}
	}

	if len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &l, nil
}

func (*lifoGroupSpec) Name() string { return filters.LifoGroupName }

// CreateFilter creates a lifoGroupFilter, that will use a queue based
// queue for handling requests instead of the fifo queue. The first
// parameter is the Name, the second MaxConcurrency, the third
// MaxQueueSize and the fourth Timeout.
//
// The Name parameter is used to group the queue by one or
// multiple routes. All other parameters are optional and defaults to
// MaxConcurrency 100, MaxQueueSize 100, Timeout 10s.  If the
// configuration for the same Name is different the behavior is
// undefined.
//
// The implementation is based on
// https://pkg.go.dev/github.com/aryszka/jobqueue, which provides more
// detailed documentation.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxQueueSize: total max = MaxConcurrency + MaxQueueSize
//
// Min values are 1 for MaxConcurrency and MaxQueueSize, and 1ms for
// Timeout. All configuration that is below will be set to these min
// values.
//
// It is enough to set the concurrency, queue size and timeout parameters for
// one instance of the filter in the group, and only the group name for the
// rest. Setting these values for multiple instances is fine, too. While only
// one of them will be used as the source for the applied settings, if there
// is accidentally a difference between the settings in the same group, a
// warning will be logged.
func (*lifoGroupSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	l := &lifoGroupFilter{}

	switch v := args[0].(type) {
	case string:
		l.name = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	// set defaults
	cfg := scheduler.Config{
		MaxConcurrency: defaultMaxConcurrency,
		MaxQueueSize:   defaultMaxQueueSize,
		Timeout:        defaultTimeout,
	}
	l.config = cfg

	if len(args) > 1 {
		l.hasConfig = true
		c, err := intArg(args[1])
		if err != nil {
			return nil, err
		}
		if c >= 1 {
			l.config.MaxConcurrency = c
		}
	}

	if len(args) > 2 {
		c, err := intArg(args[2])
		if err != nil {
			return nil, err
		}
		if c >= 1 {
			l.config.MaxQueueSize = c
		}
	}

	if len(args) > 3 {
		d, err := durationArg(args[3])
		if err != nil {
			return nil, err
		}
		if d >= 1*time.Millisecond {
			l.config.Timeout = d
		}
	}

	return l, nil
}

// Config returns the scheduler configuration for the given filter
func (l *lifoFilter) Config() scheduler.Config {
	return l.config
}

// SetQueue binds the queue to the current filter context
func (l *lifoFilter) SetQueue(q *scheduler.Queue) {
	l.queue = q
}

// GetQueue is only used in tests.
func (l *lifoFilter) GetQueue() *scheduler.Queue {
	return l.queue
}

// Close will cleanup underlying queues
func (l *lifoFilter) Close() error {
	if l.queue != nil {
		l.queue.Close()
	}
	return nil
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of inflight requests and respond to the caller,
// if the bounded queue returns an error. Status code by Error:
//
// - 503 if jobqueue.ErrStackFull
// - 502 if jobqueue.ErrTimeout
func (l *lifoFilter) Request(ctx filters.FilterContext) {
	request(l.GetQueue(), scheduler.LIFOKey, ctx)
}

// Response is the filter.Filter interface implementation. Response
// will decrease the number of inflight requests.
func (l *lifoFilter) Response(ctx filters.FilterContext) {
	response(scheduler.LIFOKey, ctx)
}

// HandleErrorResponse is to opt-in for filters to get called
// Response(ctx) in case of errors via proxy. It has to return true to opt-in.
func (l *lifoFilter) HandleErrorResponse() bool {
	return true
}

func (l *lifoGroupFilter) Group() string {
	return l.name
}

func (l *lifoGroupFilter) HasConfig() bool {
	return l.hasConfig
}

// Config returns the scheduler configuration for the given filter
func (l *lifoGroupFilter) Config() scheduler.Config {
	return l.config
}

// SetQueue binds the queue to the current filter context
func (l *lifoGroupFilter) SetQueue(q *scheduler.Queue) {
	l.queue = q
}

// GetQueue is only used in tests
func (l *lifoGroupFilter) GetQueue() *scheduler.Queue {
	return l.queue
}

// Close will cleanup underlying queues
func (l *lifoGroupFilter) Close() error {
	if l.queue != nil {
		l.queue.Close()
	}
	return nil
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of inflight requests and respond to the caller,
// if the bounded queue returns an error. Status code by Error:
//
// - 503 if jobqueue.ErrStackFull
// - 502 if jobqueue.ErrTimeout
func (l *lifoGroupFilter) Request(ctx filters.FilterContext) {
	request(l.GetQueue(), scheduler.LIFOKey, ctx)
}

// Response is the filter.Filter interface implementation. Response
// will decrease the number of inflight requests.
func (l *lifoGroupFilter) Response(ctx filters.FilterContext) {
	response(scheduler.LIFOKey, ctx)
}

// HandleErrorResponse is to opt-in for filters to get called
// Response(ctx) in case of errors via proxy. It has to return true to opt-in.
func (l *lifoGroupFilter) HandleErrorResponse() bool {
	return true
}

func request(q *scheduler.Queue, key string, ctx filters.FilterContext) {
	if q == nil {
		ctx.Logger().Warnf("Unexpected scheduler.Queue is nil for key %s", key)
		return
	}

	done, err := q.Wait()
	if err != nil {
		if span := opentracing.SpanFromContext(ctx.Request().Context()); span != nil {
			ext.Error.Set(span, true)
		}
		switch err {
		case jobqueue.ErrStackFull:
			ctx.Logger().Debugf("Failed to get an entry on to the queue to process QueueFull: %v for host %s", err, ctx.Request().Host)
			ctx.Serve(&http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "Queue Full - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
		case jobqueue.ErrTimeout:
			ctx.Logger().Debugf("Failed to get an entry on to the queue to process Timeout: %v for host %s", err, ctx.Request().Host)
			ctx.Serve(&http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "Queue timeout - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
		default:
			ctx.Logger().Errorf("Unknown error for route based LIFO: %v for host %s", err, ctx.Request().Host)
			ctx.Serve(&http.Response{StatusCode: http.StatusInternalServerError})
		}
		return
	}

	pending, _ := ctx.StateBag()[key].([]func())
	ctx.StateBag()[key] = append(pending, done)

}

func response(key string, ctx filters.FilterContext) {
	pending, _ := ctx.StateBag()[key].([]func())
	last := len(pending) - 1
	if last < 0 {
		return
	}

	pending[last]()
	ctx.StateBag()[key] = pending[:last]
}
