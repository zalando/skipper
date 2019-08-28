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
	MaxQueueSize   int
	Timeout        time.Duration
}

type Queue struct {
	stack  *jobqueue.Stack
	config Config
}

type Registry struct {
	mu     sync.Mutex
	stacks map[string]*Queue
}

type LIFOFilter interface {
	SetQueue(*Queue)
	GetQueue() *Queue
	Config() Config
}

type GroupedLIFOFilter interface {
	LIFOFilter
	Group() string
	HasConfig() bool
}

func newQueue(c Config) *Queue {
	return &Queue{
		config: c,
		// renaming Stack -> Queue in the jobqueue project will follow
		stack: jobqueue.With(jobqueue.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxQueueSize,
			Timeout:        c.Timeout,
		}),
	}
}

func (s *Queue) Wait() (done func(), err error) {
	return s.stack.Wait()
}

func (s *Queue) reconfigure() {
	// renaming Stack -> Queue in the jobqueue project will follow
	s.stack.Reconfigure(jobqueue.Options{
		MaxConcurrency: s.config.MaxConcurrency,
		MaxStackSize:   s.config.MaxQueueSize,
		Timeout:        s.config.Timeout,
	})
}

func (s *Queue) close() {
	s.stack.Close()
}

func (s *Queue) Config() Config {
	return s.config
}

func NewRegistry() *Registry {
	return &Registry{
		stacks: make(map[string]*Queue),
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
				s = newQueue(c)
				r.stacks[key] = s
			} else if s.config != c {
				s.config = c
				s.reconfigure()
			}

			lf.SetQueue(s)
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
			s = newQueue(c)
			r.stacks[key] = s
		} else if s.config != c {
			s.config = c
			s.reconfigure()
		}

		for _, glf := range group {
			glf.SetQueue(s)
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
