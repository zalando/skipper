package proxy

import (
	stdlibcontext "context"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/routing"
)

const (
	// DefaultHedgeBudgetPercent is the default maximum hedge rate as a
	// percentage of the observed request rate of a route.
	DefaultHedgeBudgetPercent = 5.0

	// DefaultHedgePercentile is the default percentile of the observed latency
	// that is used as the adaptive hedge delay.
	DefaultHedgePercentile = 0.9

	// DefaultHedgeMinDelay is the default lower limit of the adaptive hedge delay.
	DefaultHedgeMinDelay = 10 * time.Millisecond

	hedgeBurst            = 10
	hedgeColdStartMinReqs = 100
	hedgeFastTau          = 10 * time.Second
	hedgeSlowTau          = 5 * time.Minute
	hedgeWindowSeconds    = 10
	hedgeMinWindowReqs    = 20
	hedgeMaxErrorRatio    = 0.1
	hedgeMaxSurgeRatio    = 1.5
	hedgeMinBaselineRPS   = 1.0
	hedgeIdleTTL          = 5 * time.Minute
	hedgeSweepInterval    = time.Minute
	hedgeLatencyShift     = 2.0
	hedgeMinPercentile    = 0.5
)

// HedgeParams configures hedged requests. A nil *HedgeParams in Params
// disables hedged requests.
type HedgeParams struct {
	// FixedDelay is the time to wait for the response headers of the
	// primary attempt before a hedge attempt is sent. It must be greater
	// than zero, otherwise hedged requests are disabled.
	FixedDelay time.Duration

	// BudgetPercent is the maximum hedge rate as a percentage of the
	// observed request rate of a route. Defaults to DefaultHedgeBudgetPercent.
	BudgetPercent float64

	// RouteCounters enables the hedge counters per route ID, in addition to
	// the counters for all routes. Every route ID adds a Prometheus series
	// that is never removed.
	RouteCounters bool

	// Percentile enables the adaptive delay if it is greater than zero: once a
	// route has enough samples, the delay is this percentile of the observed
	// response header latency of the route, and FixedDelay is used before
	// that. It must be zero or in [0.5, 1). Zero uses FixedDelay only.
	Percentile float64

	// MinDelay is the lower limit of the adaptive delay. It does not apply
	// to FixedDelay.
	MinDelay time.Duration
}

type hedger struct {
	delay         time.Duration
	budgetPercent float64
	routeCounters bool
	percentile    float64
	minDelay      time.Duration

	mu     sync.RWMutex
	routes map[string]*routeHedge
}

func newHedger(p *HedgeParams, quit <-chan struct{}) *hedger {
	h := &hedger{
		delay:         p.FixedDelay,
		budgetPercent: p.BudgetPercent,
		routeCounters: p.RouteCounters,
		percentile:    p.Percentile,
		minDelay:      p.MinDelay,
		routes:        make(map[string]*routeHedge),
	}
	if h.budgetPercent <= 0 {
		h.budgetPercent = DefaultHedgeBudgetPercent
	}
	go h.sweepLoop(quit)
	return h
}

func (h *hedger) route(id string) *routeHedge {
	h.mu.RLock()
	r := h.routes[id]
	h.mu.RUnlock()
	if r != nil {
		return r
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if r = h.routes[id]; r == nil {
		q := h.percentile
		if q <= 0 {
			q = 0.9
		}
		r = &routeHedge{tokens: hedgeBurst, lastSeen: time.Now(), lat: newLatencyEstimator(q)}
		h.routes[id] = r
	}
	return r
}

func (h *hedger) forRoute(rt *routing.Route) *routeHedge {
	r := h.route(rt.Id)
	r.sync(rt)
	return r
}

func (h *hedger) sweepLoop(quit <-chan struct{}) {
	t := time.NewTicker(hedgeSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			h.sweep(time.Now())
		case <-quit:
			return
		}
	}
}

func (h *hedger) sweep(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, r := range h.routes {
		if r.idleSince(now) > hedgeIdleTTL {
			delete(h.routes, id)
		}
	}
}

type hedgeDecision int

const (
	hedgeAllowed hedgeDecision = iota
	hedgeColdStart
	hedgeThrottled
	hedgeBudgetExhausted
)

type hedgeWindowBucket struct{ reqs, errs int }

type routeHedge struct {
	mu       sync.Mutex
	seen     int64
	lastSeen time.Time
	route    *routing.Route
	def      string
	lat      *latencyEstimator

	tokens     float64
	lastRefill time.Time

	sec     int64
	ticks   int64
	curReqs int
	fast    float64
	slow    float64
	window  [hedgeWindowSeconds]hedgeWindowBucket
}

func routeDefinition(rt *routing.Route) string {
	d := rt.Route
	d.LBEndpoints = nil
	return d.String()
}

func (r *routeHedge) sync(rt *routing.Route) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.route == rt {
		return
	}
	r.route = rt

	def := routeDefinition(rt)
	if r.def != "" && r.def != def {
		r.seen = 0
		r.tokens = hedgeBurst
		r.lastRefill = time.Time{}
		r.sec, r.ticks, r.curReqs = 0, 0, 0
		r.fast, r.slow = 0, 0
		r.window = [hedgeWindowSeconds]hedgeWindowBucket{}
		if r.lat != nil {
			r.lat.reset()
		}
	}
	r.def = def
}

func ewmaAlpha(tau time.Duration, ticks int64) float64 {
	a := 1 - math.Exp(-1/tau.Seconds())
	if warm := 1 / float64(ticks+1); warm > a {
		return warm
	}
	return a
}

func (r *routeHedge) advance(now time.Time) {
	s := now.Unix()
	if r.sec == 0 {
		r.sec = s
		return
	}
	if s <= r.sec {
		return
	}

	if s-r.sec > 10*hedgeWindowSeconds {
		r.fast, r.slow, r.curReqs, r.ticks = 0, 0, 0, 0
		r.window = [hedgeWindowSeconds]hedgeWindowBucket{}
		r.sec = s
		return
	}

	for t := r.sec + 1; t <= s; t++ {
		rate := float64(r.curReqs)
		r.fast += ewmaAlpha(hedgeFastTau, r.ticks) * (rate - r.fast)
		r.slow += ewmaAlpha(hedgeSlowTau, r.ticks) * (rate - r.slow)
		r.ticks++
		r.curReqs = 0
		r.window[t%hedgeWindowSeconds] = hedgeWindowBucket{}
	}
	r.sec = s
}

func (r *routeHedge) arrive(now time.Time) {
	r.mu.Lock()
	r.advance(now)
	r.seen++
	r.curReqs++
	r.window[r.sec%hedgeWindowSeconds].reqs++
	r.lastSeen = now
	r.mu.Unlock()
}

func (r *routeHedge) observe(now time.Time, failed bool) {
	if !failed {
		return
	}
	r.mu.Lock()
	r.advance(now)
	r.window[r.sec%hedgeWindowSeconds].errs++
	r.mu.Unlock()
}

func (r *routeHedge) idleSince(now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return now.Sub(r.lastSeen)
}

func (r *routeHedge) throttled() bool {
	var reqs, errs int
	for _, b := range r.window {
		reqs += b.reqs
		errs += b.errs
	}
	if reqs >= hedgeMinWindowReqs && float64(errs)/float64(reqs) > hedgeMaxErrorRatio {
		return true
	}

	return r.slow >= hedgeMinBaselineRPS && r.fast > hedgeMaxSurgeRatio*r.slow
}

func (r *routeHedge) latencyShifted(now time.Time) bool {
	if r.lat == nil {
		return false
	}
	cur, prev, ok := r.lat.medianShift(now)
	return ok && float64(cur) > hedgeLatencyShift*float64(prev)
}

func (r *routeHedge) record(now time.Time, d time.Duration) {
	if r.lat != nil {
		r.lat.add(now, d)
	}
}

func (h *hedger) delayFor(r *routeHedge, now time.Time) time.Duration {
	if h.percentile <= 0 || r.lat == nil {
		return h.delay
	}
	d, _ := r.lat.percentile(now)
	if d <= 0 {
		return h.delay
	}
	return max(d, h.minDelay)
}

func (r *routeHedge) refill(now time.Time, percent float64) {
	if !r.lastRefill.IsZero() {
		dt := now.Sub(r.lastRefill).Seconds()
		r.tokens += dt * r.fast * percent / 100
		if r.tokens > hedgeBurst {
			r.tokens = hedgeBurst
		}
	}
	r.lastRefill = now
}

func (r *routeHedge) acquire(now time.Time, percent float64) hedgeDecision {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.advance(now)
	if r.seen < hedgeColdStartMinReqs {
		return hedgeColdStart
	}
	if r.throttled() || r.latencyShifted(now) {
		return hedgeThrottled
	}

	r.refill(now, percent)
	if r.tokens < 1 {
		return hedgeBudgetExhausted
	}
	r.tokens--
	return hedgeAllowed
}

func (r *routeHedge) refund() {
	r.mu.Lock()
	if r.tokens < hedgeBurst {
		r.tokens++
	}
	r.mu.Unlock()
}

type primaryOutcome struct {
	skip   bool
	failed bool
}

type hedgeAttempt struct {
	resp   *http.Response
	err    error
	hedge  bool
	ep     *routing.LBEndpoint
	cancel stdlibcontext.CancelFunc
}

func (a *hedgeAttempt) rank() int {
	switch {
	case a.err != nil:
		return 2
	case a.resp.StatusCode >= http.StatusInternalServerError:
		return 1
	default:
		return 0
	}
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancel stdlibcontext.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func (a *hedgeAttempt) release() (*http.Response, error) {
	if a.err != nil {
		a.cancel()
		return nil, a.err
	}
	if a.resp.Body == nil || a.resp.Body == http.NoBody {
		a.cancel()
		return a.resp, nil
	}
	a.resp.Body = &cancelOnCloseBody{ReadCloser: a.resp.Body, cancel: a.cancel}
	return a.resp, nil
}

func discardAttempt(a *hedgeAttempt) {
	a.cancel()
	if a.resp != nil && a.resp.Body != nil {
		_ = a.resp.Body.Close()
	}
}

func (p *Proxy) hedgeCount(ctx *context, name string) {
	p.metrics.IncCounter("hedge." + name)
	if p.hedger.routeCounters {
		p.metrics.IncCounter("hedge." + ctx.route.Id + "." + name)
	}
}

func (p *Proxy) hedgeable(ctx *context, req *http.Request) bool {
	if ctx.route == nil || ctx.route.BackendType != eskip.LBBackend {
		return false
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	if req.Body != nil && req.Body != http.NoBody {
		return false
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return false
	}
	if ctx.arrivedAsShadow || isUpgradeRequest(ctx.Request()) {
		return false
	}
	if ctx.route.Route.LBAlgorithm == loadbalancer.ConsistentHash.String() {
		return false
	}

	bag := ctx.StateBag()
	if _, ok := bag[filters.BackendHedgeDisabled]; ok {
		return false
	}
	if _, ok := bag[filters.BackendIsProxyKey]; ok {
		return false
	}
	return true
}

func backendFailed(req *http.Request, resp *http.Response, err error) bool {
	if err != nil {
		return req.Context().Err() == nil
	}
	return resp.StatusCode >= http.StatusInternalServerError
}

// preTrace is req before the client trace was injected: the hedge request is
// built from it so that both attempts do not write to the same trace.
func (p *Proxy) roundTrip(ctx *context, rt http.RoundTripper, req, preTrace *http.Request) (*http.Response, error) {
	ctx.hedgePrimary = primaryOutcome{}
	if p.hedger == nil || !p.hedgeable(ctx, req) {
		return rt.RoundTrip(req)
	}
	return p.hedgedRoundTrip(ctx, rt, req, preTrace)
}

func (p *Proxy) hedgedRoundTrip(ctx *context, rt http.RoundTripper, req, preTrace *http.Request) (*http.Response, error) {
	h := p.hedger
	state := h.forRoute(ctx.route)
	start := time.Now()
	state.arrive(start)
	delay := h.delayFor(state, start)

	if dl, ok := req.Context().Deadline(); ok && time.Until(dl) <= delay {
		p.hedgeCount(ctx, "skipped_deadline")
		resp, err := rt.RoundTrip(req)
		now := time.Now()
		state.observe(now, backendFailed(req, resp, err))
		if err == nil && resp.StatusCode < http.StatusInternalServerError {
			state.record(now, now.Sub(start))
		}
		return resp, err
	}

	primaryCtx, primaryCancel := stdlibcontext.WithCancel(req.Context())
	primary := &hedgeAttempt{cancel: primaryCancel}
	ch := make(chan *hedgeAttempt, 2)
	go func() {
		primary.resp, primary.err = rt.RoundTrip(req.WithContext(primaryCtx))
		ch <- primary
	}()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	var winner *hedgeAttempt
	select {
	case winner = <-ch:
	case <-timer.C:
		winner, ctx.hedgePrimary = p.hedge(ctx, rt, preTrace, req.URL.Host, state, primary, ch)
	}

	now := time.Now()
	state.observe(now, backendFailed(req, winner.resp, winner.err))
	if winner.err == nil && winner.resp.StatusCode < http.StatusInternalServerError {
		state.record(now, now.Sub(start))
	}
	if winner.hedge {
		ctx.backendZone = winner.ep.Zone
	}
	return winner.release()
}

func (p *Proxy) hedge(
	ctx *context,
	rt http.RoundTripper,
	preTrace *http.Request,
	primaryHost string,
	state *routeHedge,
	primary *hedgeAttempt,
	ch chan *hedgeAttempt,
) (*hedgeAttempt, primaryOutcome) {
	h := p.hedger

	switch state.acquire(time.Now(), h.budgetPercent) {
	case hedgeColdStart:
		return <-ch, primaryOutcome{}
	case hedgeThrottled:
		p.hedgeCount(ctx, "throttled")
		return <-ch, primaryOutcome{}
	case hedgeBudgetExhausted:
		p.hedgeCount(ctx, "budget_exhausted")
		return <-ch, primaryOutcome{}
	}

	ep := p.selectEndpointExcluding(ctx, primaryHost)
	if ep == nil {
		state.refund()
		p.hedgeCount(ctx, "no_alternate")
		return <-ch, primaryOutcome{}
	}

	hedgeCtx, hedgeCancel := stdlibcontext.WithCancel(preTrace.Context())
	hreq := preTrace.Clone(hedgeCtx)
	hreq.URL.Scheme = ep.Scheme
	hreq.URL.Host = ep.Host

	hrt, err := p.getRoundTripper(ctx, hreq)
	if err != nil {
		hedgeCancel()
		state.refund()
		return <-ch, primaryOutcome{}
	}
	if _, rejected := p.rejectBackend(ctx, hreq); rejected {
		hedgeCancel()
		state.refund()
		return <-ch, primaryOutcome{}
	}

	p.hedgeCount(ctx, "fired")
	p.tracing.setTag(ctx.proxySpan, "hedge", true)
	p.tracing.logEvent(ctx.proxySpan, "hedge", ep.Host)

	if ep.Metrics != nil {
		ep.Metrics.IncInflightRequest()
	}
	go func() {
		a := &hedgeAttempt{hedge: true, ep: ep, cancel: hedgeCancel}
		a.resp, a.err = hrt.RoundTrip(hreq)
		if ep.Metrics != nil {
			ep.Metrics.DecInflightRequest()
			ep.Metrics.IncRequests(routing.IncRequestsOptions{FailedRoundTrip: a.err != nil && hedgeCtx.Err() == nil})
		}
		ch <- a
	}()

	first := <-ch
	var winner, loser *hedgeAttempt
	var outcome primaryOutcome
	if first.rank() == 0 {
		winner = first
		loser = otherAttempt(primary, first, hedgeCancel)
		go func() { discardAttempt(<-ch) }()
		outcome.skip = first.hedge
	} else {
		second := <-ch
		winner, loser = first, second
		if second.rank() < first.rank() || (second.rank() == first.rank() && first.hedge) {
			winner, loser = second, first
		}
		discardAttempt(loser)
		outcome.failed = winner.hedge && primary.err != nil
	}
	loser.cancel()

	if winner.hedge {
		p.hedgeCount(ctx, "won")
		p.tracing.setTag(ctx.proxySpan, "hedge.winner", "hedge")
	} else {
		p.tracing.setTag(ctx.proxySpan, "hedge.winner", "primary")
	}
	return winner, outcome
}

func otherAttempt(primary, first *hedgeAttempt, hedgeCancel stdlibcontext.CancelFunc) *hedgeAttempt {
	if first.hedge {
		return primary
	}
	return &hedgeAttempt{cancel: hedgeCancel}
}
