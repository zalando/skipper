package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	skpnet "github.com/zalando/skipper/net"
	"golang.org/x/sync/singleflight"
)

const (
	Name       = filters.CacheName
	filterName = Name

	// State bag keys.
	stateBagKey         = "cache:key"
	stateBagBaseKey     = "cache:base-key"
	stateBagRequestTime = "cache:request-time"
	stateBagNoStore     = "cache:no-store"

	// revalidateHeader is set on background revalidation requests so the cache
	// filter skips the cache lookup and lets the response populate a fresh entry.
	revalidateHeader = "X-Cache-Revalidate"

	// X-Cache-Status header values.
	cacheStatusHeader = "X-Cache-Status"
	cacheStatusHit    = "HIT"
	cacheStatusMiss   = "MISS"
	cacheStatusStale  = "STALE"

	originDispatchDirect   = "direct"
	originDispatchPipeline = "pipeline"

	// revalQueueSize is the capacity of the shared revalidation job queue.
	// Sized to absorb short bursts; jobs are dropped (with reval_dropped metric)
	// if the worker cannot keep up.
	revalQueueSize = 256
)

// Options configures the cache filter.
type Options struct {
	MaxBytes   int64            // maximum number of bytes the in-process LRU (L1) is allowed to hold across all cached entries
	ListenAddr string           // Skipper's own address; revalidation requests loop back through it so the full filter chain runs
	NetOpts    skpnet.Options   // HTTP client options for background worker that re-fetches stale entries from origin
	L2Client   L2Client         // optional L2 cache; nil = in-process LRU only
	IsNoL2Err  func(error) bool // returns true if no L2Client error
	L1TTL      time.Duration    // max TTL for write-through L1 warming; 0 = write-around
	Metrics    metrics.Metrics  // nil defaults to metrics.Default
}

// filterCacheKey identifies a unique cache filter configuration for registry lookup.
// Routes with identical configuration share the same cacheFilter instance.
type filterCacheKey struct {
	ttl          time.Duration
	errorTTL     time.Duration
	swrWindow    time.Duration
	staleIfError time.Duration
	keyHeaders   string // canonical: sorted, comma-joined
	rfcMode      bool
}

type cacheSpec struct {
	maxBytes   int64
	listenAddr string
	client     *skpnet.Client
	storage    Storage     // shared across all filter instances
	lruStorage *LRUStorage // always non-nil; direct reference to L1, even when storage is L2Storage
	metrics    metrics.Metrics
	revalJobs  chan revalJob
	ctx        context.Context
	cancel     context.CancelFunc
	bgWg       sync.WaitGroup
	closeOnce  sync.Once
	muFilter   sync.Mutex
	filters    map[filterCacheKey]*cacheFilter
}

// statusCodeClass buckets an HTTP status code for the cache.result_total
// code_class label.
func statusCodeClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}

// NewCacheFilter returns a Spec for the cache() filter. maxBytes is the
// in-memory storage budget for the shared LRU cache backing all filter
// instances created from this Spec.
// listenAddr is Skipper's own listener address (e.g. ":9090"); revalidation
// requests are sent back through Skipper so the full filter chain runs.
//
// Route usage (RFC mode — upstream Cache-Control is fully authoritative):
//
//	-> cache() -> "https://example.org"
//
// Route usage (force mode — operator TTL is authoritative, upstream directives ignored):
//
//	-> cache("5m", "15s", "30s") -> "https://example.org"
//
// Combining force mode with stale-if-error:
//
//	-> cache("5m", "15s", "30s", "60s") -> "https://example.org"
func NewCacheFilter(opts Options) filters.Spec {
	if opts.Metrics == nil {
		opts.Metrics = metrics.Default
	}

	m := opts.Metrics
	lru := NewLRUStorage(opts.MaxBytes, func() {
		m.IncCounter("cache.lru_eviction")
	}, m)

	var store Storage = lru
	if p, ok := m.(interface{ SetCacheLRUCapacity(float64) }); ok {
		p.SetCacheLRUCapacity(float64(opts.MaxBytes))
	}
	if opts.L2Client != nil {
		store = NewL2Storage(opts.L2Client, lru, m, opts.L1TTL, opts.IsNoL2Err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	spec := &cacheSpec{
		maxBytes:   opts.MaxBytes,
		listenAddr: opts.ListenAddr,
		client:     skpnet.NewClient(opts.NetOpts),
		storage:    store,
		lruStorage: lru,
		metrics:    m,
		revalJobs:  make(chan revalJob, revalQueueSize),
		ctx:        ctx,
		cancel:     cancel,
		filters:    make(map[filterCacheKey]*cacheFilter),
	}

	// Start shared background goroutines (one worker + one scraper for all filter instances)
	spec.bgWg.Add(2)
	go spec.revalidationWorker()
	go spec.updateMetrics()

	return spec
}

func (s *cacheSpec) Name() string { return filterName }

// Close shuts down the background revalidation worker and metrics scraper.
// Safe to call multiple times.
func (s *cacheSpec) Close() error {
	s.closeOnce.Do(func() {
		s.cancel() // signals both goroutines to stop via ctx.Done()
		s.bgWg.Wait()
		s.client.Close() // tear down transport after all in-flight revalidation fetches complete
	})
	return nil
}

func (s *cacheSpec) CreateFilter(args []any) (filters.Filter, error) {
	// Valid arg counts:
	//   0, 1           → RFC mode: cache() or cache(keyHeaders)
	//   3, 4, 5        → force mode: ttl, errorTTL, swrWindow[, staleIfError[, keyHeaders]]
	var (
		rfcMode                          bool
		ttl, errorTTL, swr, staleIfError time.Duration
		keyHeaders                       []string
		err                              error
	)
	switch {
	case len(args) <= 1:
		rfcMode = true
		keyHeaders, err = parseRFCModeArgs(args)
	case len(args) >= 3 && len(args) <= 5:
		var fc forceModeConfig
		fc, err = parseForceModeArgs(args)
		ttl, errorTTL, swr, staleIfError, keyHeaders = fc.ttl, fc.errorTTL, fc.swr, fc.staleIfError, fc.keyHeaders
	default:
		err = fmt.Errorf("cache: expected 0, 1, or 3-5 args (RFC: [keyHeaders]; force: ttl, errorTTL, swrWindow[, staleIfError[, keyHeaders]]), got %d: %w", len(args), filters.ErrInvalidFilterParameters)
	}
	if err != nil {
		return nil, err
	}

	sort.Strings(keyHeaders) // canonical order for registry key
	fk := filterCacheKey{
		ttl:          ttl,
		errorTTL:     errorTTL,
		swrWindow:    swr,
		staleIfError: staleIfError,
		keyHeaders:   strings.Join(keyHeaders, ","),
		rfcMode:      rfcMode,
	}

	s.muFilter.Lock()
	defer s.muFilter.Unlock()
	if cf, ok := s.filters[fk]; ok {
		return cf, nil
	}

	cf := &cacheFilter{
		storage:      s.storage,
		lruStorage:   s.lruStorage,
		listenAddr:   s.listenAddr,
		ttl:          ttl,
		errorTTL:     errorTTL,
		swrWindow:    swr,
		staleIfError: staleIfError,
		rfcMode:      rfcMode,
		metrics:      s.metrics,
		keyHeaders:   keyHeaders,
		revalJobs:    s.revalJobs, // use spec-level shared channel
		ctx:          s.ctx,       // cancelled when spec shuts down
	}

	cf.fetch = s.client.Do
	s.filters[fk] = cf
	return cf, nil
}

// parseRFCModeArgs handles cache() and cache(keyHeaders) — 0 or 1 argument.
// RFC mode has no other configurable knobs: upstream Cache-Control is
// authoritative.
func parseRFCModeArgs(args []any) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	raw, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("cache: arg 0 (keyHeaders): expected string, got %T: %w", args[0], filters.ErrInvalidFilterParameters)
	}
	return canonicalKeyHeaders(raw), nil
}

// forceModeConfig holds the parsed arguments for force mode.
type forceModeConfig struct {
	ttl, errorTTL, swr, staleIfError time.Duration
	keyHeaders                       []string
}

// parseForceModeArgs handles cache(ttl, errorTTL, swrWindow[, staleIfError[, keyHeaders]])
// — 3 to 5 arguments. Force mode: operator-supplied values are authoritative;
// upstream Cache-Control freshness directives are ignored.
func parseForceModeArgs(args []any) (c forceModeConfig, err error) {
	c.ttl, err = toDuration(args[0])
	if err != nil {
		return forceModeConfig{}, fmt.Errorf("cache: arg 0 (ttl): %v: %w", err, filters.ErrInvalidFilterParameters)
	}

	c.errorTTL, err = toDuration(args[1])
	if err != nil {
		return forceModeConfig{}, fmt.Errorf("cache: arg 1 (errorTTL): %v: %w", err, filters.ErrInvalidFilterParameters)
	}

	c.swr, err = toDuration(args[2])
	if err != nil {
		return forceModeConfig{}, fmt.Errorf("cache: arg 2 (swrWindow): %v: %w", err, filters.ErrInvalidFilterParameters)
	}

	if len(args) >= 4 {
		raw, ok := args[3].(string)
		if !ok {
			return forceModeConfig{}, fmt.Errorf("cache: arg 3 (staleIfError): expected string, got %T: %w", args[3], filters.ErrInvalidFilterParameters)
		}
		c.staleIfError, err = time.ParseDuration(raw)
		if err != nil {
			return forceModeConfig{}, fmt.Errorf("cache: arg 3 (staleIfError): %v: %w", err, filters.ErrInvalidFilterParameters)
		}
	}

	if len(args) == 5 {
		raw, ok := args[4].(string)
		if !ok {
			return forceModeConfig{}, fmt.Errorf("cache: arg 4 (keyHeaders): expected string, got %T: %w", args[4], filters.ErrInvalidFilterParameters)
		}
		c.keyHeaders = canonicalKeyHeaders(raw)
	}

	return c, nil
}

// canonicalKeyHeaders splits a comma-separated header-name list into
// canonical form. Shared by RFC-mode (1-arg) and force-mode (5-arg) keyHeaders
// parsing — the only part of arg parsing that's genuinely common to both modes.
func canonicalKeyHeaders(raw string) []string {
	var keyHeaders []string
	for h := range strings.SplitSeq(raw, ",") {
		if name := strings.TrimSpace(h); name != "" {
			keyHeaders = append(keyHeaders, http.CanonicalHeaderKey(name))
		}
	}
	return keyHeaders
}

// revalidationWorker is the single background goroutine (spec-level, shared across
// all filter instances) that processes revalidation jobs sequentially. It calls
// the per-instance doRevalidate method to respect each route's configuration.
func (s *cacheSpec) revalidationWorker() {
	defer s.bgWg.Done()
	for {
		select {
		case job := <-s.revalJobs:
			if job.filter != nil {
				s.metrics.MeasureSince("cache.reval_wait_duration", job.enqueuedAt)
				start := time.Now()
				job.filter.doRevalidateQueued(job)
				s.metrics.MeasureSince("cache.reval_duration", start)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

const lruBytesScrapeInterval = 10 * time.Second

// updateMetrics periodically updates the lru_bytes gauge so it stays current
// even when no evictions occur. It's spec-level and shared across all filter instances.
func (s *cacheSpec) updateMetrics() {
	defer s.bgWg.Done()
	ticker := time.NewTicker(lruBytesScrapeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.metrics.UpdateGauge("cache.lru_bytes", float64(s.lruStorage.lru.Bytes()))
			s.metrics.UpdateGauge("cache.reval_queue_depth", float64(len(s.revalJobs)))
		case <-s.ctx.Done():
			return
		}
	}
}

type revalJob struct {
	key          string
	baseKey      string
	req          *http.Request // cloned via Request.Clone
	backendURL   string        // non-empty: dial here directly instead of looping through listenAddr
	outgoingHost string        // Host header to send with backendURL; see requestDispatch
	filter       *cacheFilter  // instance whose doRevalidate to call
	route        string        // route for skipper_cache_route_reval_total
	enqueuedAt   time.Time     // wall-clock time the job entered the queue; used to measure wait time
}

// cacheFilter does not implement filters.FilterCloser. The routing layer calls
// Close() on every FilterCloser when a route is invalidated; because all
// goroutines and shared resources are owned by cacheSpec, closing them here
// would be destructive. Lifecycle is managed exclusively by cacheSpec.Close().
type cacheFilter struct {
	storage      Storage
	lruStorage   *LRUStorage // always non-nil; direct reference to L1, even when storage is L2Storage
	listenAddr   string
	ttl          time.Duration
	errorTTL     time.Duration
	swrWindow    time.Duration
	staleIfError time.Duration
	keyHeaders   []string // request headers folded into the base cache key

	// rfcMode true: upstream Cache-Control is authoritative (cache()).
	// false: operator ttl/errorTTL/swrWindow are authoritative (force mode).
	rfcMode   bool
	coldSF    singleflight.Group // cold-miss coalescing
	revalSF   singleflight.Group // coalesces concurrent background revalidations per key
	revalJobs chan revalJob      // shared background revalidation queue from cacheSpec
	ctx       context.Context    // cancelled when cacheSpec shuts down
	fetch     func(*http.Request) (*http.Response, error)
	metrics   metrics.Metrics
}

// tagSpan sets cache_status and (when >= 0) cache_ttl_remaining_ms
// on the active OpenTracing span. No-op when no span is present.
func tagSpan(ctx filters.FilterContext, status string, ttlRemainingMs int64) {
	span := opentracing.SpanFromContext(ctx.Request().Context())
	if span == nil {
		return
	}
	span.SetTag("cache_status", status)
	if ttlRemainingMs >= 0 {
		span.SetTag("cache_ttl_remaining_ms", ttlRemainingMs)
	}
}

// updateLRUBytesGauge records the current per-route L1 (LRU) footprint. It
// reads f.lruStorage directly rather than f.storage, since f.storage may be
// an *L2Storage wrapping L1 — f.lruStorage is always a direct, non-nil
// reference to L1 regardless of whether L2 is configured for this route.
// Called once per request outcome (hit/miss/stale) so the gauge stays
// accurate under any traffic mix, not just hit-heavy routes.
func (f *cacheFilter) updateLRUBytesGauge(ctx filters.FilterContext) {
	bytes, entries := f.lruStorage.lru.Usage()
	if p, ok := f.metrics.(interface{ SetCacheLRUBytes(string, float64) }); ok {
		p.SetCacheLRUBytes(ctx.RouteId(), float64(bytes))
	}
	if p, ok := f.metrics.(interface{ SetCacheLRUEntries(string, float64) }); ok {
		p.SetCacheLRUEntries(ctx.RouteId(), float64(entries))
	}
}

func routeLabel(routeID string) string {
	if routeID == "" {
		return "unknown"
	}
	return routeID
}

func (f *cacheFilter) incRouteResult(route, result string) {
	if p, ok := f.metrics.(interface{ IncCacheRouteResult(string, string) }); ok {
		p.IncCacheRouteResult(route, result)
	}
}

// incRouteReval records a background refresh outcome: started, success, failed or dropped.
func (f *cacheFilter) incRouteReval(route, outcome string) {
	if p, ok := f.metrics.(interface{ IncCacheRouteReval(string, string) }); ok {
		p.IncCacheRouteReval(route, outcome)
	}
}

// observeOriginFetch records an origin fetch duration. resp is nil when err is set.
func (f *cacheFilter) observeOriginFetch(route string, direct bool, start time.Time, resp *http.Response, err error) {
	dispatch := originDispatchPipeline
	if direct {
		dispatch = originDispatchDirect
	}
	if p, ok := f.metrics.(interface {
		ObserveCacheOriginDuration(string, string, string, float64)
	}); ok {
		p.ObserveCacheOriginDuration(route, originStatusClass(resp, err), dispatch, time.Since(start).Seconds())
	}
}

// originStatusClass maps an origin fetch result to one of 2xx, 3xx, 4xx, 5xx, error.
// Status codes outside 200-599 map to "other".
func originStatusClass(resp *http.Response, err error) string {
	if err != nil || resp == nil {
		return "error"
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return "2xx"
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return "3xx"
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return "4xx"
	case resp.StatusCode >= 500 && resp.StatusCode < 600:
		return "5xx"
	default:
		return "other"
	}
}

// recordCacheControl counts each Cache-Control directive present on an origin
// response once. Directive values are dropped and unknown names map to "other",
// so the label set is fixed.
func (f *cacheFilter) recordCacheControl(h http.Header) {
	p, ok := f.metrics.(interface{ IncOriginCacheControl(string) })
	if !ok {
		return
	}
	var seen [len(cacheControlLabels)]bool
	for _, v := range h.Values("Cache-Control") {
		for _, d := range strings.Split(v, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(d), "=")
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			i := cacheControlLabelIndex(name)
			if seen[i] {
				continue
			}
			seen[i] = true
			p.IncOriginCacheControl(cacheControlLabels[i])
		}
	}
}

// cacheControlLabels is the fixed label set for skipper_cache_origin_cache_control_total.
// The last entry is the catch-all for unknown directives.
var cacheControlLabels = [9]string{
	"public", "private", "no-store", "no-cache", "max-age",
	"s-maxage", "stale-while-revalidate", "stale-if-error", "other",
}

func cacheControlLabelIndex(name string) int {
	for i, l := range cacheControlLabels[:len(cacheControlLabels)-1] {
		if name == l {
			return i
		}
	}
	return len(cacheControlLabels) - 1
}

// Request checks the cache. On a hit it calls ctx.Serve() to short-circuit the
// backend roundtrip. On a stale hit it serves stale and fires a background
// revalidation. On a miss it stores the computed key in the state bag so
// Response() can populate the cache.
func (f *cacheFilter) Request(ctx filters.FilterContext) {
	// Key is built from the already-filtered request: mutations by earlier filters
	// (e.g. header stripping by auth) are intentionally reflected in the cache key.
	baseKey := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
	key := baseKey
	// GetSilent: this is an internal plumbing lookup, not a real cache decision —
	// it must not count toward hit/l1_hit/l2_hit/l2_miss/l2_get_error on every request.
	if sentinel, _ := f.storage.GetSilent(ctx.Request().Context(), "vary:"+baseKey); sentinel != nil && len(sentinel.VaryHeaders) > 0 {
		key = varyKey(baseKey, ctx.Request(), sentinel.VaryHeaders)
	}
	ctx.StateBag()[stateBagKey] = key
	ctx.StateBag()[stateBagBaseKey] = baseKey
	ctx.StateBag()[stateBagRequestTime] = time.Now()

	// Unsafe methods skip the cache lookup; Response() will invalidate using the key above.
	if isUnsafeMethod(ctx.Request().Method) {
		return
	}

	// Revalidation requests bypass the cache lookup so Response() can store a
	// fresh entry. Strip the header so it never reaches the upstream.
	if ctx.Request().Header.Get(revalidateHeader) != "" {
		ctx.Request().Header.Del(revalidateHeader)
		return
	}

	reqDir := parseRequestCacheControl(ctx.Request().Header)
	var entry *Entry
	var err error
	if reqDir.onlyIfCached {
		entry, err = f.storage.Get(ctx.Request().Context(), key)
		if err != nil || entry == nil || !entry.IsUsable(time.Now()) {
			ctx.Serve(&http.Response{
				StatusCode: http.StatusGatewayTimeout,
				Header:     http.Header{cacheStatusHeader: {cacheStatusMiss}},
				Body:       http.NoBody,
			})
			return
		}
	} else if reqDir.noCache || reqDir.noStore {
		if reqDir.noStore {
			ctx.StateBag()[stateBagNoStore] = true
		}
		return
	} else {
		if ctx.Request().Context().Err() != nil {
			return
		}
		entry, err = f.storage.Get(ctx.Request().Context(), key)
	}
	if err != nil || entry == nil {
		f.coalesce(ctx, key)
		return
	}

	rsp := &http.Response{
		StatusCode: entry.StatusCode,
		Header:     entry.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(entry.Payload)),
	}

	now := time.Now()
	pastTTL := now.After(entry.CreatedAt.Add(entry.TTL))
	if entry.IsStale(now) {
		d := parseCacheControl(entry.Header)
		if d.mustRevalidate || d.proxyRevalidate || d.noCache || d.sMaxAge >= 0 {
			f.coalesce(ctx, key)
			return
		}
		// RFC 9111 §5.2.1 max-stale.
		if reqDir.maxStale >= 0 {
			staleness := time.Since(entry.CreatedAt) - entry.TTL
			if staleness > time.Duration(reqDir.maxStale)*time.Second {
				f.coalesce(ctx, key)
				return
			}
		}
		rsp.Header.Set(cacheStatusHeader, cacheStatusStale)
		tagSpan(ctx, cacheStatusStale, -1)
		setAgeHeader(rsp, entry, now)
		f.metrics.IncCounter("cache.stale")
		f.incRouteResult(routeLabel(ctx.RouteId()), "stale")
		if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
			p.IncCacheResult("stale", statusCodeClass(entry.StatusCode))
		}
		f.updateLRUBytesGauge(ctx)
		method := ctx.Request().Method
		if isCacheableMethod(method) && evaluateConditionals(ctx.Request(), entry) {
			notModified := &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     rsp.Header.Clone(),
				Body:       http.NoBody,
				Request:    ctx.Request(), // link response to originating request per net/http convention
			}
			ctx.Serve(notModified)
			revalReq, backendURL, outgoingHost := f.requestDispatch(ctx, "cache.reval_error")
			f.enqueueRevalidation(key, revalReq, backendURL, outgoingHost, routeLabel(ctx.RouteId()), baseKey)
			return
		}
		ctx.Serve(headBodyOmitted(method, rsp))
		revalReq, backendURL, outgoingHost := f.requestDispatch(ctx, "cache.reval_error")
		f.enqueueRevalidation(key, revalReq, backendURL, outgoingHost, routeLabel(ctx.RouteId()), baseKey)
		return
	}

	// Entry is past TTL+SWR but still in storage (kept alive by stale-if-error window).
	// Treat as a miss so the upstream is hit; coalesce applies stale-if-error.
	if pastTTL {
		f.coalesce(ctx, key)
		return
	}

	// RFC 9111 §5.2.1 min-fresh.
	if reqDir.minFresh >= 0 {
		elapsed := time.Since(entry.CreatedAt)
		remaining := entry.TTL - elapsed
		if remaining < time.Duration(reqDir.minFresh)*time.Second {
			f.coalesce(ctx, key)
			return
		}
	}

	rsp.Header.Set(cacheStatusHeader, cacheStatusHit)
	tagSpan(ctx, cacheStatusHit, max(0, entry.TTL-time.Since(entry.CreatedAt)).Milliseconds())
	setAgeHeader(rsp, entry, time.Now())
	f.metrics.IncCounter("cache.hit")
	f.incRouteResult(routeLabel(ctx.RouteId()), "hit")
	if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
		p.IncCacheResult("hit", statusCodeClass(entry.StatusCode))
	}
	f.updateLRUBytesGauge(ctx)
	method := ctx.Request().Method
	if isCacheableMethod(method) && evaluateConditionals(ctx.Request(), entry) {
		notModified := &http.Response{
			StatusCode: http.StatusNotModified,
			Header:     rsp.Header.Clone(),
			Body:       http.NoBody,
			Request:    ctx.Request(), // link response to originating request per net/http convention
		}
		ctx.Serve(notModified)
		return
	}
	ctx.Serve(headBodyOmitted(method, rsp))
}

// coalesceResult carries the fetched entry, a stale-if-error snapshot taken
// before f.fetch() (so a 5xx result can't overwrite it first), and what's
// needed to decide whether a follower may reuse the leader's entry.
type coalesceResult struct {
	entry     *Entry
	stored    *Entry        // snapshot before fetch; nil if no eligible stale-if-error entry existed
	storeable bool          // storeDecision's verdict for entry; also gates sharing with non-leader waiters
	varyNames []string      // Vary header names from entry's response, for in-memory follower matching
	leaderReq *http.Request // the exact *http.Request that was actually fetched
}

// Checks if this response can be persisted, or handed to a party
// that isn't the one who fetched it - a coalesce follower, or
// background revalidation triggered by someone else's request
func (f *cacheFilter) storeDecision(req *http.Request, statusCode int, header http.Header, d cacheDirectives) (ttl time.Duration, store bool) {
	// A 304 has no representation body. Loopback revalidation must preserve
	// the stored entry until the worker merges its conditional response.
	if statusCode == http.StatusNotModified {
		return 0, false
	}
	// Vary: * means every response is unique — never cache. Response() already
	// short-circuits on this before calling storeDecision; coalesce()'s leader-fetch
	// path has no such pre-check, so it belongs here too, in the one choke point both
	// callers share.
	if slices.Contains(parseVaryNames(header.Get("Vary")), "*") {
		return 0, false
	}
	// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
	if f.rfcMode && (d.noStore || d.private) {
		return 0, false
	}
	ttl, store = f.resolveTTL(statusCode, header, d)
	if !store {
		return 0, false
	}
	// RFC 9111 §3.5, unconditional (not mode-gated) — unless Authorization is
	// already folded into the key (keyHeaders), in which case every request on
	// this key already shares the same credential.
	if req.Header.Get("Authorization") != "" && !slices.Contains(f.keyHeaders, "Authorization") && !d.public && !d.mustRevalidate {
		return 0, false
	}
	return ttl, true
}

// coalesce gates concurrent cold misses for the same key behind a single upstream
// fetch, preventing thundering herd within this process. A follower only reuses
// the leader's response when storeDecision and Vary allow it (see storeDecision).
// Note: coalescing is process-local — a fleet of N Skipper instances may still issue
// up to N simultaneous origin requests for the same cold miss.
// Stale-if-error (RFC 5861 §4) is also applied here: a pre-fetch snapshot of any
// eligible stored entry is served on transport errors or HTTP 500/502/503/504.
func (f *cacheFilter) coalesce(ctx filters.FilterContext, key string) {
	source, backendURL, outgoingHost := f.requestDispatch(ctx, "cache.coalesce_error")
	req, err := f.prepareDispatch(source, backendURL, outgoingHost)
	if err != nil {
		f.metrics.IncCounter("cache.coalesce_error")
		log.WithError(err).Warn("cache: failed to prepare cold-miss dispatch")
		return
	}

	waitStart := time.Now()
	route := routeLabel(ctx.RouteId())
	ch := f.coldSF.DoChan(key, func() (any, error) {
		// Capture any existing stale-if-error eligible entry before fetching, so that a
		// subsequent 5xx response cannot overwrite it in storage before we read it.
		sieStored := f.staleIfErrorEntry(key)

		requestTime := time.Now()
		resp, err := f.fetch(req)
		f.observeOriginFetch(route, backendURL != "", requestTime, resp, err)
		if err != nil {
			if sieStored != nil {
				return &coalesceResult{stored: sieStored}, err
			}
			return nil, err
		}
		responseTime := time.Now()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if sieStored != nil {
				return &coalesceResult{stored: sieStored}, err
			}
			return nil, err
		}

		f.recordCacheControl(resp.Header)
		directives := parseCacheControl(resp.Header)
		ttl, shouldStore := f.storeDecision(req, resp.StatusCode, resp.Header, directives)
		swr := f.resolveSWR(resp.StatusCode, directives)
		cia := correctedInitialAge(requestTime, responseTime, resp.Header)
		coalescedHeader := resp.Header.Clone()
		stripHopByHop(coalescedHeader)
		entry := &Entry{
			StatusCode:           resp.StatusCode,
			Header:               coalescedHeader,
			Payload:              body,
			CreatedAt:            responseTime,
			TTL:                  ttl,
			StaleWhileRevalidate: swr,
			StaleIfError:         f.staleIfError,
			ETag:                 resp.Header.Get("ETag"),
			LastModified:         resp.Header.Get("Last-Modified"),
			CorrectedInitialAge:  cia,
			ResponseTime:         responseTime,
		}
		if shouldStore && !(f.canServeStaleIfError(sieStored) && staleIfErrorStatus(resp.StatusCode)) {
			baseKey, _ := ctx.StateBag()[stateBagBaseKey].(string)
			if baseKey == "" {
				baseKey = key
			}
			if err := f.storeEntry(context.Background(), baseKey, key, req, entry); err != nil {
				log.WithError(err).Warn("cache: Set failed (cold-miss store)")
			}
		}
		return &coalesceResult{
			entry:     entry,
			stored:    sieStored,
			storeable: shouldStore,
			varyNames: parseVaryNames(resp.Header.Get("Vary")),
			leaderReq: req,
		}, nil
	})

	select {
	case res := <-ch:
		f.metrics.MeasureSince("cache.coalesce_wait_duration", waitStart)
		if res.Err != nil || res.Val == nil {
			f.metrics.IncCounter("cache.coalesce_error")
			if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
				p.IncCacheResult("coalesce_error", "n/a")
			}
			if res.Val == nil {
				return
			}
		}
		cr := res.Val.(*coalesceResult)
		entry := cr.entry

		// RFC 5861 §4: preserve the last good entry on eligible failures.
		// This check lives here (not in Response()) because coalesce always calls
		// ctx.Serve(), which sets ctx.Served()=true and causes Response() to return early.
		// cr.stored was already vetted by storeDecision, so any waiter may share it.
		if f.canServeStaleIfError(cr.stored) && (res.Err != nil || staleIfErrorStatus(entry.StatusCode)) {
			staleRsp := &http.Response{
				StatusCode: cr.stored.StatusCode,
				Header:     cr.stored.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(cr.stored.Payload)),
			}
			staleRsp.Header.Set(cacheStatusHeader, cacheStatusStale)
			tagSpan(ctx, cacheStatusStale, -1)
			setAgeHeader(staleRsp, cr.stored, time.Now())
			f.metrics.IncCounter("cache.stale")
			f.incRouteResult(routeLabel(ctx.RouteId()), "stale")
			if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
				p.IncCacheResult("stale", statusCodeClass(cr.stored.StatusCode))
			}
			ctx.Serve(headBodyOmitted(ctx.Request().Method, staleRsp))
			return
		}
		if res.Err != nil {
			return
		}

		// req is this goroutine's clone; cr.leaderReq is whichever clone won DoChan.
		// A follower reuses the leader's entry only if it's storeable, or its
		// identity already matches via keyHeaders (same key => same identity) or
		// Vary — scoped to this burst only, unlike storeDecision's stricter
		// persistence check, which future requests can read back. Otherwise: fetch
		// independently.
		sameVaryDimension := len(cr.varyNames) > 0 && varyKey("", req, cr.varyNames) == varyKey("", cr.leaderReq, cr.varyNames)
		varyMatches := len(cr.varyNames) == 0 || sameVaryDimension
		identityMatchesLeader := !slices.Contains(cr.varyNames, "*") && varyMatches && (len(f.keyHeaders) > 0 || sameVaryDimension)
		if req != cr.leaderReq && (!varyMatches || !cr.storeable && !identityMatchesLeader) {
			f.metrics.IncCounter("cache.coalesce_bypass")
			if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
				p.IncCacheResult("coalesce_bypass", "n/a")
			}
			if cr.storeable {
				baseKey, _ := ctx.StateBag()[stateBagBaseKey].(string)
				if baseKey == "" {
					baseKey = key
				}
				f.coalesce(ctx, varyKey(baseKey, req, cr.varyNames))
				return
			}
			resp, err := f.fetch(req)
			if err != nil {
				f.metrics.IncCounter("cache.coalesce_bypass_error")
				if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("coalesce_bypass_error", "n/a")
				}
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				f.metrics.IncCounter("cache.coalesce_bypass_error")
				if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
					p.IncCacheResult("coalesce_bypass_error", "n/a")
				}
				return
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			ctx.Serve(headBodyOmitted(ctx.Request().Method, resp))
			return
		}

		rsp := &http.Response{
			StatusCode: entry.StatusCode,
			Header:     entry.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(entry.Payload)),
		}
		rsp.Header.Set(cacheStatusHeader, cacheStatusMiss)
		tagSpan(ctx, cacheStatusMiss, -1)
		f.metrics.IncCounter("cache.miss")
		f.incRouteResult(routeLabel(ctx.RouteId()), "miss")
		if p, ok := f.metrics.(interface{ IncCacheResult(string, string) }); ok {
			p.IncCacheResult("miss", statusCodeClass(entry.StatusCode))
		}
		f.updateLRUBytesGauge(ctx)
		ctx.Serve(headBodyOmitted(ctx.Request().Method, rsp))
	case <-ctx.Request().Context().Done():
		// Client disconnected. Remaining waiters still receive their result.
	}
}

func staleIfErrorStatus(status int) bool {
	return status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func (f *cacheFilter) staleIfErrorEntry(key string) *Entry {
	if f.staleIfError <= 0 {
		return nil
	}
	entry, err := f.storage.GetSilent(context.Background(), key)
	if err != nil || !f.canServeStaleIfError(entry) {
		return nil
	}
	return entry
}

func (f *cacheFilter) canServeStaleIfError(entry *Entry) bool {
	return f.staleIfError > 0 && entry != nil && entry.StatusCode < 500 &&
		time.Since(entry.CreatedAt)-entry.TTL <= f.staleIfError
}

// Response stores the upstream response in the cache, freshens HEAD entries,
// and invalidates on unsafe methods. Stale-if-error is handled in coalesce,
// not here, because coalesce calls ctx.Serve() which causes Response to return early.
func (f *cacheFilter) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()
	key, _ := ctx.StateBag()[stateBagKey].(string)
	if key == "" {
		return
	}

	// RFC 9111 §4.3.5: HEAD 200 freshens the stored GET entry's headers.
	// This block runs before ctx.Served() so freshening happens even when
	// Request() already served the HEAD from cache.
	if ctx.Request().Method == http.MethodHead && rsp.StatusCode == http.StatusOK {
		if stored, err := f.storage.Get(ctx.Request().Context(), key); err == nil && stored != nil {
			for k, vv := range rsp.Header {
				if bodyRelatedHeaders[k] {
					continue
				}
				stored.Header[k] = vv
			}
			if etag := rsp.Header.Get("ETag"); etag != "" {
				stored.ETag = etag
			}
			if lm := rsp.Header.Get("Last-Modified"); lm != "" {
				stored.LastModified = lm
			}
			if err := f.storage.Set(ctx.Request().Context(), key, stored); err != nil {
				log.WithError(err).Warn("cache: Set failed (HEAD freshen)")
			}
		}
		return
	}

	// Hit or stale path: already served from cache in Request(); nothing to do.
	if ctx.Served() {
		return
	}

	// RFC 9111 §4.4: invalidate cached entry on successful unsafe method.
	// Also invalidate same-origin Location/Content-Location URIs.
	if isUnsafeMethod(ctx.Request().Method) && rsp.StatusCode < 400 {
		if err := f.storage.Delete(ctx.Request().Context(), key); err != nil {
			log.WithError(err).Warn("cache: Delete failed (unsafe method invalidation)")
		}
		if err := f.storage.Delete(ctx.Request().Context(), "vary:"+key); err != nil {
			log.WithError(err).Warn("cache: Delete failed (vary sentinel invalidation)")
		}
		for _, hdrName := range []string{"Location", "Content-Location"} {
			if loc := rsp.Header.Get(hdrName); loc != "" && sameOrigin(ctx.Request(), loc) {
				if locKey := cacheKeyForURL(ctx.RouteId(), ctx.Request(), loc, f.keyHeaders); locKey != "" {
					if err := f.storage.Delete(ctx.Request().Context(), locKey); err != nil {
						log.WithError(err).Warn("cache: Delete failed (Location invalidation)")
					}
					if err := f.storage.Delete(ctx.Request().Context(), "vary:"+locKey); err != nil {
						log.WithError(err).Warn("cache: Delete failed (vary sentinel for Location)")
					}
				}
			}
		}
		return
	}

	if ctx.StateBag()[stateBagNoStore] == true {
		rsp.Header.Set(cacheStatusHeader, cacheStatusMiss)
		tagSpan(ctx, cacheStatusMiss, -1)
		f.metrics.IncCounter("cache.miss")
		f.incRouteResult(routeLabel(ctx.RouteId()), "miss")
		return
	}

	rsp.Header.Set(cacheStatusHeader, cacheStatusMiss)
	tagSpan(ctx, cacheStatusMiss, -1)
	f.metrics.IncCounter("cache.miss")
	f.incRouteResult(routeLabel(ctx.RouteId()), "miss")

	// Vary: * means every response is unique — never cache.
	varyHeader := rsp.Header.Get("Vary")
	if varyHeader == "*" {
		return
	}

	f.recordCacheControl(rsp.Header)
	directives := parseCacheControl(rsp.Header)
	ttl, shouldStore := f.storeDecision(ctx.Request(), rsp.StatusCode, rsp.Header, directives)
	if !shouldStore {
		return
	}
	swr := f.resolveSWR(rsp.StatusCode, directives)

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"method": ctx.Request().Method,
			"url":    ctx.Request().URL.String(),
		}).WithError(err).Warn("cache: failed to read response body, skipping cache storage")
		return
	}

	rsp.Body = io.NopCloser(bytes.NewReader(body))

	baseKey, _ := ctx.StateBag()[stateBagBaseKey].(string)
	if baseKey == "" {
		baseKey = key
	}
	responseTime := time.Now()
	var requestTime time.Time
	if rt, ok := ctx.StateBag()[stateBagRequestTime].(time.Time); ok {
		requestTime = rt
	} else {
		requestTime = responseTime
	}
	cia := correctedInitialAge(requestTime, responseTime, rsp.Header)
	storedHeader := rsp.Header.Clone()
	stripHopByHop(storedHeader)
	entry := &Entry{
		StatusCode:           rsp.StatusCode,
		Header:               storedHeader,
		Payload:              body,
		CreatedAt:            responseTime,
		TTL:                  ttl,
		StaleWhileRevalidate: swr,
		StaleIfError:         f.staleIfError,
		ETag:                 rsp.Header.Get("ETag"),
		LastModified:         rsp.Header.Get("Last-Modified"),
		CorrectedInitialAge:  cia,
		ResponseTime:         responseTime,
	}
	if err := f.storeEntry(ctx.Request().Context(), baseKey, key, ctx.Request(), entry); err != nil {
		log.WithError(err).Warn("cache: Set failed (response store)")
	}
}

func (f *cacheFilter) storeEntry(ctx context.Context, baseKey, key string, req *http.Request, entry *Entry) error {
	varyNames := parseVaryNames(entry.Header.Get("Vary"))
	entry.VaryHeaders = varyNames
	if len(varyNames) > 0 {
		key = varyKey(baseKey, req, varyNames)
		sentinel := &Entry{
			CreatedAt:            entry.CreatedAt,
			TTL:                  entry.TTL,
			StaleWhileRevalidate: entry.StaleWhileRevalidate,
			StaleIfError:         entry.StaleIfError,
			VaryHeaders:          varyNames,
		}
		if err := f.storage.Set(ctx, "vary:"+baseKey, sentinel); err != nil {
			log.WithError(err).Warn("cache: Set failed (vary sentinel)")
		}
	}
	return f.storage.Set(ctx, key, entry)
}

// requestDispatch decides how cold misses and background revalidation reach the
// origin: directly at the route's resolved backend when safe (force mode,
// static network backend — ctx.BackendUrl() non-empty), or via self-loopback
// through skipper's own listener otherwise (RFC mode, or a load-balanced /
// dynamic backend with no single resolvable URL — BackendUrl() is empty for
// both per its own doc comment).
//
// The two branches deliberately use different request objects. Direct
// dispatch uses ctx.Request() — already transformed by earlier filters
// (modPath, setRequestHeader, ...) into exactly what should be sent to the
// backend. Using the pre-filter request here would be wrong: it would send
// the client-facing path/host straight to the backend, bypassing whatever
// those filters did. The self-loopback fallback still needs revalidationRequest
// (ctx.OriginalRequest(), falling back to ctx.Request()) because it has to
// re-match a route, which requires the pre-mutation path.
//
// outgoingHost is the Host header to send with the direct-dispatch request —
// ctx.OutgoingHost() (the same value the main proxy pipeline would use for this
// route), not the client-facing Host that ctx.Request() still carries. It is
// empty for self-loopback, which must keep the client-facing Host to re-match
// a route on re-entry.
//
// Note: RFC mode always uses self-loopback, never direct dispatch, even when
// ctx.BackendUrl() is non-empty. A Response()-filter positioned after cache()
// could rewrite Cache-Control; only the self-loopback path re-runs the full
// filter chain and sees that rewritten value before doRevalidate reads it for
// TTL and stale-while-revalidate purposes (resolveTTL, resolveSWR). Direct
// dispatch would read the pre-rewrite upstream headers instead.
func (f *cacheFilter) requestDispatch(ctx filters.FilterContext, errorMetric string) (req *http.Request, backendURL string, outgoingHost string) {
	if !f.rfcMode {
		if b := ctx.BackendUrl(); b != "" {
			if _, err := parseBackendURL(b); err == nil {
				return ctx.Request(), b, ctx.OutgoingHost()
			} else {
				f.metrics.IncCounter(errorMetric)
				log.WithField("backendURL", b).WithError(err).Warn("cache: invalid backend URL, using self-loopback")
			}
		}
	}
	return revalidationRequest(ctx), "", ""
}

func parseBackendURL(backendURL string) (*url.URL, error) {
	u, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("cache: backend must be an absolute HTTP(S) URL with a host: %q", backendURL)
	}
	return u, nil
}

// prepareDispatch leaves the inbound request untouched. Loopback must replay
// the original request; direct dispatch must retain earlier filter mutations.
func (f *cacheFilter) prepareDispatch(source *http.Request, backendURL, outgoingHost string) (*http.Request, error) {
	req := source.Clone(context.Background())
	if backendURL != "" {
		u, err := parseBackendURL(backendURL)
		if err != nil {
			return nil, err
		}
		req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
		req.Host = outgoingHost
	} else {
		req.URL.Scheme, req.URL.Host = "http", f.listenAddr
		req.Header.Set(revalidateHeader, "1")
	}
	req.RequestURI = ""
	return req, nil
}

// revalidationRequest returns the request to replay for background revalidation.
// It must be ctx.OriginalRequest(), not ctx.Request(): by the time this filter's
// Request() runs, earlier filters (e.g. modPath stripping a path prefix before
// forwarding) may have already mutated ctx.Request() in place. doRevalidate loops
// the request back through skipper's own listener so the full filter chain reruns
// on it; replaying the already-mutated request can no longer match the route that
// produced that mutation, so revalidation permanently fails with a routing error
// and a stale entry (e.g. a cached error) never gets refreshed.
// ctx.OriginalRequest() can be nil per its interface contract, so fall back to
// ctx.Request() rather than passing nil into enqueueRevalidation.
func revalidationRequest(ctx filters.FilterContext) *http.Request {
	if orig := ctx.OriginalRequest(); orig != nil {
		return orig
	}
	return ctx.Request()
}

// enqueueRevalidation sends a revalidation job to the background worker.
// The request is cloned in the calling goroutine before orig is released.
// backendURL and outgoingHost, when non-empty, tell doRevalidate to dispatch
// directly to that backend using that Host header instead of looping back
// through skipper's own listener (see requestDispatch).
// If the queue is full the job is dropped and reval_dropped is incremented.
// route labels skipper_cache_route_reval_total for this job.
// The closure captures f.doRevalidate so the spec-level worker respects this route's config.
func (f *cacheFilter) enqueueRevalidation(key string, orig *http.Request, backendURL, outgoingHost, route string, baseKeys ...string) {
	baseKey := key
	if len(baseKeys) > 0 {
		baseKey = baseKeys[0]
	}
	cloned := orig.Clone(context.Background())
	job := revalJob{
		key:          key,
		baseKey:      baseKey,
		req:          cloned,
		backendURL:   backendURL,
		outgoingHost: outgoingHost,
		filter:       f,
		route:        route,
		enqueuedAt:   time.Now(),
	}
	select {
	case f.revalJobs <- job:
	case <-f.ctx.Done():
		f.metrics.IncCounter("cache.reval_dropped")
		f.incRouteReval(route, "dropped")
	default:
		f.metrics.IncCounter("cache.reval_dropped")
		f.incRouteReval(route, "dropped")
	}
}

// doRevalidate revalidates key against the upstream. It sends a conditional
// request (If-None-Match / If-Modified-Since) when the stored entry carries
// validators; a 304 response reuses the stored payload and merges new headers.
// backendURL, when non-empty, is dialed directly using outgoingHost as the
// Host header, bypassing skipper's proxy pipeline (force mode, static backend
// only) - so it's invisible to MeasureBackend* metrics and the access log;
// cache.reval_backend_dispatch is incremented instead. Otherwise the request
// loops back through skipper's own listener (f.listenAddr), which is counted
// normally.
func (f *cacheFilter) doRevalidate(key string, req *http.Request, backendURL, outgoingHost string, baseKeys ...string) {
	baseKey := key
	if len(baseKeys) > 0 {
		baseKey = baseKeys[0]
	}
	f.revalidate(key, baseKey, req, backendURL, outgoingHost, time.Time{}, "unknown")
}

func (f *cacheFilter) doRevalidateQueued(job revalJob) {
	f.revalidate(job.key, job.baseKey, job.req, job.backendURL, job.outgoingHost, job.enqueuedAt, job.route)
}

func (f *cacheFilter) revalidate(key, baseKey string, req *http.Request, backendURL, outgoingHost string, enqueuedAt time.Time, route string) {
	f.revalSF.Do(key, func() (any, error) { //nolint:errcheck
		// A previous queued job may have refreshed this variant while this job
		// waited. Failed refreshes leave the original timestamp, so remain retryable.
		if !enqueuedAt.IsZero() {
			if current, err := f.storage.GetSilent(context.Background(), key); err == nil && current != nil &&
				!current.CreatedAt.Before(enqueuedAt) && time.Now().Before(current.CreatedAt.Add(current.TTL)) {
				return nil, nil
			}
		}
		f.incRouteReval(route, "started")
		outcome := "failed"
		defer func() { f.incRouteReval(route, outcome) }()
		req, err := f.prepareDispatch(req, backendURL, outgoingHost)
		if err != nil {
			f.metrics.IncCounter("cache.reval_error")
			log.WithField("backendURL", backendURL).WithError(err).Warn("cache: failed to prepare background revalidation dispatch")
			return nil, nil
		}
		if backendURL != "" {
			f.metrics.IncCounter("cache.reval_backend_dispatch")
		}

		if stored, err := f.storage.Get(context.Background(), key); err == nil && stored != nil {
			if stored.ETag != "" {
				req.Header.Set("If-None-Match", stored.ETag)
			}
			if stored.LastModified != "" {
				req.Header.Set("If-Modified-Since", stored.LastModified)
			}
		}

		requestTime := time.Now()
		resp, err := f.fetch(req)
		f.observeOriginFetch(route, backendURL != "", requestTime, resp, err)
		if err != nil {
			f.metrics.IncCounter("cache.reval_error")
			log.WithFields(log.Fields{
				"url": req.URL.String(),
			}).WithError(err).Warn("cache: background revalidation fetch failed")
			return nil, nil
		}
		responseTime := time.Now()
		defer resp.Body.Close()

		originFailed := resp.StatusCode >= 500 && resp.StatusCode < 600
		if originFailed {
			f.metrics.IncCounter("cache.reval_error")
		}
		if staleIfErrorStatus(resp.StatusCode) && f.staleIfErrorEntry(key) != nil {
			return nil, nil
		}

		var body []byte
		var responseHeader http.Header
		var statusCode int

		if resp.StatusCode == http.StatusNotModified {
			// Reuse stored payload and merge any updated headers from 304.
			if stored, serr := f.storage.Get(context.Background(), key); serr == nil && stored != nil {
				body = stored.Payload
				responseHeader = stored.Header.Clone()
				mergeHeaders := resp.Header.Clone()
				stripHopByHop(mergeHeaders)
				for k, vv := range mergeHeaders {
					responseHeader[k] = vv
				}
			} else {
				outcome = "dropped"
				log.WithField("key", key).Debug("cache: revalidation received 304 but entry was evicted, skipping refresh")
				return nil, nil
			}
			statusCode = http.StatusOK
		} else {
			var rerr error
			body, rerr = io.ReadAll(resp.Body)
			if rerr != nil {
				if !originFailed {
					f.metrics.IncCounter("cache.reval_error")
				}
				log.WithFields(log.Fields{
					"url": req.URL.String(),
				}).WithError(rerr).Warn("cache: failed to read response body during background revalidation")
				return nil, nil
			}
			responseHeader = resp.Header.Clone()
			stripHopByHop(responseHeader)
			statusCode = resp.StatusCode
		}

		directives := parseCacheControl(responseHeader)
		ttl, shouldStore := f.storeDecision(req, statusCode, responseHeader, directives)
		if !shouldStore {
			if !originFailed {
				outcome = "dropped"
			}
			return nil, nil
		}

		cia := correctedInitialAge(requestTime, responseTime, responseHeader)
		entry := &Entry{
			StatusCode:           statusCode,
			Header:               responseHeader,
			Payload:              body,
			CreatedAt:            responseTime,
			TTL:                  ttl,
			StaleWhileRevalidate: f.resolveSWR(statusCode, directives),
			StaleIfError:         f.staleIfError,
			ETag:                 responseHeader.Get("ETag"),
			LastModified:         responseHeader.Get("Last-Modified"),
			CorrectedInitialAge:  cia,
			ResponseTime:         responseTime,
		}
		if err := f.storeEntry(context.Background(), baseKey, key, req, entry); err != nil {
			log.WithError(err).Warn("cache: Set failed (background revalidation)")
		} else if !originFailed {
			outcome = "success"
		}
		return nil, nil
	})
}

func (f *cacheFilter) ttlForStatus(status int) time.Duration {
	switch {
	case status == http.StatusOK:
		return f.ttl
	case status == http.StatusNotFound || (status >= 500 && status < 600):
		return f.errorTTL
	default:
		return 0
	}
}

// resolveTTL decides the TTL to use for a stored entry.
//
// Force mode (rfcMode==false): operator ttl/errorTTL is authoritative; upstream
// freshness directives are ignored. Semantic blockers (no-store, private) are
// ignored too — callers must gate those in RFC mode only.
//
// RFC mode (rfcMode==true): upstream Cache-Control drives TTL per RFC 9111.
// Heuristic freshness (§4.2.2) applies when no explicit directive is present.
//
// Returns (ttl, store) where store==false means the response must not be cached
// (only possible in RFC mode when no directive and heuristic yields nothing).
func (f *cacheFilter) resolveTTL(statusCode int, header http.Header, directives cacheDirectives) (ttl time.Duration, store bool) {
	if !f.rfcMode {
		// no-cache: store with TTL=0 so ETag/Last-Modified survive for conditional revalidation.
		if directives.noCache {
			return 0, true
		}
		baseTTL := f.ttlForStatus(statusCode)
		return baseTTL, baseTTL > 0
	}

	if directives.noCache {
		return 0, true
	}
	hasExplicitDirective := directives.maxAge >= 0 || directives.sMaxAge >= 0 || header.Get("Expires") != ""
	if !hasExplicitDirective {
		if h := heuristicTTL(header, directives, time.Now()); h > 0 {
			return h, true
		}
		return 0, false
	}
	// s-maxage takes precedence over max-age (RFC 9111 §5.2.2.10), then Expires.
	if directives.sMaxAge >= 0 {
		ttl = time.Duration(directives.sMaxAge) * time.Second
	} else if directives.maxAge >= 0 {
		ttl = time.Duration(directives.maxAge) * time.Second
	} else {
		ttl = capTTLByExpires(0, header, directives) // 0 = uncapped
	}
	// Store even when TTL=0 (expired/invalid Expires or max-age=0) so
	// ETag/Last-Modified are preserved for conditional revalidation (§5.2.2.4).
	return ttl, true
}

// resolveSWR decides the stale-while-revalidate window for a stored entry.
//
// Force mode (rfcMode==false): operator swrWindow is authoritative, matching
// resolveTTL's force-mode behavior.
//
// RFC mode (rfcMode==true): honors the response's stale-while-revalidate
// directive (RFC 5861 §3) when present; otherwise there is no SWR window.
func (f *cacheFilter) resolveSWR(statusCode int, directives cacheDirectives) time.Duration {
	if statusCode != http.StatusOK {
		return 0
	}
	if !f.rfcMode {
		return f.swrWindow
	}
	if directives.staleWhileRevalidate >= 0 {
		return time.Duration(directives.staleWhileRevalidate) * time.Second
	}
	return 0
}

// cacheKey builds a deterministic cache key from the route ID and request.
// routeID is included so entries from different routes never collide when all
// routes share the same LRUStorage instance.
// SHA-256 is used for uniform distribution and to satisfy the lru.go requirement
// that keys passed to ShardedByteLRU are pre-hashed by the caller.
// keyHeaders is an optional list of request header names whose values are folded
// into the key, allowing per-header cache isolation (e.g. "Authorization").
func cacheKey(routeID string, r *http.Request, keyHeaders []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s://%s%s?%s", routeID, r.URL.Scheme, r.Host, r.URL.Path, r.URL.RawQuery)
	for _, name := range keyHeaders {
		fmt.Fprintf(h, "\n%s: %s", name, r.Header.Get(name))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// parseHTTPTime parses an HTTP date per RFC 9111 §4.2.
// Rejects dates with non-GMT zone abbreviations that http.ParseTime would silently
// accept with a wrong offset (RFC 850 format only; RFC 1123 already rejects non-GMT).
func parseHTTPTime(s string) (time.Time, error) {
	t, err := http.ParseTime(s)
	if err != nil {
		return time.Time{}, err
	}
	name, offset := t.Zone()
	if offset != 0 || (name != "UTC" && name != "GMT") {
		return time.Time{}, fmt.Errorf("cache: non-GMT date %q rejected", s)
	}
	return t, nil
}

// correctedInitialAge computes the corrected_initial_age per RFC 9111 §4.2.3.
// It is called once at cache-entry-creation time and stored in Entry.CorrectedInitialAge.
func correctedInitialAge(requestTime, responseTime time.Time, rspHeader http.Header) time.Duration {
	var apparentAge time.Duration
	if dateStr := rspHeader.Get("Date"); dateStr != "" {
		if dateVal, err := parseHTTPTime(dateStr); err == nil {
			if diff := responseTime.Sub(dateVal); diff > 0 {
				apparentAge = diff
			}
		}
	}
	var ageValue time.Duration
	if ageStr := rspHeader.Get("Age"); ageStr != "" {
		if v, err := strconv.ParseInt(ageStr, 10, 64); err == nil && v >= 0 {
			ageValue = time.Duration(v) * time.Second
		}
	}
	responseDelay := max(responseTime.Sub(requestTime), 0)
	correctedAgeValue := ageValue + responseDelay
	if correctedAgeValue > apparentAge {
		return correctedAgeValue
	}
	return apparentAge
}

// setAgeHeader computes and sets the Age header per RFC 9111 §4.2.3.
// For new entries (ResponseTime set), uses precise corrected_initial_age formula.
// For legacy entries (ResponseTime zero), falls back to the elapsed-time formula.
func setAgeHeader(rsp *http.Response, entry *Entry, now time.Time) {
	var age int64
	if !entry.ResponseTime.IsZero() {
		// RFC 9111 §4.2.3 precise formula.
		residentTime := max(now.Sub(entry.ResponseTime), 0)
		secs := max(int64((entry.CorrectedInitialAge + residentTime).Seconds()), 0)
		age = secs
	} else {
		elapsed := max(int64(now.Sub(entry.CreatedAt).Seconds()), 0)
		if existing := rsp.Header.Get("Age"); existing != "" {
			if v, err := strconv.ParseInt(existing, 10, 64); err == nil {
				elapsed += v
			}
		}
		age = elapsed
	}
	rsp.Header.Set("Age", strconv.FormatInt(age, 10))
}

// evaluateConditionals checks client If-None-Match / If-Modified-Since against
// a cached entry per RFC 9111 §4.3.2 / RFC 9110 §13. Returns true when the
// client condition is "not modified" (cache should respond 304).
// Only call for cacheable methods (GET, HEAD).
func evaluateConditionals(req *http.Request, entry *Entry) bool {
	if inm := req.Header.Get("If-None-Match"); inm != "" {
		return matchesETag(inm, entry.ETag)
	}
	if ims := req.Header.Get("If-Modified-Since"); ims != "" && entry.LastModified != "" {
		imsTime, err := parseHTTPTime(ims)
		if err != nil {
			return false
		}
		lmTime, err := parseHTTPTime(entry.LastModified)
		if err != nil {
			return false
		}
		return !lmTime.After(imsTime)
	}
	return false
}

// matchesETag reports whether the If-None-Match header value matches etag
// using weak comparison per RFC 9110 §8.8.3.2.
func matchesETag(ifNoneMatch, etag string) bool {
	if etag == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	normalise := func(e string) string {
		return strings.TrimPrefix(strings.TrimSpace(e), "W/")
	}
	normEtag := normalise(etag)
	for token := range strings.SplitSeq(ifNoneMatch, ",") {
		if normalise(token) == normEtag {
			return true
		}
	}
	return false
}

// heuristicTTL returns a heuristic freshness lifetime per RFC 9111 §4.2.2:
// ttl = 0.1 * (Date - Last-Modified). Returns 0 when any explicit freshness
// directive is present, when Last-Modified is absent, or when inapplicable.
func heuristicTTL(header http.Header, d cacheDirectives, now time.Time) time.Duration {
	if d.maxAge >= 0 || d.sMaxAge >= 0 || header.Get("Expires") != "" {
		return 0
	}
	lmStr := header.Get("Last-Modified")
	if lmStr == "" {
		return 0
	}
	lm, err := parseHTTPTime(lmStr)
	if err != nil {
		return 0
	}
	ref := now
	if dateStr := header.Get("Date"); dateStr != "" {
		if t, err := parseHTTPTime(dateStr); err == nil {
			ref = t
		}
	}
	age := ref.Sub(lm)
	if age <= 0 {
		return 0
	}
	return time.Duration(float64(age) * 0.1)
}

func capTTLByExpires(ttl time.Duration, header http.Header, d cacheDirectives) time.Duration {
	if d.maxAge >= 0 || d.sMaxAge >= 0 {
		return ttl // RFC 9111 §5.3: Expires ignored when max-age or s-maxage present
	}
	exp := header.Get("Expires")
	if exp == "" {
		return ttl
	}
	t, err := parseHTTPTime(exp)
	if err != nil {
		return 0 // RFC 9111 §5.3: invalid date (incl. "0") = already expired
	}
	remaining := time.Until(t)
	if remaining <= 0 {
		return 0
	}
	if ttl == 0 || remaining < ttl { // ttl==0 means uncapped (pure RFC mode)
		return remaining
	}
	return ttl
}

// isCacheableMethod reports whether method may be served from cache per RFC 9111.
func isCacheableMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodPatch:
		return true
	}
	return false
}

// headBodyOmitted returns rsp with an empty body when method is HEAD.
// RFC 9110 §9.3.2: HEAD responses must not include a message body.
func headBodyOmitted(method string, rsp *http.Response) *http.Response {
	if method != http.MethodHead {
		return rsp
	}
	out := *rsp
	out.Body = http.NoBody
	out.ContentLength = 0
	return &out
}

// bodyRelatedHeaders are omitted when freshening a GET entry from a HEAD response
// (RFC 9111 §4.3.5). These headers describe the body and are meaningless from HEAD.
var bodyRelatedHeaders = map[string]bool{
	"Content-Length":    true,
	"Content-Range":     true,
	"Transfer-Encoding": true,
}

// hopByHopHeaders are never stored in a cache entry per RFC 9111 §3.1 / RFC 9110 §7.6.1.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// stripHopByHop removes hop-by-hop headers from h in-place.
// Also removes any header named in the Connection value per RFC 9110 §7.6.1.
func stripHopByHop(h http.Header) {
	for _, name := range h.Values("Connection") {
		for field := range strings.SplitSeq(name, ",") {
			h.Del(strings.TrimSpace(field))
		}
	}
	for name := range hopByHopHeaders {
		h.Del(name)
	}
}

func parseVaryNames(varyHeader string) []string {
	if varyHeader == "" {
		return nil
	}
	var names []string
	for p := range strings.SplitSeq(varyHeader, ",") {
		if name := strings.TrimSpace(p); name != "" {
			names = append(names, http.CanonicalHeaderKey(name))
		}
	}
	return names
}

func varyKey(base string, r *http.Request, varyHeaders []string) string {
	if len(varyHeaders) == 0 {
		return base
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s", base)
	for _, name := range varyHeaders {
		fmt.Fprintf(h, "\n%s: %s", name, r.Header.Get(name))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func toDuration(v any) (time.Duration, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("expected string, got %T", v)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %v", d)
	}
	return d, nil
}

// sameOrigin reports whether rawTarget is same-origin as base.
// Relative URIs (no host) are always treated as same-origin.
func sameOrigin(base *http.Request, rawTarget string) bool {
	u, err := url.Parse(rawTarget)
	if err != nil || u.Host == "" {
		return true
	}
	return u.Scheme == base.URL.Scheme && u.Host == base.URL.Host
}

// cacheKeyForURL builds the cache key for rawTarget resolved against base.
// routeID must match the routeID used when the entry was originally stored.
// Returns "" if rawTarget cannot be parsed.
func cacheKeyForURL(routeID string, base *http.Request, rawTarget string, keyHeaders []string) string {
	u, err := url.Parse(rawTarget)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		host := base.Host
		if host == "" {
			host = base.URL.Host
		}
		u.Host = host
		u.Scheme = base.URL.Scheme
	}
	// Synthesise a minimal *http.Request so we can reuse cacheKey.
	synthetic := &http.Request{URL: u, Host: u.Host, Header: base.Header}
	return cacheKey(routeID, synthetic, keyHeaders)
}
