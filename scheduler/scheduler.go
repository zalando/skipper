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
	LIFOKey = "lifo"
)

type Config struct {
	MaxConcurrency int
	MaxQueueSize   int
	Timeout        time.Duration
	CloseTimeout   time.Duration
}

type QueueStatus struct {
	ActiveRequests int
	QueuedRequests int
}

type Queue struct {
	queue                    *jobqueue.Stack
	config                   Config
	activeRequestsMetricsKey string
	queuedRequestsMetricsKey string
}

type Options struct {
	MetricsUpdateTimeout   time.Duration
	EnableRouteLIFOMetrics bool
	Metrics                metrics.Metrics
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

func (r *Registry) newQueue(name string, c Config) *Queue {
	measure := name == "::global" || r.options.EnableRouteLIFOMetrics
	q := &Queue{
		config: c,
		// renaming Stack -> Queue in the jobqueue project will follow
		queue: jobqueue.With(jobqueue.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxQueueSize,
			Timeout:        c.Timeout,
		}),
	}

	if measure {
		if name == "" {
			name = "unknown"
		}

		if name == "::global" {
			name = "global"
		}

		q.activeRequestsMetricsKey = fmt.Sprintf("lifo.%s.active", name)
		q.queuedRequestsMetricsKey = fmt.Sprintf("lifo.%s.queued", name)
		r.measure()
	}

	return q
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
				q = r.newQueue(ri.Id, c)
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
			q = r.newQueue(name, c)
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
	if r.options.Metrics == nil || r.measuring {
		return
	}

	r.measuring = true
	go func() {
		for {
			r.queues.Range(func(_, value interface{}) bool {
				q := value.(*Queue)
				s := q.Status()
				r.options.Metrics.UpdateGauge(q.activeRequestsMetricsKey, float64(s.ActiveRequests))
				r.options.Metrics.UpdateGauge(q.queuedRequestsMetricsKey, float64(s.QueuedRequests))
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
	var global *Queue
	globali, ok := r.queues.Load("global")
	if !ok {
		// ::global avoids conflict with a route ID
		global = r.newQueue("::global", c)
		r.queues.Store("global", global)
		return global
	}

	global = globali.(*Queue)
	global.config = c
	global.reconfigure()
	return global
}

func (r *Registry) Close() {
	r.queues.Range(func(_, value interface{}) bool {
		value.(*Queue).close()
		return true
	})

	close(r.quit)
}
