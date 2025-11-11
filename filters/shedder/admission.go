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

func getIntArg(a interface{}) (int, error) {
	if i, ok := a.(int); ok {
		return i, nil
	}

	if f, ok := a.(float64); ok {
		return int(f), nil
	}

	return 0, filters.ErrInvalidFilterParameters
}

func getDurationArg(a interface{}) (time.Duration, error) {
	if s, ok := a.(string); ok {
		return time.ParseDuration(s)
	}
	return 0, filters.ErrInvalidFilterParameters
}

func getFloat64Arg(a interface{}) (float64, error) {
	if f, ok := a.(float64); ok {
		return f, nil
	}

	return 0, filters.ErrInvalidFilterParameters
}

func getModeArg(a interface{}) (mode, error) {
	s, ok := a.(string)
	if !ok {
		return 0, filters.ErrInvalidFilterParameters
	}
	switch s {
	case "active":
		return active, nil
	case "inactive":
		return inactive, nil
	case "logInactive":
		return logInactive, nil
	}

	return 0, filters.ErrInvalidFilterParameters
}

type mode int

const (
	inactive mode = iota + 1
	logInactive
	active
)

func (m mode) String() string {
	switch m {
	case active:
		return "active"
	case inactive:
		return "inactive"
	case logInactive:
		return "logInactive"
	}
	return "unknown"
}

const (
	counterPrefix              = "shedder.admission_control."
	admissionControlSpanName   = "admission_control"
	admissionSignalHeaderKey   = "Admission-Control"
	admissionSignalHeaderValue = "true"
	admissionControlKey        = "shedder:admission_control"
	admissionControlValue      = "reject"
	minWindowSize              = 1
	maxWindowSize              = 100
)

type Options struct {
	Tracer   opentracing.Tracer
	testRand bool
}

type admissionControlPre struct{}

// Do removes duplicate filters, because we can only handle one in a
// chain. The last one will override the others.
func (spec *admissionControlPre) Do(routes []*eskip.Route) []*eskip.Route {
	for _, r := range routes {
		foundAt := -1
		toDelete := make(map[int]struct{})

		for i, f := range r.Filters {
			if f.Name == filters.AdmissionControlName {
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

type admissionControlPost struct {
	filters map[string]*admissionControl
}

// Do implements routing.PostProcessor and makes it possible to close goroutines.
func (spec *admissionControlPost) Do(routes []*routing.Route) []*routing.Route {
	inUse := make(map[string]struct{})

	for _, r := range routes {

		for _, f := range r.Filters {
			if ac, ok := f.Filter.(*admissionControl); ok {
				oldAc, okOld := spec.filters[r.Id]
				if okOld {
					// replace: close the old one
					oldAc.Close()
				}
				spec.filters[r.Id] = ac
				inUse[r.Id] = struct{}{}
			}
		}
	}

	for id, f := range spec.filters {
		if _, ok := inUse[id]; !ok {
			// delete: close the old one
			f.Close()
		}
	}
	return routes
}

type AdmissionControlSpec struct {
	tracer   opentracing.Tracer
	testRand bool
}

type admissionControl struct {
	once   sync.Once
	mu     sync.Mutex
	quit   chan struct{}
	closed bool

	metrics      metrics.Metrics
	metricSuffix string
	tracer       opentracing.Tracer

	mode                 mode
	windowSize           int
	minRps               int
	d                    time.Duration
	successThreshold     float64 // (0,1]
	maxRejectProbability float64 // (0,1]
	exponent             float64 // >0

	averageRpsFactor float64
	totals           []int64
	success          []int64
	counter          *atomic.Int64
	successCounter   *atomic.Int64

	muRand sync.Mutex
	rand   func() float64
}

func randWithSeed() func() float64 {
	return rand.New(rand.NewPCG(2, 3)).Float64
}

func NewAdmissionControl(o Options) filters.Spec {
	tracer := o.Tracer
	if tracer == nil {
		tracer = &opentracing.NoopTracer{}
	}
	return &AdmissionControlSpec{
		tracer:   tracer,
		testRand: o.testRand,
	}
}

func (*AdmissionControlSpec) PreProcessor() *admissionControlPre {
	return &admissionControlPre{}
}

func (*AdmissionControlSpec) PostProcessor() *admissionControlPost {
	return &admissionControlPost{
		filters: make(map[string]*admissionControl),
	}
}

func (*AdmissionControlSpec) Name() string { return filters.AdmissionControlName }

// CreateFilter creates a new admissionControl filter with passed configuration:
//
//	admissionControl(metricSuffix, mode, d, windowSize, minRps, successThreshold, maxRejectProbability, exponent)
//	admissionControl("$app", "active", "1s", 5, 10, 0.1, 0.95, 0.5)
//
// metricSuffix is the suffix key to expose reject counter, should be unique by filter instance
// mode is one of "active", "inactive", "logInactive"
//
//	active will reject traffic
//	inactive will never reject traffic
//	logInactive will not reject traffic, but log to debug filter settings
//
// windowSize is within [minWindowSize, maxWindowSize]
// minRps threshold that needs to be reached such that the filter will apply
// successThreshold is within (0,1] and sets the lowest request success rate at which the filter will not reject requests.
// maxRejectProbability is within (0,1] and sets the upper bound of reject probability.
// exponent >0, 1: linear, 1/2: qudratic, 1/3: cubic, ..
//
// see also https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/admission_control_filter#admission-control
func (spec *AdmissionControlSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var err error

	if len(args) != 8 {
		return nil, filters.ErrInvalidFilterParameters
	}

	metricSuffix, ok := args[0].(string)
	if !ok {
		log.Warn("metricsuffix required as string")
		return nil, filters.ErrInvalidFilterParameters
	}

	mode, err := getModeArg(args[1])
	if err != nil {
		log.Warnf("mode failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	d, err := getDurationArg(args[2])
	if err != nil {
		log.Warnf("d failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	windowSize, err := getIntArg(args[3])
	if err != nil {
		log.Warnf("windowsize failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}
	if minWindowSize > windowSize || windowSize > maxWindowSize {
		log.Warnf("windowsize too small, should be within: [%d,%d], got: %d", minWindowSize, maxWindowSize, windowSize)
		return nil, filters.ErrInvalidFilterParameters
	}

	minRps, err := getIntArg(args[4])
	if err != nil {
		log.Warnf("minRequests failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	threshold, err := getFloat64Arg(args[5])
	if err != nil {
		log.Warnf("threshold failed %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	maxRejectProbability, err := getFloat64Arg(args[6])
	if err != nil {
		log.Warnf("maxRejectProbability failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	exponent, err := getFloat64Arg(args[7])
	if err != nil {
		log.Warnf("exponent failed: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}
	if exponent <= 0.0 {
		log.Warn("exponent should be >0")
		return nil, filters.ErrInvalidFilterParameters
	}

	averageRpsFactor := float64(time.Second) / (float64(d) * float64(windowSize))

	r := rand.Float64
	if spec.testRand {
		r = randWithSeed()
	}

	ac := &admissionControl{
		once: sync.Once{},

		quit:         make(chan struct{}),
		metrics:      metrics.Default,
		metricSuffix: metricSuffix,
		tracer:       spec.tracer,

		mode:                 mode,
		d:                    d,
		windowSize:           windowSize,
		minRps:               minRps,
		successThreshold:     threshold,
		maxRejectProbability: maxRejectProbability,
		exponent:             exponent,

		averageRpsFactor: averageRpsFactor,
		totals:           make([]int64, windowSize),
		success:          make([]int64, windowSize),
		counter:          new(atomic.Int64),
		successCounter:   new(atomic.Int64),
		rand:             r,
		//rand:             randWithSeed(), // flakytest
	}
	go ac.tickWindows(d)
	return ac, nil
}

// Close stops the background goroutine. The filter keeps working on stale data.
func (ac *admissionControl) Close() error {
	ac.once.Do(func() {
		ac.closed = true
		close(ac.quit)
	})
	return nil
}

func (ac *admissionControl) tickWindows(d time.Duration) {
	t := time.NewTicker(d)
	defer t.Stop()
	i := 0

	for range t.C {
		select {
		case <-ac.quit:
			return
		default:
		}
		val := ac.counter.Swap(0)
		ok := ac.successCounter.Swap(0)

		ac.mu.Lock()
		ac.totals[i] = val
		ac.success[i] = ok
		ac.mu.Unlock()

		i = (i + 1) % ac.windowSize
	}
}

func (ac *admissionControl) count() (float64, float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return float64(sum(ac.totals)), float64(sum(ac.success))
}

func sum(a []int64) int64 {
	var result int64
	for _, v := range a {
		result += v
	}
	return result
}

func (ac *admissionControl) setCommonTags(span opentracing.Span) {
	span.SetTag("admissionControl.group", ac.metricSuffix)
	span.SetTag("admissionControl.mode", ac.mode.String())
	span.SetTag("admissionControl.duration", ac.d.String())
	span.SetTag("admissionControl.windowSize", ac.windowSize)
}

// calculates P_{reject} see https://opensource.zalando.com/skipper/reference/filters/#admissioncontrol
func (ac *admissionControl) pReject() float64 {
	var rejectP float64

	total, success := ac.count()
	avgRps := total * ac.averageRpsFactor
	if avgRps < float64(ac.minRps) {
		if ac.mode == logInactive {
			log.Infof("avgRps %0.2f does not reach minRps %d", avgRps, ac.minRps)
		}
		return -1
	}

	s := success / ac.successThreshold
	if ac.mode == logInactive {
		log.Infof("%s: total < s = %v, rejectP = (%0.2f - %0.2f) / (%0.2f + 1)  --- success: %0.2f and threshold: %0.2f", filters.AdmissionControlName, total < s, total, s, total, success, ac.successThreshold)
	}
	if total < s {
		return -1
	}
	rejectP = (total - s) / (total + 1)
	rejectP = math.Pow(rejectP, ac.exponent)

	rejectP = math.Min(rejectP, ac.maxRejectProbability)
	return math.Max(rejectP, 0.0)
}

func (ac *admissionControl) shouldReject() bool {
	p := ac.pReject() // [0, ac.maxRejectProbability] and -1 to disable
	var r float64
	ac.muRand.Lock()
	/* #nosec */
	r = ac.rand() // [0,1)
	ac.muRand.Unlock()

	if ac.mode == logInactive {
		log.Infof("%s: p: %0.2f, r: %0.2f", filters.AdmissionControlName, p, r)
	}

	return p > r
}

func (ac *admissionControl) Request(ctx filters.FilterContext) {
	span := ac.startSpan(ctx.Request().Context())
	defer span.Finish()

	ac.setCommonTags(span)
	ac.metrics.IncCounter(counterPrefix + "total." + ac.metricSuffix)

	if ac.shouldReject() {
		ac.metrics.IncCounter(counterPrefix + "reject." + ac.metricSuffix)
		ext.Error.Set(span, true)

		ctx.StateBag()[admissionControlKey] = admissionControlValue

		// shadow mode to measure data
		if ac.mode != active {
			return
		}

		header := make(http.Header)
		header.Set(admissionSignalHeaderKey, admissionSignalHeaderValue)
		ctx.Serve(&http.Response{
			Header:     header,
			StatusCode: http.StatusServiceUnavailable,
		})
	}
}

func (ac *admissionControl) Response(ctx filters.FilterContext) {
	// we don't want to count our short cutted responses as errors
	if ctx.StateBag()[admissionControlKey] == admissionControlValue {
		return
	}

	// we don't want to count other shedders in the call path as errors
	if ctx.Response().Header.Get(admissionSignalHeaderKey) == admissionSignalHeaderValue {
		return
	}

	if ctx.Response().StatusCode < 499 {
		ac.successCounter.Add(1)
	}
	ac.counter.Add(1)
}

func (ac *admissionControl) startSpan(ctx context.Context) (span opentracing.Span) {
	parent := opentracing.SpanFromContext(ctx)
	if parent != nil {
		span = ac.tracer.StartSpan(admissionControlSpanName, opentracing.ChildOf(parent.Context()))
		ext.Component.Set(span, "skipper")
		span.SetTag("mode", ac.mode.String())
	}
	return
}

// HandleErrorResponse is to opt-in for filters to get called
// Response(ctx) in case of errors via proxy. It has to return true to
// opt-in.
func (ac *admissionControl) HandleErrorResponse() bool { return true }
