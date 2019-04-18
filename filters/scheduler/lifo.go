package scheduler

import (
	"net/http"
	"time"

	"github.com/aryszka/jobstack"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
)

// TODO: must be documented that it cannot be used together with the legacy shunting, meaning
// that it's incompatible with MarkServed().

type (
	lifoSpec      struct{}
	lifoGroupSpec struct{}

	lifoFilter struct {
		config scheduler.Config
		stack  *scheduler.Stack
	}

	lifoGroupFilter struct {
		name  string
		stack *scheduler.Stack
	}
)

const (
	lifoKey      = "lifo-done"
	lifoGroupKey = "lifo-group-done"

	LIFOName      = "lifo"
	LIFOGroupName = "lifoGroup"
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
// All parameters are optional and defaults to
// https://godoc.org/github.com/aryszka/jobstack#Options, which
// defaults to MaxConcurrency 1, MaxStackSize infinite, Timeout
// infinite.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxStackSize: total max = MaxConcurrency + MaxStackSize
//
// Min values are 1 for MaxConcurrency and MaxStackSize, and 1ms for
// Timeout. All configration that is below will be set to these min
// values.
func (s *lifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		l   lifoFilter
		err error
	)

	if len(args) > 0 {
		if l.config.MaxConcurrency, err = intArg(args[0]); err != nil {
			return nil, err
		}
		if l.config.MaxConcurrency < 1 {
			l.config.MaxConcurrency = 1
		}
	}

	if len(args) > 1 {
		if l.config.MaxStackSize, err = intArg(args[1]); err != nil {
			return nil, err
		}
		if l.config.MaxStackSize < 1 {
			l.config.MaxStackSize = 1
		}
	}

	if len(args) > 2 {
		if l.config.Timeout, err = durationArg(args[2]); err != nil {
			return nil, err
		}
		if l.config.Timeout < 1*time.Millisecond {
			l.config.Timeout = 1 * time.Millisecond
		}
	}

	if len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &l, nil
}

func request(s *scheduler.Stack, key string, ctx filters.FilterContext) {
	if s == nil {
		log.Warningf("Unexpected scheduler.Stack is nil for key %s", key)
		return
	}

	done, err := s.Ready()
	if err != nil {
		// TODO:
		// - replace the log with metrics
		// - allow custom status code
		// - provide more info in the header about the reason

		switch err {
		case jobstack.ErrStackFull:
			log.Errorf("Failed to get an entry on to the stack to process: %v", err)
			ctx.Serve(&http.Response{StatusCode: http.StatusServiceUnavailable, Status: "Stack Full"})
		case jobstack.ErrTimeout:
			log.Errorf("Failed to get an entry on to the stack to process: %v", err)
			ctx.Serve(&http.Response{StatusCode: http.StatusBadGateway, Status: "Stack timeout"})
		default:
			log.Errorf("Unknown error for route based LIFO: %v", err)
			ctx.Serve(&http.Response{StatusCode: http.StatusServiceUnavailable})
		}
		return
	}

	ctx.StateBag()[key] = done
}

func response(key string, ctx filters.FilterContext) {
	done := ctx.StateBag()[key]
	if done == nil {
		return
	}

	if f, ok := done.(func()); ok {
		f()
	}
}

func (l *lifoFilter) Request(ctx filters.FilterContext) {
	request(l.stack, lifoKey, ctx)
}

func (l *lifoFilter) Response(ctx filters.FilterContext) {
	response(lifoKey, ctx)
}

func (l *lifoFilter) Config() scheduler.Config {
	return l.config
}

func (l *lifoFilter) SetStack(s *scheduler.Stack) {
	l.stack = s
}

func (s *lifoGroupSpec) Name() string { return LIFOGroupName }

func (s *lifoGroupSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch v := args[0].(type) {
	case string:
		return &lifoGroupFilter{name: v}, nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (g *lifoGroupFilter) Request(ctx filters.FilterContext) {
	request(g.stack, lifoGroupKey, ctx)
}

func (g *lifoGroupFilter) Response(ctx filters.FilterContext) {
	response(lifoGroupKey, ctx)
}

func (g *lifoGroupFilter) GroupName() string {
	return g.name
}

func (g *lifoGroupFilter) SetStack(s *scheduler.Stack) {
	g.stack = s
}
