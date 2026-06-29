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

	// revalQueueSize is the capacity of the shared revalidation job queue.
	// Sized to absorb short bursts; jobs are dropped (with reval_dropped metric)
	// if the worker cannot keep up.
	revalQueueSize = 256
)

// Options configures the cache filter. All fields are required unless stated otherwise.
type Options struct {
	MaxBytes   int64                    // in-memory storage budget for the LRU
	ListenAddr string                   // Skipper's own listener address (e.g., ":9090")
	NetOpts    skpnet.Options           // network options for revalidation requests
	ValkeyRing *skpnet.ValkeyRingClient // optional L2 cache backend; nil = LRU only
	L1TTL      time.Duration            // max TTL to use when warming L1 from Valkey writes
	Metrics    metrics.Metrics          // optional; defaults to metrics.Default if nil
}

// NewCacheFilter returns a Spec for the cache() filter.
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
		m.IncCounter("lru_eviction")
	}, m)

	var store Storage = lru
	if opts.ValkeyRing != nil {
		store = NewValkeyStorage(opts.ValkeyRing, lru, m, opts.L1TTL)
	}

	spec := &cacheSpec{
		maxBytes:     opts.MaxBytes,
		listenAddr:   opts.ListenAddr,
		client:       skpnet.NewClient(opts.NetOpts),
		storage:      store,
		lruStorage:   lru,
		metrics:      m,
		revalJobs:    make(chan revalJob, revalQueueSize),
		lruBytesDone: make(chan struct{}),
	}

	// Start shared background goroutines (one worker + one scraper for all filter instances)
	spec.bgWg.Add(2)
	go spec.revalidationWorker()
	go spec.lruBytesScraper()

	return spec
}

type cacheSpec struct {
	maxBytes     int64
	listenAddr   string
	client       *skpnet.Client
	storage      Storage     // shared across all filter instances
	lruStorage   *LRUStorage // always non-nil; direct reference to L1, even when storage is ValkeyStorage
	metrics      metrics.Metrics
	revalJobs    chan revalJob  // shared queue; one spec-level worker drains this
	lruBytesDone chan struct{}  // closed to signal lruBytesScraper to stop
	bgWg         sync.WaitGroup // tracks spec-level background goroutines
}

func (s *cacheSpec) Name() string { return filterName }

// Close shuts down the background revalidation worker and lru_bytes scraper.
// Safe to call multiple times; idempotent via a guard.
func (s *cacheSpec) Close() {
	select {
	case <-s.lruBytesDone:
		// Already closed; prevent panic on double-close.
	default:
		close(s.revalJobs)
		close(s.lruBytesDone)
		s.bgWg.Wait()
	}
}

func (s *cacheSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 && (len(args) < 3 || len(args) > 5) {
		return nil, fmt.Errorf("cache: expected 0 or 3-5 args (ttl, errorTTL, swrWindow[, staleIfError[, keyHeaders]]), got %d: %w", len(args), filters.ErrInvalidFilterParameters)
	}

	rfcMode := len(args) == 0 // zero args → pure RFC mode

	var ttl, errorTTL, swr, staleIfError time.Duration
	if len(args) >= 3 {
		var err error
		ttl, err = toDuration(args[0])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 0 (ttl): %v: %w", err, filters.ErrInvalidFilterParameters)
		}

		errorTTL, err = toDuration(args[1])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 1 (errorTTL): %v: %w", err, filters.ErrInvalidFilterParameters)
		}

		swr, err = toDuration(args[2])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 2 (swrWindow): %v: %w", err, filters.ErrInvalidFilterParameters)
		}

		if len(args) >= 4 {
			if _, ok := args[3].(string); !ok {
				return nil, fmt.Errorf("cache: arg 3 (staleIfError): expected string, got %T: %w", args[3], filters.ErrInvalidFilterParameters)
			}
			staleIfError, err = time.ParseDuration(args[3].(string))
			if err != nil {
				return nil, fmt.Errorf("cache: arg 3 (staleIfError): %v: %w", err, filters.ErrInvalidFilterParameters)
			}
		}
	}

	var keyHeaders []string
	if len(args) == 5 {
		raw, ok := args[4].(string)
		if !ok {
			return nil, fmt.Errorf("cache: arg 4 (keyHeaders): expected string, got %T: %w", args[4], filters.ErrInvalidFilterParameters)
		}
		for h := range strings.SplitSeq(raw, ",") {
			if name := strings.TrimSpace(h); name != "" {
				keyHeaders = append(keyHeaders, http.CanonicalHeaderKey(name))
			}
		}
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
		revalJobs:    s.revalJobs,    // use spec-level shared channel
		lruBytesDone: s.lruBytesDone, // use spec-level shared signal
	}

	cf.fetch = s.client.Do
	return cf, nil
}

// revalidationWorker is the single background goroutine (spec-level, shared across
// all filter instances) that processes revalidation jobs sequentially. It calls
// the per-instance doRevalidateFn closure to respect each route's configuration.
func (s *cacheSpec) revalidationWorker() {
	defer s.bgWg.Done()
	for job := range s.revalJobs {
		if job.doRevalFn != nil {
			job.doRevalFn()
		}
	}
	log.Debug("cache: revalidation worker stopped")
}

const lruBytesScrapeInterval = 10 * time.Second

// lruBytesScraper periodically updates the lru_bytes gauge so it stays current
// even when no evictions occur. It's spec-level and shared across all filter instances.
// It exits when lruBytesDone is closed (via cacheSpec.Close).
func (s *cacheSpec) lruBytesScraper() {
	defer s.bgWg.Done()
	ticker := time.NewTicker(lruBytesScrapeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.metrics.UpdateGauge("lru_bytes", float64(s.lruStorage.lru.Bytes()))
		case <-s.lruBytesDone:
			return
		}
	}
}

type revalJob struct {
	key       string
	req       *http.Request // pre-cloned, safe to use after the originating request ends
	doRevalFn func()        // closure with access to per-instance doRevalidate
}

type cacheFilter struct {
	storage      Storage
	lruStorage   *LRUStorage // always non-nil; direct reference to L1, even when storage is ValkeyStorage
	listenAddr   string
	ttl          time.Duration
	errorTTL     time.Duration
	swrWindow    time.Duration
	staleIfError time.Duration
	keyHeaders   []string // request headers folded into the base cache key

	// rfcMode true: upstream Cache-Control is authoritative (cache()).
	// false: operator ttl/errorTTL/swrWindow are authoritative (force mode).
	rfcMode      bool
	coldSF       singleflight.Group // cold-miss coalescing
	revalSF      singleflight.Group // coalesces concurrent background revalidations per key
	revalJobs    chan revalJob      // shared background revalidation queue from cacheSpec
	lruBytesDone chan struct{}      // shared channel from cacheSpec; closed to stop scraper
	fetch        func(*http.Request) (*http.Response, error)
	metrics      metrics.Metrics
}

// Close is a no-op on individual filter instances; the real Close is on cacheSpec.
// This exists for test compatibility.
func (f *cacheFilter) Close() {}

// tagSpan sets cache_status, cache_key, and (when >= 0) cache_ttl_remaining_ms
// on the active OpenTracing span. No-op when no span is present.
func tagSpan(ctx filters.FilterContext, status, key string, ttlRemainingMs int64) {
	span := opentracing.SpanFromContext(ctx.Request().Context())
	if span == nil {
		return
	}
	span.SetTag("cache_status", status)
	span.SetTag("cache_key", key)
	if ttlRemainingMs >= 0 {
		span.SetTag("cache_ttl_remaining_ms", ttlRemainingMs)
	}
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
	if sentinel, _ := f.storage.Get(ctx.Request().Context(), "vary:"+baseKey); sentinel != nil && len(sentinel.VaryHeaders) > 0 {
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
		tagSpan(ctx, cacheStatusStale, key, -1)
		setAgeHeader(rsp, entry, now)
		ctx.Metrics().IncCounter("stale")
		method := ctx.Request().Method
		if (method == http.MethodGet || method == http.MethodHead) && evaluateConditionals(ctx.Request(), entry) {
			notModified := &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     rsp.Header.Clone(),
				Body:       http.NoBody,
				Request:    ctx.Request(), // link response to originating request per net/http convention
			}
			ctx.Serve(notModified)
			f.enqueueRevalidation(key, ctx.Request())
			return
		}
		ctx.Serve(headBodyOmitted(method, rsp))
		f.enqueueRevalidation(key, ctx.Request())
		return
	}

	// Entry is past TTL+SWR but still in storage (kept alive by stale-if-error window).
	// Treat as a miss so the upstream is hit; Response() will apply stale-if-error on 5xx.
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
	tagSpan(ctx, cacheStatusHit, key, max(0, entry.TTL-time.Since(entry.CreatedAt)).Milliseconds())
	setAgeHeader(rsp, entry, time.Now())
	ctx.Metrics().IncCounter("hit")
	method := ctx.Request().Method
	if (method == http.MethodGet || method == http.MethodHead) && evaluateConditionals(ctx.Request(), entry) {
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

// coalesceResult carries both the fetched entry and any SIE-eligible stored entry
// snapshotted before the fetch. The snapshot is taken before f.fetch() so that a
// 5xx result cannot overwrite it in storage before the stale-if-error check runs.
type coalesceResult struct {
	entry  *Entry
	stored *Entry // snapshot before fetch; nil if no eligible SIE entry existed
}

// coalesce gates concurrent cold misses for the same key behind a single upstream
// fetch, preventing thundering herd. All waiters receive the same response.
// Stale-if-error (RFC 5861 §4) is also applied here: a pre-fetch snapshot of any
// eligible stored entry is served on 5xx instead of propagating the error.
func (f *cacheFilter) coalesce(ctx filters.FilterContext, key string) {
	req := ctx.Request().Clone(context.Background())

	ch := f.coldSF.DoChan(key, func() (interface{}, error) {
		// Capture any existing SIE-eligible entry before fetching, so that a
		// subsequent 5xx response cannot overwrite it in storage before we read it.
		var sieStored *Entry
		if f.staleIfError > 0 {
			if s, err := f.storage.Get(context.Background(), key); err == nil && s != nil {
				staleAge := time.Since(s.CreatedAt) - s.TTL
				if staleAge <= f.staleIfError {
					sieStored = s
				}
			}
		}

		requestTime := time.Now()
		resp, err := f.fetch(req)
		if err != nil {
			return nil, err
		}
		responseTime := time.Now()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		directives := parseCacheControl(resp.Header)
		// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
		if f.rfcMode && (directives.noStore || directives.private) {
			cia := correctedInitialAge(requestTime, responseTime, resp.Header)
			coalescedHeader := resp.Header.Clone()
			stripHopByHop(coalescedHeader)
			return &coalesceResult{
				entry: &Entry{
					StatusCode:          resp.StatusCode,
					Header:              coalescedHeader,
					Payload:             body,
					CreatedAt:           responseTime,
					TTL:                 0,
					CorrectedInitialAge: cia,
					ResponseTime:        responseTime,
				},
				stored: sieStored,
			}, nil
		}
		ttl, shouldStore := f.resolveTTL(resp.StatusCode, resp.Header, directives)
		swr := f.swrWindow
		if resp.StatusCode != http.StatusOK {
			swr = 0
		}
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
		if shouldStore {
			if err := f.storage.Set(context.Background(), key, entry); err != nil {
				log.WithError(err).Warn("cache: Set failed (cold-miss store)")
			}
		}
		return &coalesceResult{entry: entry, stored: sieStored}, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil || res.Val == nil {
			ctx.Metrics().IncCounter("coalesce_error")
			return
		}
		cr := res.Val.(*coalesceResult)
		entry := cr.entry

		// RFC 5861 §4: on 5xx, serve a stale entry if within the stale-if-error window.
		// This check lives here (not in Response()) because coalesce always calls
		// ctx.Serve(), which sets ctx.Served()=true and causes Response() to return early.
		if cr.stored != nil && entry.StatusCode >= 500 {
			staleRsp := &http.Response{
				StatusCode: cr.stored.StatusCode,
				Header:     cr.stored.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(cr.stored.Payload)),
			}
			staleRsp.Header.Set(cacheStatusHeader, cacheStatusStale)
			tagSpan(ctx, cacheStatusStale, key, -1)
			setAgeHeader(staleRsp, cr.stored, time.Now())
			ctx.Serve(headBodyOmitted(ctx.Request().Method, staleRsp))
			return
		}

		rsp := &http.Response{
			StatusCode: entry.StatusCode,
			Header:     entry.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(entry.Payload)),
		}
		rsp.Header.Set(cacheStatusHeader, cacheStatusMiss)
		tagSpan(ctx, cacheStatusMiss, key, -1)
		ctx.Metrics().IncCounter("miss")
		ctx.Serve(headBodyOmitted(ctx.Request().Method, rsp))
	case <-ctx.Request().Context().Done():
		// Client disconnected. Remaining waiters still receive their result.
	}
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
		tagSpan(ctx, cacheStatusMiss, key, -1)
		ctx.Metrics().IncCounter("miss")
		return
	}

	rsp.Header.Set(cacheStatusHeader, cacheStatusMiss)
	tagSpan(ctx, cacheStatusMiss, key, -1)
	ctx.Metrics().IncCounter("miss")

	// Vary: * means every response is unique — never cache.
	varyHeader := rsp.Header.Get("Vary")
	if varyHeader == "*" {
		return
	}

	directives := parseCacheControl(rsp.Header)
	// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
	if f.rfcMode && (directives.noStore || directives.private) {
		return
	}

	ttl, shouldStore := f.resolveTTL(rsp.StatusCode, rsp.Header, directives)
	if !shouldStore {
		return
	}

	if ctx.Request().Header.Get("Authorization") != "" && !directives.public && !directives.mustRevalidate {
		return
	}

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
	varyNames := parseVaryNames(varyHeader)
	storeKey := key
	if len(varyNames) > 0 {
		storeKey = varyKey(baseKey, ctx.Request(), varyNames)
		sentinel := &Entry{
			CreatedAt:            time.Now(),
			TTL:                  ttl,
			StaleWhileRevalidate: f.swrWindow,
			VaryHeaders:          varyNames,
		}
		if err := f.storage.Set(ctx.Request().Context(), "vary:"+baseKey, sentinel); err != nil {
			log.WithError(err).Warn("cache: Set failed (vary sentinel)")
		}
	}

	swr := f.swrWindow
	if rsp.StatusCode != http.StatusOK {
		swr = 0
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
		VaryHeaders:          varyNames,
		CorrectedInitialAge:  cia,
		ResponseTime:         responseTime,
	}
	if err := f.storage.Set(ctx.Request().Context(), storeKey, entry); err != nil {
		log.WithError(err).Warn("cache: Set failed (response store)")
	}
}

// enqueueRevalidation sends a revalidation job to the background worker.
// The request is cloned in the calling goroutine before orig is released.
// If the queue is full the job is dropped and reval_dropped is incremented.
// The closure captures f.doRevalidate so the spec-level worker respects this route's config.
func (f *cacheFilter) enqueueRevalidation(key string, orig *http.Request) {
	cloned := orig.Clone(context.Background())
	job := revalJob{
		key: key,
		req: cloned,
		doRevalFn: func() {
			f.doRevalidate(key, cloned)
		},
	}
	select {
	case f.revalJobs <- job:
	default:
		f.metrics.IncCounter("reval_dropped")
	}
}

// doRevalidate revalidates key against the upstream. It sends a conditional
// request (If-None-Match / If-Modified-Since) when the stored entry carries
// validators; a 304 response reuses the stored payload and merges new headers.
func (f *cacheFilter) doRevalidate(key string, req *http.Request) {
	f.revalSF.Do(key, func() (interface{}, error) { //nolint:errcheck
		req.Header.Set(revalidateHeader, "1")
		req.URL.Scheme = "http"
		req.URL.Host = f.listenAddr
		req.RequestURI = ""

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
		if err != nil {
			f.metrics.IncCounter("reval_error")
			log.WithFields(log.Fields{
				"url": req.URL.String(),
			}).WithError(err).Warn("cache: background revalidation fetch failed")
			return nil, nil
		}
		responseTime := time.Now()
		defer resp.Body.Close()

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
				log.WithField("key", key).Debug("cache: revalidation received 304 but entry was evicted, skipping refresh")
				return nil, nil
			}
			statusCode = http.StatusOK
		} else {
			var rerr error
			body, rerr = io.ReadAll(resp.Body)
			if rerr != nil {
				f.metrics.IncCounter("reval_error")
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
		// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
		if f.rfcMode && (directives.noStore || directives.private) {
			return nil, nil
		}
		ttl, shouldStore := f.resolveTTL(statusCode, responseHeader, directives)
		if !shouldStore {
			return nil, nil
		}

		cia := correctedInitialAge(requestTime, responseTime, responseHeader)
		entry := &Entry{
			StatusCode:           statusCode,
			Header:               responseHeader,
			Payload:              body,
			CreatedAt:            responseTime,
			TTL:                  ttl,
			StaleWhileRevalidate: f.swrWindow,
			StaleIfError:         f.staleIfError,
			ETag:                 responseHeader.Get("ETag"),
			LastModified:         responseHeader.Get("Last-Modified"),
			CorrectedInitialAge:  cia,
			ResponseTime:         responseTime,
		}
		if err := f.storage.Set(context.Background(), key, entry); err != nil {
			log.WithError(err).Warn("cache: Set failed (background revalidation)")
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
	responseDelay := responseTime.Sub(requestTime)
	if responseDelay < 0 {
		responseDelay = 0
	}
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
		residentTime := now.Sub(entry.ResponseTime)
		if residentTime < 0 {
			residentTime = 0
		}
		secs := int64((entry.CorrectedInitialAge + residentTime).Seconds())
		if secs < 0 {
			secs = 0
		}
		age = secs
	} else {
		elapsed := int64(now.Sub(entry.CreatedAt).Seconds())
		if elapsed < 0 {
			elapsed = 0
		}
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
// Only call for GET and HEAD requests.
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

func toDuration(v interface{}) (time.Duration, error) {
	var d time.Duration
	if dv, ok := v.(time.Duration); ok {
		d = dv
	} else {
		s, ok := v.(string)
		if !ok {
			return 0, fmt.Errorf("expected string, got %T", v)
		}
		var err error
		d, err = time.ParseDuration(s)
		if err != nil {
			return 0, err
		}
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
