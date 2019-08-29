package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

// note: Config must stay comparable because it is used to detect changes in route specific LIFO config

const (
	LIFOKey   = "lifo"
	globalKey = "::global" // (cannot come from an eskip route id)
)

type Config struct {
	Name                     string
	MaxConcurrency           int
	MaxQueueSize             int
	Timeout                  time.Duration
	CloseTimeout             time.Duration
	Metrics                  metrics.Metrics
	activeRequestsMetricsKey string
	queuedRequestsMetricsKey string
}

type QueueStatus struct {
	ActiveRequests int
	QueuedRequests int
}

type Queue struct {
	queue  *jobqueue.Stack
	config Config
}

type Options struct {
	MetricsUpdateTimeout time.Duration
}

type Registry struct {
	options   Options
	queues    *sync.Map
	measuring bool
	quit      chan struct{}
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
	if c.Metrics != nil {
		if c.Name == "" {
			c.Name = "unknown"
		}

		c.activeRequestsMetricsKey = fmt.Sprintf("lifo.%s.active", c.Name)
		c.queuedRequestsMetricsKey = fmt.Sprintf("lifo.%s.queued", c.Name)
	}

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

func (q *Queue) Close() {
	q.queue.Close()
}

func (q *Queue) Config() Config {
	return q.config
}

func RegistryWith(o Options) *Registry {
	if o.MetricsUpdateTimeout <= 0 {
		o.MetricsUpdateTimeout = time.Second
	}

	return &Registry{
		options: o,
		queues:  new(sync.Map),
		quit:    make(chan struct{}),
	}
}

func NewRegistry() *Registry {
	return RegistryWith(Options{})
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

			var q *Queue
			key := fmt.Sprintf("lifo::%s", ri.Id)
			c := lf.Config()
			qi, ok := r.queues.Load(key)
			if ok {
				q = qi.(*Queue)
			}

			if !ok {
				q = newQueue(c)
				r.queues.Store(key, q)
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

		var q *Queue
		key := fmt.Sprintf("group-lifo::%s", name)
		qi, ok := r.queues.Load(key)
		if ok {
			q = qi.(*Queue)
		}

		if !ok {
			q = newQueue(c)
			r.queues.Store(key, q)
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

func (r *Registry) measure() {
	if r.measuring {
		return
	}

	r.measuring = true
	go func() {
		for {
			r.queues.Range(func(_, value interface{}) bool {
				q := value.(*Queue)
				s := q.Status()
				q.config.Metrics.UpdateGauge(q.config.activeRequestsMetricsKey, float64(s.ActiveRequests))
				q.config.Metrics.UpdateGauge(q.config.queuedRequestsMetricsKey, float64(s.QueuedRequests))
				return true
			})

			select {
			case <-time.After(r.options.MetricsUpdateTimeout):
			case <-r.quit:
				return
			}
		}
	}()
}

func (r *Registry) Global(c Config) *Queue {
	c.Name = "global"
	if c.Metrics != nil {
		r.measure()
	}

	var global *Queue
	globali, ok := r.queues.Load(globalKey)
	if !ok {
		global = newQueue(c)
		r.queues.Store(globalKey, global)
		return global
	}

	global = globali.(*Queue)
	global.config = c
	global.reconfigure()
	return global
}

func (r *Registry) Close() {
	r.queues.Range(func(_, value interface{}) bool {
		value.(*Queue).Close()
		return true
	})

	close(r.quit)
}
