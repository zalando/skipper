package lifo

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/lifo"
)

// TODO: must be documented that it cannot be used together with the legacy shunting, meaning
// that it's incompatible with MarkServed().

type (
	lifoSpec      struct{}
	lifoGroupSpec struct{}
)

type lifoFilter struct {
	config lifo.Config
	stack  *lifo.Stack
}

type lifoGroupFilter struct {
	name  string
	stack *lifo.Stack
}

const (
	lifoKey      = "lifo-done"
	lifoGroupKey = "lifo-group-done"
)

const (
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

func (s *lifoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		l   lifoFilter
		err error
	)

	if len(args) > 0 {
		if l.config.MaxConcurrency, err = intArg(args[0]); err != nil {
			return nil, err
		}
	}

	if len(args) > 1 {
		if l.config.MaxStackSize, err = intArg(args[1]); err != nil {
			return nil, err
		}
	}

	if len(args) > 2 {
		if l.config.Timeout, err = durationArg(args[2]); err != nil {
			return nil, err
		}
	}

	if len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &l, nil
}

func request(s *lifo.Stack, key string, ctx filters.FilterContext) {
	if s == nil {
		return
	}

	done, err := s.Ready()
	if err != nil {
		// TODO:
		// - replace the log with metrics
		// - allow custom status code
		// - provide more info in the header about the reason

		log.Debug("route based LIFO:", err)
		ctx.Serve(&http.Response{StatusCode: http.StatusServiceUnavailable})
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

func (l *lifoFilter) Config() lifo.Config {
	return l.config
}

func (l *lifoFilter) SetLIFO(s *lifo.Stack) {
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

func (g *lifoGroupFilter) SetLIFO(s *lifo.Stack) {
	g.stack = s
}
