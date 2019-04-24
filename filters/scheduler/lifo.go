package scheduler

import (
	"net/http"
	"sync"
	"time"

	"github.com/aryszka/jobstack"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/scheduler"
)

// TODO: must be documented that it cannot be used together with the legacy shunting, meaning
// that it's incompatible with MarkServed().

type (
	lifoSpec struct{}

	lifoFilter struct {
		key    string
		config scheduler.Config
		stack  *scheduler.Stack
	}

	groupConfig struct {
		mu     sync.Mutex
		config map[string]scheduler.Config
	}
)

const (
	LIFOName = "lifo"

	lifoKey = "lifo-done"

	defaultMaxConcurreny = 100
	defaultMaxStackSize  = 100
	defaultTimeout       = 10 * time.Second
)

var configStore groupConfig

func NewLIFO() filters.Spec {
	return &lifoSpec{}
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
// parameter is the Name, the second MaxConcurrency, the third
// MaxStackSize and the fourth Timeout.
//
// The Name parameter is used to group the queue by one or
// multiple routes. All other parameters are optional and defaults to
// MaxConcurrency 100, MaxStackSize 100, Timeout 10s.  If the
// configuration for the same Name is different the behavior is
// undefined.

// The implementation is based on
// https://godoc.org/github.com/aryszka/jobstack, which provides more
// detailed documentation.
//
// The total maximum number of requests has to be computed by adding
// MaxConcurrency and MaxStackSize: total max = MaxConcurrency + MaxStackSize
//
// Min values are 1 for MaxConcurrency and MaxStackSize, and 1ms for
// Timeout. All configration that is below will be set to these min
// values.
func (s *lifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	l := &lifoFilter{}

	switch v := args[0].(type) {
	case string:
		l.key = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	// changes will only happen if we change the name of the group
	if config, ok := l.getConfig(); ok {
		l.config = config
		return l, nil
	}

	// set defaults
	l.config.MaxConcurrency = defaultMaxConcurreny
	l.config.MaxStackSize = defaultMaxStackSize
	l.config.Timeout = defaultTimeout

	if len(args) > 1 {
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

func (l *lifoFilter) Request(ctx filters.FilterContext) {
	request(l.stack, lifoKey, ctx)
}

func (l *lifoFilter) Response(ctx filters.FilterContext) {
	response(lifoKey, ctx)
}

func (l *lifoFilter) Config() scheduler.Config {
	cfg, _ := l.getConfig()
	return cfg
}

func (l *lifoFilter) getConfig() (scheduler.Config, bool) {
	configStore.mu.Lock()
	defer configStore.mu.Unlock()
	res, ok := configStore.config[l.key]
	return res, ok
}

func (l *lifoFilter) SetStack(s *scheduler.Stack) {
	l.stack = s
}

func (l *lifoFilter) GetStack() *scheduler.Stack {
	return l.stack
}

func (l *lifoFilter) Key() string {
	return l.key
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
