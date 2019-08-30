package scheduler

import (
	"net/http"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
)

type (
	lifoSpec      struct{}
	lifoGroupSpec struct{}

	lifoFilter struct {
		key    string
		config scheduler.Config
		stack  *scheduler.Stack
	}

	lifoGroupFilter struct {
		name      string
		hasConfig bool
		config    scheduler.Config
		stack     *scheduler.Stack
	}
)

const (
	LIFOName      = "lifo"
	LIFOGroupName = "lifoGroup"

	defaultMaxConcurreny = 100
	defaultMaxStackSize  = 100
	defaultTimeout       = 10 * time.Second
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

func (s *lifoSpec) Name() string { return LIFOName }

// CreateFilter creates a lifoFilter, that will use a stack based
// queue for handling requests instead of the fifo queue. The first
// parameter is MaxConcurrency the second MaxStackSize and the third
// Timeout.
//
// The implementation is based on
// https://godoc.org/github.com/aryszka/jobqueue, which provides more
// detailed documentation.
//
// All parameters are optional and defaults to
// MaxConcurrency 100, MaxStackSize 100, Timeout 10s.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxStackSize: total max = MaxConcurrency + MaxStackSize
//
// Min values are 1 for MaxConcurrency and MaxStackSize, and 1ms for
// Timeout. All configration that is below will be set to these min
// values.
func (s *lifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var l lifoFilter

	// set defaults
	l.config.MaxConcurrency = defaultMaxConcurreny
	l.config.MaxStackSize = defaultMaxStackSize
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
			l.config.MaxStackSize = c
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

func (*lifoGroupSpec) Name() string { return LIFOGroupName }

// CreateFilter creates a lifoGroupFilter, that will use a stack based
// queue for handling requests instead of the fifo queue. The first
// parameter is the Name, the second MaxConcurrency, the third
// MaxStackSize and the fourth Timeout.
//
// The Name parameter is used to group the queue by one or
// multiple routes. All other parameters are optional and defaults to
// MaxConcurrency 100, MaxStackSize 100, Timeout 10s.  If the
// configuration for the same Name is different the behavior is
// undefined.
//
// The implementation is based on
// https://godoc.org/github.com/aryszka/jobqueue, which provides more
// detailed documentation.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxStackSize: total max = MaxConcurrency + MaxStackSize
//
// Min values are 1 for MaxConcurrency and MaxStackSize, and 1ms for
// Timeout. All configration that is below will be set to these min
// values.
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
		MaxConcurrency: defaultMaxConcurreny,
		MaxStackSize:   defaultMaxStackSize,
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
			l.config.MaxStackSize = c
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

// SetStack binds the stack to the current filter context
func (l *lifoFilter) SetStack(s *scheduler.Stack) {
	l.stack = s
}

// GetStack is only used in tests.
func (l *lifoFilter) GetStack() *scheduler.Stack {
	return l.stack
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of inflight requests and respond to the caller,
// if the bounded stack returns an error. Status code by Error:
//
// - 503 if jobqueue.ErrStackFull
// - 502 if jobqueue.ErrTimeout
func (l *lifoFilter) Request(ctx filters.FilterContext) {
	request(l.GetStack(), scheduler.LIFOKey, ctx)
}

// Response is the filter.Filter interface implementation. Response
// will decrease the number of inflight requests.
func (l *lifoFilter) Response(ctx filters.FilterContext) {
	response(scheduler.LIFOKey, ctx)
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

// SetStack binds the stack to the current filter context
func (l *lifoGroupFilter) SetStack(s *scheduler.Stack) {
	l.stack = s
}

// GetStack is only used in tests
func (l *lifoGroupFilter) GetStack() *scheduler.Stack {
	return l.stack
}

// Request is the filter.Filter interface implementation. Request will
// increase the number of inflight requests and respond to the caller,
// if the bounded stack returns an error. Status code by Error:
//
// - 503 if jobqueue.ErrStackFull
// - 502 if jobqueue.ErrTimeout
func (l *lifoGroupFilter) Request(ctx filters.FilterContext) {
	request(l.GetStack(), scheduler.LIFOKey, ctx)
}

// Response is the filter.Filter interface implementation. Response
// will decrease the number of inflight requests.
func (l *lifoGroupFilter) Response(ctx filters.FilterContext) {
	response(scheduler.LIFOKey, ctx)
}

func request(s *scheduler.Stack, key string, ctx filters.FilterContext) {
	if s == nil {
		log.Warningf("Unexpected scheduler.Stack is nil for key %s", key)
		return
	}

	done, err := s.Wait()
	if err != nil {
		// TODO: replace the log with metrics
		switch err {
		case jobqueue.ErrStackFull:
			log.Errorf("Failed to get an entry on to the stack to process StackFull: %v for host %s", err, ctx.Request().Host)
			ctx.Serve(&http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "Stack Full - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
		case jobqueue.ErrTimeout:
			log.Errorf("Failed to get an entry on to the stack to process Timeout: %v for host %s", err, ctx.Request().Host)
			ctx.Serve(&http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "Stack timeout - https://opensource.zalando.com/skipper/operation/operation/#scheduler",
			})
		default:
			log.Errorf("Unknown error for route based LIFO: %v for host %s", err, ctx.Request().Host)
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
