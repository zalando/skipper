package scheduler

import (
	"time"

	"github.com/aryszka/jobstack"
	"github.com/zalando/skipper/routing"
)

// note: Config must stay comparable because it is used to detect changes in route specific LIFO config

type Config struct {
	Name           string
	MaxConcurrency int
	MaxStackSize   int
	Timeout        time.Duration
}

type Stack struct {
	stack  *jobstack.Stack
	config Config
}

type Registry struct {
	groupConfig map[string]Config
	stacks      map[string]*Stack
}

type LIFOFilter interface {
	SetStack(*Stack)
	GetStack() *Stack
	Config() Config
	Key() string
}

func newStack(c Config) *Stack {
	return &Stack{
		config: c,
		stack: jobstack.With(jobstack.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxStackSize,
			Timeout:        c.Timeout,
		}),
	}
}

func (s *Stack) Ready() (done func(), err error) {
	return s.stack.Ready()
}

func (s *Stack) close() {
	s.stack.Close()
}

func (s *Stack) Config() Config {
	return s.config
}

func NewRegistry() *Registry {
	return &Registry{
		groupConfig: make(map[string]Config),
		stacks:      make(map[string]*Stack),
	}
}

func (r *Registry) getStack(name string) (s *Stack, ok bool) {
	s, ok = r.stacks[name]
	return
}

func (r *Registry) setStack(name string, s *Stack) {
	r.stacks[name] = s
}

// Do implements routing.PostProcessor and sets the stack for the scheduler filters.
//
// It preserves the existing stack when available.
func (r *Registry) Do(routes []*routing.Route) []*routing.Route {
	rr := make([]*routing.Route, len(routes))
	for i, ri := range routes {
		rr[i] = ri
		for _, fi := range ri.Filters {
			cf, ok := fi.Filter.(LIFOFilter)
			if ok {
				c := cf.Config()
				key := cf.Key()
				s, ok := r.getStack(key)
				if !ok {
					s = newStack(c)
					r.setStack(key, s)
				} else if c != s.config { // UpdateDoc
					s.close()
					s = newStack(c)
					r.setStack(key, s)
				}

				cf.SetStack(s)
			}
		}
	}
	return rr
}

func (r *Registry) Close() {
	for _, s := range r.stacks {
		s.close()
	}
}
