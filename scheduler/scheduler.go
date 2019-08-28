package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/routing"
)

// note: Config must stay comparable because it is used to detect changes in route specific LIFO config

const (
	LIFOKey = "lifo"
)

type Config struct {
	Name           string
	MaxConcurrency int
	MaxStackSize   int
	Timeout        time.Duration
}

type Stack struct {
	stack  *jobqueue.Stack
	config Config
}

type Registry struct {
	mu     sync.Mutex
	stacks map[string]*Stack
}

type LIFOFilter interface {
	SetStack(*Stack)
	GetStack() *Stack
	Config() Config
}

type GroupedLIFOFilter interface {
	LIFOFilter
	Group() string
	HasConfig() bool
}

func newStack(c Config) *Stack {
	return &Stack{
		config: c,
		stack: jobqueue.With(jobqueue.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxStackSize,
			Timeout:        c.Timeout,
		}),
	}
}

func (s *Stack) Wait() (done func(), err error) {
	return s.stack.Wait()
}

func (s *Stack) close() {
	s.stack.Close()
}

func (s *Stack) Config() Config {
	return s.config
}

func NewRegistry() *Registry {
	return &Registry{
		stacks: make(map[string]*Stack),
	}
}

func (r *Registry) initLIFOFilters(routes []*routing.Route) []*routing.Route {
	rr := make([]*routing.Route, len(routes))
	groups := make(map[string][]GroupedLIFOFilter)

	for i, ri := range routes {
		rr[i] = ri
		for _, fi := range ri.Filters {
			// TODO: warn on multiple lifos in the same route
			if glf, ok := fi.Filter.(GroupedLIFOFilter); ok {
				groupName := glf.Group()
				groups[groupName] = append(groups[groupName], glf)
				continue
			}

			lf, ok := fi.Filter.(LIFOFilter)
			if !ok {
				continue
			}

			key := fmt.Sprintf("lifo::%s", ri.Id)
			c := lf.Config()
			s, ok := r.stacks[key]
			if !ok {
				s = newStack(c)
				r.stacks[key] = s
			} else if s.config != c {
				s.config = c // TODO: the config is actually not updated this way
				r.stacks[key] = s
				// TODO: tear down here
			}

			lf.SetStack(s)
		}
	}

	for name, group := range groups {
		var (
			c           Config
			foundConfig bool
		)

		for _, glf := range group {
			if !glf.HasConfig() {
				continue
			}

			if foundConfig && glf.Config() != c {
				log.Warnf("Found mismatching configuration for the LIFO group: %s", name)
				continue
			}

			c = glf.Config()
			foundConfig = true
		}

		key := fmt.Sprintf("group-lifo::%s", name)
		s, ok := r.stacks[key]
		if !ok {
			s = newStack(c)
			r.stacks[key] = s
		} else if s.config != c {
			s.config = c
			r.stacks[key] = s
			// TODO: tear down here
		}

		for _, glf := range group {
			glf.SetStack(s)
		}
	}

	return rr
}

// Do implements routing.PostProcessor and sets the stack for the scheduler filters.
//
// It preserves the existing stack when available.
func (r *Registry) Do(routes []*routing.Route) []*routing.Route {
	return r.initLIFOFilters(routes)
}

func (r *Registry) Close() {
	for _, s := range r.stacks {
		s.close()
	}
}
