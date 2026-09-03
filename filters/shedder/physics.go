package shedder

import (
	"context"
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

const (
	physicsCounterPrefix  = "shedder.physics."
	physicsSpanName       = "physics_shedder"
	physicsStateBagKey    = "shedder:physics"
	physicsStateBagReject = "reject"

	physicsTickDuration    = 100 * time.Millisecond
	physicsEwmaAlpha       = 0.01
	physicsSigmaMultiplier = 3.0
	physicsErrorWeight     = 5.0
	physicsMaxRejectProb   = 0.95
	physicsWarmupTicks     = 50

	physicsMinWindow = 2 * physicsTickDuration
	physicsMaxWindow = 60 * time.Second
)

type PhysicsShedderOptions struct {
	Tracer   opentracing.Tracer
	testRand bool
}

// PhysicsShedderSpec is exported so tests and wiring code can type-assert
// the returned filters.Spec to call PreProcessor/PostProcessor.
type PhysicsShedderSpec struct {
	tracer   opentracing.Tracer
	testRand bool
}

type physicsShedderPre struct{}

// Do removes duplicate physicsShedder filters per route. Only the last
// instance survives so a chain always has at most one.
func (*physicsShedderPre) Do(routes []*eskip.Route) []*eskip.Route {
	for _, r := range routes {
		foundAt := -1
		toDelete := make(map[int]struct{})

		for i, f := range r.Filters {
			if f.Name == filters.PhysicsShedderName {
				if foundAt != -1 {
					toDelete[foundAt] = struct{}{}
				}
				foundAt = i
			}
		}

		if len(toDelete) == 0 {
			continue
		}

		rf := make([]*eskip.Filter, 0, len(r.Filters)-len(toDelete))
		for i, f := range r.Filters {
			if _, ok := toDelete[i]; !ok {
				rf = append(rf, f)
			}
		}
		r.Filters = rf
	}

	return routes
}

type physicsShedderPost struct {
	filters map[string]*physicsShedder
}

// Do implements routing.PostProcessor so we can stop tick goroutines when
// a route is replaced or removed.
func (p *physicsShedderPost) Do(routes []*routing.Route) []*routing.Route {
	inUse := make(map[string]struct{})

	for _, r := range routes {
		for _, f := range r.Filters {
			if ps, ok := f.Filter.(*physicsShedder); ok {
				if old, okOld := p.filters[r.Id]; okOld && old != ps {
					old.Close()
				}
				p.filters[r.Id] = ps
				inUse[r.Id] = struct{}{}
			}
		}
	}

	for id, f := range p.filters {
		if _, ok := inUse[id]; !ok {
			f.Close()
			delete(p.filters, id)
		}
	}
	return routes
}

type physicsShedder struct {
	once   sync.Once
	quit   chan struct{}
	done   chan struct{}
	closed bool

	metrics      metrics.Metrics
	metricSuffix string
	tracer       opentracing.Tracer

	mode          mode
	tickDuration  time.Duration
	windowSize    int
	latencyTarget time.Duration

	// hot path counters, swapped by tickWindows
	counter      atomic.Int64
	errorCounter atomic.Int64
	latencyNs    atomic.Int64

	mu           sync.Mutex
	totals       []int64
	errors       []int64
	latencySumNs []int64
	bucketIdx    int
	ticksSeen    int

	// EWMA baseline of R, owned by tick goroutine.
	ewmaMu     float64
	ewmaVar    float64
	ewmaPrimed bool

	// Published reject probability (float64 bits) read lock-free from hot path.
	pRejectBits atomic.Uint64

	muRand sync.Mutex
	rand   func() float64
}

// NewPhysicsShedder creates the filter spec. A single spec is shared
// across routes; per-route state lives on the filter instance.
func NewPhysicsShedder(o PhysicsShedderOptions) filters.Spec {
	tracer := o.Tracer
	if tracer == nil {
		tracer = &opentracing.NoopTracer{}
	}
	return &PhysicsShedderSpec{
		tracer:   tracer,
		testRand: o.testRand,
	}
}

func (*PhysicsShedderSpec) PreProcessor() *physicsShedderPre {
	return &physicsShedderPre{}
}

func (*PhysicsShedderSpec) PostProcessor() *physicsShedderPost {
	return &physicsShedderPost{
		filters: make(map[string]*physicsShedder),
	}
}

func (*PhysicsShedderSpec) Name() string { return filters.PhysicsShedderName }

// CreateFilter parses the filter arguments:
//
//	physicsShedder(metricSuffix, mode, latencyTarget)
//	physicsShedder(metricSuffix, mode, latencyTarget, window)
//
// metricSuffix identifies this filter in metrics.
// mode is one of "active", "inactive", "logInactive".
// latencyTarget is the expected per-request latency (e.g. "200ms").
// window is the observation window (default "5s").
func (spec *PhysicsShedderSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 3 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	metricSuffix, ok := args[0].(string)
	if !ok || metricSuffix == "" {
		log.Warn("physicsShedder: metricSuffix must be a non-empty string")
		return nil, filters.ErrInvalidFilterParameters
	}

	md, err := getModeArg(args[1])
	if err != nil {
		log.Warnf("physicsShedder: mode failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	latencyTarget, err := getDurationArg(args[2])
	if err != nil || latencyTarget <= 0 {
		log.Warnf("physicsShedder: latencyTarget must be a positive duration: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	window := 5 * time.Second
	if len(args) == 4 {
		window, err = getDurationArg(args[3])
		if err != nil {
			log.Warnf("physicsShedder: window failed: %v", err)
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	if window < physicsMinWindow || window > physicsMaxWindow {
		log.Warnf("physicsShedder: window out of range [%s, %s], got %s",
			physicsMinWindow, physicsMaxWindow, window)
		return nil, filters.ErrInvalidFilterParameters
	}

	windowSize := int(window / physicsTickDuration)

	r := rand.Float64
	if spec.testRand {
		r = randWithSeed()
	}

	ps := &physicsShedder{
		quit:          make(chan struct{}),
		done:          make(chan struct{}),
		metrics:       metrics.Default,
		metricSuffix:  metricSuffix,
		tracer:        spec.tracer,
		mode:          md,
		tickDuration:  physicsTickDuration,
		windowSize:    windowSize,
		latencyTarget: latencyTarget,
		totals:        make([]int64, windowSize),
		errors:        make([]int64, windowSize),
		latencySumNs:  make([]int64, windowSize),
		rand:          r,
	}
	go ps.tickWindows()
	return ps, nil
}

// Close stops the tick goroutine and waits for it to exit. Safe to call
// multiple times.
func (ps *physicsShedder) Close() error {
	ps.once.Do(func() {
		ps.closed = true
		close(ps.quit)
	})
	<-ps.done
	return nil
}

func (ps *physicsShedder) tickWindows() {
	defer close(ps.done)
	t := time.NewTicker(ps.tickDuration)
	defer t.Stop()

	for {
		select {
		case <-ps.quit:
			return
		case <-t.C:
		}

		reqs := ps.counter.Swap(0)
		errs := ps.errorCounter.Swap(0)
		lat := ps.latencyNs.Swap(0)

		ps.mu.Lock()
		ps.totals[ps.bucketIdx] = reqs
		ps.errors[ps.bucketIdx] = errs
		ps.latencySumNs[ps.bucketIdx] = lat
		ps.bucketIdx = (ps.bucketIdx + 1) % ps.windowSize
		if ps.ticksSeen < math.MaxInt32 {
			ps.ticksSeen++
		}
		sumReqs := sum(ps.totals)
		sumErrs := sum(ps.errors)
		sumLat := sum(ps.latencySumNs)
		ticksSeen := ps.ticksSeen
		ps.mu.Unlock()

		r := ps.computeResistance(sumReqs, sumErrs, sumLat)
		threshold := ps.updateBaseline(r)
		rejectP := ps.computeRejectProbability(r, threshold, ticksSeen)

		ps.pRejectBits.Store(math.Float64bits(rejectP))

		ps.publishGauges(r, ps.ewmaMu, threshold)

		if ps.mode == logInactive {
			log.Infof("%s[%s]: R=%.3f mu=%.3f sigma=%.3f threshold=%.3f pReject=%.3f ticks=%d reqs=%d errs=%d",
				filters.PhysicsShedderName, ps.metricSuffix,
				r, ps.ewmaMu, math.Sqrt(ps.ewmaVar), threshold, rejectP, ticksSeen, sumReqs, sumErrs)
		}
	}
}

// computeResistance collapses window-aggregated metrics into a single R
// value combining latency-vs-target and error rate.
func (ps *physicsShedder) computeResistance(sumReqs, sumErrs, sumLatNs int64) float64 {
	if sumReqs <= 0 {
		return 0
	}
	avgLatNs := float64(sumLatNs) / float64(sumReqs)
	latencyRatio := avgLatNs / float64(ps.latencyTarget)
	errorRate := float64(sumErrs) / float64(sumReqs)
	return latencyRatio + physicsErrorWeight*errorRate
}

// updateBaseline advances the EWMA running mean and variance of R and
// returns the shed threshold (mu + k*sigma). Called only from the tick
// goroutine.
func (ps *physicsShedder) updateBaseline(r float64) float64 {
	if !ps.ewmaPrimed {
		ps.ewmaMu = r
		ps.ewmaVar = 0
		ps.ewmaPrimed = true
	} else {
		diff := r - ps.ewmaMu
		ps.ewmaMu += physicsEwmaAlpha * diff
		ps.ewmaVar = (1 - physicsEwmaAlpha) * (ps.ewmaVar + physicsEwmaAlpha*diff*diff)
	}
	return ps.ewmaMu + physicsSigmaMultiplier*math.Sqrt(ps.ewmaVar)
}

// computeRejectProbability returns the probability to reject an incoming
// request given the current resistance and adaptive threshold. It is 0
// during warmup or when R is at or below threshold, and is clamped to
// physicsMaxRejectProb otherwise.
func (ps *physicsShedder) computeRejectProbability(r, threshold float64, ticksSeen int) float64 {
	if ticksSeen < physicsWarmupTicks {
		return 0
	}
	if r <= threshold || r <= 0 {
		return 0
	}
	p := (r - threshold) / r
	if p > physicsMaxRejectProb {
		p = physicsMaxRejectProb
	}
	if p < 0 {
		p = 0
	}
	return p
}

func (ps *physicsShedder) publishGauges(r, baseline, threshold float64) {
	if ps.metrics == nil {
		return
	}
	ps.metrics.UpdateGauge(physicsCounterPrefix+"resistance."+ps.metricSuffix, r)
	ps.metrics.UpdateGauge(physicsCounterPrefix+"baseline."+ps.metricSuffix, baseline)
	ps.metrics.UpdateGauge(physicsCounterPrefix+"threshold."+ps.metricSuffix, threshold)
}

func (ps *physicsShedder) pReject() float64 {
	b := ps.pRejectBits.Load()
	return math.Float64frombits(b)
}

func (ps *physicsShedder) setCommonTags(span opentracing.Span, r, threshold, p float64) {
	span.SetTag("physicsShedder.group", ps.metricSuffix)
	span.SetTag("physicsShedder.mode", ps.mode.String())
	span.SetTag("physicsShedder.latencyTarget", ps.latencyTarget.String())
	span.SetTag("physicsShedder.R", r)
	span.SetTag("physicsShedder.threshold", threshold)
	span.SetTag("physicsShedder.pReject", p)
}

func (ps *physicsShedder) startSpan(ctx context.Context) opentracing.Span {
	parent := opentracing.SpanFromContext(ctx)
	if parent == nil {
		return nil
	}
	span := ps.tracer.StartSpan(physicsSpanName, opentracing.ChildOf(parent.Context()))
	ext.Component.Set(span, "skipper")
	span.SetTag("mode", ps.mode.String())
	return span
}

// Request implements the hot-path rejection decision.
func (ps *physicsShedder) Request(ctx filters.FilterContext) {
	span := ps.startSpan(ctx.Request().Context())

	ctx.StateBag()[physicsStartTimeKey] = time.Now()

	if ps.metrics != nil {
		ps.metrics.IncCounter(physicsCounterPrefix + "total." + ps.metricSuffix)
	}

	p := ps.pReject()
	reject := false
	if p > 0 {
		ps.muRand.Lock()
		r := ps.rand()
		ps.muRand.Unlock()
		reject = p > r
	}

	if span != nil {
		ps.setCommonTags(span, 0, 0, p)
		defer span.Finish()
	}

	if !reject {
		return
	}

	if ps.metrics != nil {
		if ps.mode == active {
			ps.metrics.IncCounter(physicsCounterPrefix + "reject." + ps.metricSuffix)
		} else {
			ps.metrics.IncCounter(physicsCounterPrefix + "would_reject." + ps.metricSuffix)
		}
	}

	if span != nil {
		ext.Error.Set(span, true)
	}

	if ps.mode != active {
		return
	}

	ctx.StateBag()[physicsStateBagKey] = physicsStateBagReject

	header := make(http.Header)
	header.Set(admissionSignalHeaderKey, admissionSignalHeaderValue)
	ctx.Serve(&http.Response{
		Header:     header,
		StatusCode: http.StatusServiceUnavailable,
	})
}

// Response records outcome for the tick loop to consume.
func (ps *physicsShedder) Response(ctx filters.FilterContext) {
	// Skip our own short-cut responses.
	if ctx.StateBag()[physicsStateBagKey] == physicsStateBagReject {
		return
	}
	// Skip responses produced by upstream shedders.
	if ctx.Response().Header.Get(admissionSignalHeaderKey) == admissionSignalHeaderValue {
		return
	}

	ps.counter.Add(1)
	if ctx.Response().StatusCode >= 500 {
		ps.errorCounter.Add(1)
	}

	// Latency: use StateBag start time if a predicate/filter set it,
	// otherwise leave latency contribution to zero for this request.
	if startAny, ok := ctx.StateBag()[physicsStartTimeKey]; ok {
		if start, ok := startAny.(time.Time); ok {
			ps.latencyNs.Add(time.Since(start).Nanoseconds())
		}
	}
}

// HandleErrorResponse opts in for Response callbacks on proxy errors.
func (*physicsShedder) HandleErrorResponse() bool { return true }

// physicsStartTimeKey stores request start time in StateBag so Response
// can measure request latency without allocating a context key.
const physicsStartTimeKey = "shedder:physics:start"
