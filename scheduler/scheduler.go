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

type QueueStatus struct {
	ActiveRequests int
	QueuedRequests int
}

type Queue struct {
	queue  *jobqueue.Stack
	config Config
}

type Registry struct {
	mu     sync.Mutex
	queues map[string]*Queue
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
		queue: jobqueue.With(jobqueue.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxQueueSize,
			Timeout:        c.Timeout,
		}),
	}
}

func (q *Queue) Wait() (done func(), err error) {
	return q.queue.Wait()
}

func (q *Queue) Status() QueueStatus {
	st := q.queue.Status()
	return QueueStatus{
		ActiveRequests: st.ActiveJobs,
		QueuedRequests: st.QueuedJobs,
	}
}

func (q *Queue) reconfigure() {
	// renaming Stack -> Queue in the jobqueue project will follow
	q.queue.Reconfigure(jobqueue.Options{
		MaxConcurrency: q.config.MaxConcurrency,
		MaxStackSize:   q.config.MaxQueueSize,
		Timeout:        q.config.Timeout,
	})
}

func (q *Queue) close() {
	q.queue.Close()
}

func (q *Queue) Config() Config {
	return q.config
}

func NewRegistry() *Registry {
	return &Registry{
		queues: make(map[string]*Queue),
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
			q, ok := r.queues[key]
			if !ok {
				q = newQueue(c)
				r.queues[key] = q
			} else if q.config != c {
				q.config = c
				q.reconfigure()
			}

			lf.SetQueue(q)
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
		q, ok := r.queues[key]
		if !ok {
			q = newQueue(c)
			r.queues[key] = q
		} else if q.config != c {
			q.config = c
			q.reconfigure()
		}

		for _, glf := range group {
			glf.SetQueue(q)
		}
	}

	return rr
}

// Do implements routing.PostProcessor and sets the queue for the scheduler filters.
//
// It preserves the existing queue when available.
func (r *Registry) Do(routes []*routing.Route) []*routing.Route {
	return r.initLIFOFilters(routes)
}

func (r *Registry) Close() {
	for _, q := range r.queues {
		q.close()
	}
}
