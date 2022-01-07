// Package scheduler provides a registry to be used as a postprocessor for the routes
// that use a LIFO filter.
package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

// note: Config must stay comparable because it is used to detect changes in route specific LIFO config

const (
	// Key used during routing to pass lifo values from the filters to the proxy.
	LIFOKey = "lifo"
)

// Config can be used to provide configuration of the registry.
type Config struct {

	// MaxConcurrency defines how many jobs are allowed to run concurrently.
	// Defaults to 1.
	MaxConcurrency int

	// MaxStackSize defines how many jobs may be waiting in the stack.
	// Defaults to infinite.
	MaxQueueSize int

	// Timeout defines how long a job can be waiting in the stack.
	// Defaults to infinite.
	Timeout time.Duration

	// CloseTimeout sets a maximum duration for how long the queue can wait
	// for the active and queued jobs to finish. Defaults to infinite.
	CloseTimeout time.Duration
}

// QueueStatus reports the current status of a queue. It can be used for metrics.
type QueueStatus struct {

	// ActiveRequests represents the number of the requests currently being handled.
	ActiveRequests int

	// QueuedRequests represents the number of requests waiting to be handled.
	QueuedRequests int

	// Closed indicates that the queue was closed.
	Closed bool
}

// Queue objects implement a LIFO queue for handling requests, with a maximum allowed
// concurrency and queue size. Currently, they can be used from the lifo and lifoGroup
// filters in the filters/scheduler package only.
type Queue struct {
	queue                    *jobqueue.Stack
	config                   Config
	metrics                  metrics.Metrics
	activeRequestsMetricsKey string
	errorFullMetricsKey      string
	errorOtherMetricsKey     string
	errorTimeoutMetricsKey   string
	queuedRequestsMetricsKey string
}

// Options provides options for the registry.
type Options struct {

	// MetricsUpdateTimeout defines the frequence of how often the LIFO metrics
	// are updated when they are enabled. Defaults to 1s.
	MetricsUpdateTimeout time.Duration

	// EnableRouteLIFOMetrics enables collecting metrics about the LIFO queues.
	EnableRouteLIFOMetrics bool

	// Metrics must be provided to the registry in order to collect the LIFO metrics.
	Metrics metrics.Metrics
}

// Registry maintains a set of LIFO queues. It is used to preserve LIFO queue instances
// across multiple generations of the routing. It implements the routing.PostProcessor
// interface, it is enough to just pass in to routing.Routing when initializing it.
//
// When the EnableRouteLIFOMetrics is set, then the registry starts a background goroutine
// for regularly take snapshots of the active lifo queues and update the corresponding
// metrics. This goroutine is started when the first lifo filter is detected and returns
// when the registry is closed. Individual metrics objects (keys) are used for each
// lifo filter, and one for each lifo group defined by the lifoGroup filter.
//
type Registry struct {
	options   Options
	queues    *sync.Map
	measuring bool
	quit      chan struct{}
}

// LIFOFilter is the interface that needs to be implemented by the filters that
// use a LIFO queue maintained by the registry.
type LIFOFilter interface {

	// SetQueue will be used by the registry to pass in the right queue to
	// the filter.
	SetQueue(*Queue)

	// GetQueue is currently used only by tests.
	GetQueue() *Queue

	// Config will be called by the registry once during processing the
	// routing to get the right queue settings from the filter.
	Config() Config
}

// GroupedLIFOFilter is an extension of the LIFOFilter interface for filters
// that use a shared queue.
type GroupedLIFOFilter interface {
	LIFOFilter

	// Group returns the name of the group.
	Group() string

	// HasConfig indicates that the current filter provides the queue
	// queue settings for the group.
	HasConfig() bool
}

// Wait blocks until a request can be processed or needs to be rejected.
// When it can be processed, calling done indicates that it has finished.
// It is mandatory to call done() the request was processed. When the
// request needs to be rejected, an error will be returned.
func (q *Queue) Wait() (done func(), err error) {
	done, err = q.queue.Wait()
	if q.metrics != nil && err != nil {
		switch err {
		case jobqueue.ErrStackFull:
			q.metrics.IncCounter(q.errorFullMetricsKey)
		case jobqueue.ErrTimeout:
			q.metrics.IncCounter(q.errorTimeoutMetricsKey)
		default:
			q.metrics.IncCounter(q.errorOtherMetricsKey)
		}
	}

	return done, err
}

// Status returns the current status of a queue.
func (q *Queue) Status() QueueStatus {
	st := q.queue.Status()
	return QueueStatus{
		ActiveRequests: st.ActiveJobs,
		QueuedRequests: st.QueuedJobs,
		Closed:         st.Closed,
	}
}

// Config returns the configuration that the queue was created with.
func (q *Queue) Config() Config {
	return q.config
}

func (q *Queue) reconfigure() {
	q.queue.Reconfigure(jobqueue.Options{
		MaxConcurrency: q.config.MaxConcurrency,
		MaxStackSize:   q.config.MaxQueueSize,
		Timeout:        q.config.Timeout,
	})
}

func (q *Queue) close() {
	q.queue.Close()
}

// RegistryWith (Options) creates a registry with the provided options.
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

// NewRegistry creates a registry with the default options.
func NewRegistry() *Registry {
	return RegistryWith(Options{})
}

func (r *Registry) newQueue(name string, c Config) *Queue {
	q := &Queue{
		config: c,
		// renaming Stack -> Queue in the jobqueue project will follow
		queue: jobqueue.With(jobqueue.Options{
			MaxConcurrency: c.MaxConcurrency,
			MaxStackSize:   c.MaxQueueSize,
			Timeout:        c.Timeout,
		}),
	}

	if r.options.EnableRouteLIFOMetrics {
		if name == "" {
			name = "unknown"
		}

		q.activeRequestsMetricsKey = fmt.Sprintf("lifo.%s.active", name)
		q.queuedRequestsMetricsKey = fmt.Sprintf("lifo.%s.queued", name)
		q.errorFullMetricsKey = fmt.Sprintf("lifo.%s.error.full", name)
		q.errorOtherMetricsKey = fmt.Sprintf("lifo.%s.error.other", name)
		q.errorTimeoutMetricsKey = fmt.Sprintf("lifo.%s.error.timeout", name)
		q.metrics = r.options.Metrics
		r.measure()
	}

	return q
}

// Returns routing.PreProcessor that ensures single lifo filter instance per route
//
// Registry can not implement routing.PreProcessor directly due to unfortunate method name clash with routing.PostProcessor
func (r *Registry) PreProcessor() routing.PreProcessor {
	return registryPreProcessor{}
}

type registryPreProcessor struct{}

func (registryPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	for _, r := range routes {
		lifoCount := 0
		for _, f := range r.Filters {
			if f.Name == filters.LifoName {
				lifoCount++
			}
		}
		// remove all but last lifo instances
		if lifoCount > 1 {
			old := r.Filters
			r.Filters = make([]*eskip.Filter, 0, len(old)-lifoCount+1)
			for _, f := range old {
				if lifoCount > 1 && f.Name == filters.LifoName {
					log.Debugf("Removing non-last %v from %s", f, r.Id)
					lifoCount--
				} else {
					r.Filters = append(r.Filters, f)
				}
			}
		}
	}
	return routes
}

// Do implements routing.PostProcessor and sets the queue for the scheduler filters.
//
// It preserves the existing queue when available.
func (r *Registry) Do(routes []*routing.Route) []*routing.Route {
	rr := make([]*routing.Route, len(routes))
	existingKeys := make(map[string]bool)
	groups := make(map[string][]GroupedLIFOFilter)

	for i, ri := range routes {
		rr[i] = ri
		var lifoCount int
		for _, fi := range ri.Filters {
			if glf, ok := fi.Filter.(GroupedLIFOFilter); ok {
				groupName := glf.Group()
				groups[groupName] = append(groups[groupName], glf)
				continue
			}

			lf, ok := fi.Filter.(LIFOFilter)
			if !ok {
				continue
			}

			lifoCount++
			var q *Queue
			key := fmt.Sprintf("lifo::%s", ri.Id)
			existingKeys[key] = true
			c := lf.Config()
			qi, ok := r.queues.Load(key)
			if ok {
				// Will not reach here if routes were pre-processed
				// because key is derived from the unique route id and
				// pre-processor ensures single lifo filter instance per route
				q = qi.(*Queue)
				if q.config != c {
					q.config = c
					q.reconfigure()
				}
			} else {
				q = r.newQueue(ri.Id, c)
				r.queues.Store(key, q)
			}

			lf.SetQueue(q)
		}

		if lifoCount > 1 {
			log.Warnf("Found multiple lifo filters on route: %q", ri.Id)
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
		existingKeys[key] = true
		qi, ok := r.queues.Load(key)
		if ok {
			q = qi.(*Queue)
			if q.config != c {
				q.config = c
				q.reconfigure()
			}
		} else {
			q = r.newQueue(name, c)
			r.queues.Store(key, q)
		}

		for _, glf := range group {
			glf.SetQueue(q)
		}
	}

	r.queues.Range(func(key, qi interface{}) bool {
		if !existingKeys[key.(string)] {
			qi.(*Queue).close()
			r.queues.Delete(key)
		}

		return true
	})

	return rr
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

// Close closes the registry, including gracefull tearing down the stored
// queues.
func (r *Registry) Close() {
	r.queues.Range(func(_, value interface{}) bool {
		value.(*Queue).close()
		return true
	})

	close(r.quit)
}
