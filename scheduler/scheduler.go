// Package scheduler provides a registry to be used as a postprocessor for the routes
// that use a LIFO filter.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"golang.org/x/sync/semaphore"
)

// note: Config must stay comparable because it is used to detect changes in route specific LIFO config

const (
	// LIFOKey used during routing to pass lifo values from the filters to the proxy.
	LIFOKey = "lifo"
	// FIFOKey used during routing to pass fifo values from the filters to the proxy.
	FIFOKey = "fifo"
)

var (
	ErrQueueFull      = errors.New("queue full")
	ErrQueueTimeout   = errors.New("queue timeout")
	ErrClientCanceled = errors.New("client canceled")
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

// FifoQueue objects implement a FIFO queue for handling requests,
// with a maximum allowed concurrency and queue size. Currently, they
// can be used from the fifo filters in the filters/scheduler package
// only.
type FifoQueue struct {
	queue                    *fifoQueue
	config                   Config
	metrics                  metrics.Metrics
	activeRequestsMetricsKey string
	errorFullMetricsKey      string
	errorOtherMetricsKey     string
	errorTimeoutMetricsKey   string
	queuedRequestsMetricsKey string
}

type fifoQueue struct {
	mu             sync.RWMutex
	counter        *atomic.Int64
	sem            *semaphore.Weighted
	timeout        time.Duration
	maxQueueSize   int64
	maxConcurrency int64
	closed         bool
}

func (fq *fifoQueue) status() QueueStatus {
	fq.mu.RLock()
	maxConcurrency := fq.maxConcurrency
	closed := fq.closed
	fq.mu.RUnlock()

	all := fq.counter.Load()

	var queued, active int64
	if all > maxConcurrency {
		queued = all - maxConcurrency
		active = maxConcurrency
	} else {
		queued = 0
		active = all
	}

	return QueueStatus{
		ActiveRequests: int(active),
		QueuedRequests: int(queued),
		Closed:         closed,
	}
}

func (fq *fifoQueue) close() {
	fq.mu.Lock()
	fq.closed = true
	fq.mu.Unlock()
}

func (fq *fifoQueue) reconfigure(c Config) {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	fq.maxConcurrency = int64(c.MaxConcurrency)
	fq.maxQueueSize = int64(c.MaxQueueSize)
	fq.timeout = c.Timeout
	fq.sem = semaphore.NewWeighted(int64(c.MaxConcurrency))
	fq.counter = new(atomic.Int64)
}

func (fq *fifoQueue) wait(ctx context.Context) (func(), error) {
	fq.mu.RLock()
	maxConcurrency := fq.maxConcurrency
	maxQueueSize := fq.maxQueueSize
	timeout := fq.timeout
	sem := fq.sem
	cnt := fq.counter
	fq.mu.RUnlock()

	// check request context expired
	// https://github.com/golang/go/issues/63615
	if err := ctx.Err(); err != nil {
		switch err {
		case context.DeadlineExceeded:
			return nil, ErrQueueTimeout
		case context.Canceled:
			return nil, ErrClientCanceled
		default:
			// does not exist yet in Go stdlib as of Go1.18.4
			return nil, err
		}
	}

	// handle queue
	all := cnt.Add(1)
	// queue full?
	if all > maxConcurrency+maxQueueSize {
		cnt.Add(-1)
		return nil, ErrQueueFull
	}

	// set timeout
	c, done := context.WithTimeout(ctx, timeout)
	defer done()

	// limit concurrency
	if err := sem.Acquire(c, 1); err != nil {
		cnt.Add(-1)
		switch err {
		case context.DeadlineExceeded:
			return nil, ErrQueueTimeout
		case context.Canceled:
			return nil, ErrClientCanceled
		default:
			// does not exist yet in Go stdlib as of Go1.18.4
			return nil, err
		}
	}

	return func() {
		// postpone release to Response() filter
		cnt.Add(-1)
		sem.Release(1)
	}, nil

}

// Options provides options for the registry.
type Options struct {

	// MetricsUpdateTimeout defines the frequency of how often the
	// FIFO and LIFO metrics are updated when they are enabled.
	// Defaults to 1s.
	MetricsUpdateTimeout time.Duration

	// EnableRouteLIFOMetrics enables collecting metrics about the LIFO queues.
	EnableRouteLIFOMetrics bool

	// EnableRouteFIFOMetrics enables collecting metrics about the FIFO queues.
	EnableRouteFIFOMetrics bool

	// Metrics must be provided to the registry in order to collect the FIFO and LIFO metrics.
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
type Registry struct {
	options   Options
	measuring bool
	quit      chan struct{}

	mu          sync.Mutex
	lifoQueues  map[queueId]*Queue
	lifoDeleted map[*Queue]time.Time
	fifoQueues  map[queueId]*FifoQueue
	fifoDeleted map[*FifoQueue]time.Time
}

type queueId struct {
	name    string
	grouped bool
}

// Amount of time to wait before closing the deleted queues
var queueCloseDelay = 1 * time.Minute

// FIFOFilter is the interface that needs to be implemented by the filters that
// use a FIFO queue maintained by the registry.
type FIFOFilter interface {

	// SetQueue will be used by the registry to pass in the right queue to
	// the filter.
	SetQueue(*FifoQueue)

	// GetQueue is currently used only by tests.
	GetQueue() *FifoQueue

	// Config will be called by the registry once during processing the
	// routing to get the right queue settings from the filter.
	Config() Config
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

// Wait blocks until a request can be processed or needs to be
// rejected.  It returns done() and an error. When it can be
// processed, calling done indicates that it has finished.  It is
// mandatory to call done() the request was processed. When the
// request needs to be rejected, an error will be returned and done
// will be nil.
func (fq *FifoQueue) Wait(ctx context.Context) (func(), error) {
	f, err := fq.queue.wait(ctx)
	if err != nil && fq.metrics != nil {
		switch err {
		case ErrQueueFull:
			fq.metrics.IncCounter(fq.errorFullMetricsKey)
		case ErrQueueTimeout:
			fq.metrics.IncCounter(fq.errorTimeoutMetricsKey)
		case ErrClientCanceled:
			// This case is handled in the proxy with status code 499
		default:
			fq.metrics.IncCounter(fq.errorOtherMetricsKey)
		}
	}
	return f, err
}

// Status returns the current status of a queue.
func (fq *FifoQueue) Status() QueueStatus {
	return fq.queue.status()
}

// Config returns the configuration that the queue was created with.
func (fq *FifoQueue) Config() Config {
	return fq.config
}

// Reconfigure updates the connfiguration of the FifoQueue. It will
// reset the current state.
func (fq *FifoQueue) Reconfigure(c Config) {
	fq.config = c
	fq.queue.reconfigure(c)
}

func (fq *FifoQueue) close() {
	fq.queue.close()
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

func (q *Queue) Close() {
	q.queue.Close()
}

// RegistryWith (Options) creates a registry with the provided options.
func RegistryWith(o Options) *Registry {
	if o.MetricsUpdateTimeout <= 0 {
		o.MetricsUpdateTimeout = time.Second
	}

	return &Registry{
		options:     o,
		quit:        make(chan struct{}),
		fifoQueues:  make(map[queueId]*FifoQueue),
		fifoDeleted: make(map[*FifoQueue]time.Time),
		lifoQueues:  make(map[queueId]*Queue),
		lifoDeleted: make(map[*Queue]time.Time),
	}
}

// NewRegistry creates a registry with the default options.
func NewRegistry() *Registry {
	return RegistryWith(Options{})
}

func (r *Registry) getFifoQueue(id queueId, c Config) *FifoQueue {
	r.mu.Lock()
	defer r.mu.Unlock()

	fq, ok := r.fifoQueues[id]
	if ok {
		if fq.config != c {
			fq.Reconfigure(c)
		}
	} else {
		fq = r.newFifoQueue(id.name, c)
		r.fifoQueues[id] = fq
	}
	return fq
}

func (r *Registry) newFifoQueue(name string, c Config) *FifoQueue {
	q := &FifoQueue{
		config: c,
		queue: &fifoQueue{
			counter:        new(atomic.Int64),
			sem:            semaphore.NewWeighted(int64(c.MaxConcurrency)),
			maxConcurrency: int64(c.MaxConcurrency),
			maxQueueSize:   int64(c.MaxQueueSize),
			timeout:        c.Timeout,
		},
	}

	if r.options.EnableRouteFIFOMetrics {
		if name == "" {
			name = "unknown"
		}

		q.activeRequestsMetricsKey = fmt.Sprintf("fifo.%s.active", name)
		q.queuedRequestsMetricsKey = fmt.Sprintf("fifo.%s.queued", name)
		q.errorFullMetricsKey = fmt.Sprintf("fifo.%s.error.full", name)
		q.errorOtherMetricsKey = fmt.Sprintf("fifo.%s.error.other", name)
		q.errorTimeoutMetricsKey = fmt.Sprintf("fifo.%s.error.timeout", name)
		q.metrics = r.options.Metrics
		r.measure()
	}

	return q
}

func (r *Registry) getQueue(id queueId, c Config) *Queue {
	r.mu.Lock()
	defer r.mu.Unlock()

	q, ok := r.lifoQueues[id]
	if ok {
		if q.config != c {
			q.config = c
			q.reconfigure()
		}
	} else {
		q = r.newQueue(id.name, c)
		r.lifoQueues[id] = q
	}
	return q
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

func (r *Registry) deleteUnused(inUse map[queueId]struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	closeCutoff := now.Add(-queueCloseDelay)

	// fifo
	for q, deleted := range r.fifoDeleted {
		if deleted.Before(closeCutoff) {
			delete(r.fifoDeleted, q)
			q.close()
		}
	}
	for id, q := range r.fifoQueues {
		if _, ok := inUse[id]; !ok {
			delete(r.fifoQueues, id)
			r.fifoDeleted[q] = now
		}
	}

	// lifo
	for q, deleted := range r.lifoDeleted {
		if deleted.Before(closeCutoff) {
			delete(r.lifoDeleted, q)
			q.Close()
		}
	}
	for id, q := range r.lifoQueues {
		if _, ok := inUse[id]; !ok {
			delete(r.lifoQueues, id)
			r.lifoDeleted[q] = now
		}
	}
}

// PreProcessor returns routing.PreProcessor that ensures single lifo filter instance per route
//
// Registry can not implement routing.PreProcessor directly due to unfortunate method name clash with routing.PostProcessor
func (r *Registry) PreProcessor() routing.PreProcessor {
	return registryPreProcessor{}
}

type registryPreProcessor struct{}

func (registryPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	for _, r := range routes {
		lifoCount := 0
		fifoCount := 0
		for _, f := range r.Filters {
			switch f.Name {
			case filters.FifoName:
				fifoCount++
			case filters.LifoName:
				lifoCount++
			}
		}
		// remove all but last fifo instances
		if fifoCount > 1 {
			old := r.Filters
			r.Filters = make([]*eskip.Filter, 0, len(old)-fifoCount+1)
			for _, f := range old {
				if fifoCount > 1 && f.Name == filters.FifoName {
					log.Debugf("Removing non-last %v from %s", f, r.Id)
					fifoCount--
				} else {
					r.Filters = append(r.Filters, f)
				}
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
	inUse := make(map[queueId]struct{})
	groups := make(map[string][]GroupedLIFOFilter)

	for i, ri := range routes {
		rr[i] = ri
		for _, fi := range ri.Filters {
			if ff, ok := fi.Filter.(FIFOFilter); ok {
				id := queueId{ri.Id, false}
				inUse[id] = struct{}{}
				fq := r.getFifoQueue(id, ff.Config())
				ff.SetQueue(fq)
				continue
			}

			if glf, ok := fi.Filter.(GroupedLIFOFilter); ok {
				groupName := glf.Group()
				groups[groupName] = append(groups[groupName], glf)
				continue
			}

			lf, ok := fi.Filter.(LIFOFilter)
			if !ok {
				continue
			}

			id := queueId{ri.Id, false}
			inUse[id] = struct{}{}

			q := r.getQueue(id, lf.Config())

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

		id := queueId{name, true}
		inUse[id] = struct{}{}

		q := r.getQueue(id, c)

		for _, glf := range group {
			glf.SetQueue(q)
		}
	}

	r.deleteUnused(inUse)

	return rr
}

func (r *Registry) measure() {
	if r.options.Metrics == nil || r.measuring {
		return
	}

	r.measuring = true
	go func() {
		ticker := time.NewTicker(r.options.MetricsUpdateTimeout)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.updateMetrics()
			case <-r.quit:
				return
			}
		}
	}()
}

func (r *Registry) updateMetrics() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, q := range r.fifoQueues {
		s := q.Status()
		r.options.Metrics.UpdateGauge(q.activeRequestsMetricsKey, float64(s.ActiveRequests))
		r.options.Metrics.UpdateGauge(q.queuedRequestsMetricsKey, float64(s.QueuedRequests))
	}

	for _, q := range r.lifoQueues {
		s := q.Status()
		r.options.Metrics.UpdateGauge(q.activeRequestsMetricsKey, float64(s.ActiveRequests))
		r.options.Metrics.UpdateGauge(q.queuedRequestsMetricsKey, float64(s.QueuedRequests))
	}
}

func (r *Registry) UpdateMetrics() {
	if r.options.Metrics != nil {
		r.updateMetrics()
	}
}

// Close closes the registry, including graceful tearing down the stored queues.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for q := range r.fifoDeleted {
		delete(r.fifoDeleted, q)
		q.close()
	}

	for q := range r.lifoDeleted {
		delete(r.lifoDeleted, q)
		q.Close()
	}

	for id, q := range r.lifoQueues {
		delete(r.lifoQueues, id)
		q.Close()
	}

	close(r.quit)
}
