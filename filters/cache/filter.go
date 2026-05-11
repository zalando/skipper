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
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	sknet "github.com/zalando/skipper/net"
	"golang.org/x/sync/singleflight"
)

const (
	Name        = "cache"
	filterName  = Name
	stateBagKey = "cache:key"

	// revalidateHeader is set on background revalidation requests so the cache
	// filter skips the cache lookup and lets the response populate a fresh entry.
	revalidateHeader = "X-Cache-Revalidate"
)

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
func NewCacheFilter(maxBytes int64, listenAddr string) filters.Spec {
	return &cacheSpec{
		maxBytes:   maxBytes,
		listenAddr: listenAddr,
		client:     sknet.NewClient(sknet.Options{}),
		storage:    NewLRUStorage(maxBytes, func() { metrics.Default.IncCounter("lru_eviction") }),
	}
}

type cacheSpec struct {
	maxBytes   int64
	listenAddr string
	client     *sknet.Client
	storage    Storage // shared across all filter instances
}

func (s *cacheSpec) Name() string { return filterName }

func (s *cacheSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 && (len(args) < 3 || len(args) > 5) {
		return nil, fmt.Errorf("cache: expected 0 or 3-5 args (ttl, errorTTL, swrWindow[, staleIfError[, keyHeaders]]), got %d", len(args))
	}

	var ttl, errorTTL, swr, staleIfError time.Duration
	rfcMode := len(args) == 0 // zero args → pure RFC mode

	if len(args) >= 3 {
		var err error
		ttl, err = toDuration(args[0])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 0 (ttl): %w", err)
		}

		errorTTL, err = toDuration(args[1])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 1 (errorTTL): %w", err)
		}

		swr, err = toDuration(args[2])
		if err != nil {
			return nil, fmt.Errorf("cache: arg 2 (swrWindow): %w", err)
		}

		if len(args) >= 4 {
			s, ok := args[3].(string)
			if !ok {
				return nil, fmt.Errorf("cache: arg 3 (staleIfError): expected string, got %T", args[3])
			}
			staleIfError, err = time.ParseDuration(s)
			if err != nil {
				return nil, fmt.Errorf("cache: arg 3 (staleIfError): %w", err)
			}
		}
	}

	var keyHeaders []string
	if len(args) == 5 {
		raw, ok := args[4].(string)
		if !ok {
			return nil, fmt.Errorf("cache: arg 4 (keyHeaders): expected string, got %T", args[4])
		}
		for _, h := range strings.Split(raw, ",") {
			if name := strings.TrimSpace(h); name != "" {
				keyHeaders = append(keyHeaders, http.CanonicalHeaderKey(name))
			}
		}
	}

	m := metrics.Default
	cf := &cacheFilter{
		storage:      s.storage,
		listenAddr:   s.listenAddr,
		ttl:          ttl,
		errorTTL:     errorTTL,
		swrWindow:    swr,
		staleIfError: staleIfError,
		rfcMode:      rfcMode,
		metrics:      m,
		keyHeaders:   keyHeaders,
	}
	cf.fetch = s.client.Do
	return cf, nil
}

type cacheFilter struct {
	storage      Storage
	listenAddr   string
	ttl          time.Duration
	errorTTL     time.Duration
	swrWindow    time.Duration
	staleIfError time.Duration
	keyHeaders   []string // request headers folded into the base cache key
	// rfcMode true: upstream Cache-Control is authoritative (cache()).
	// false: operator ttl/errorTTL/swrWindow are authoritative (force mode).
	rfcMode bool
	sf      singleflight.Group // cold-miss coalescing
	revalSF singleflight.Group // background revalidation coalescing
	fetch   func(*http.Request) (*http.Response, error)
	metrics metrics.Metrics
}

// Request checks the cache. On a hit it calls ctx.Serve() to short-circuit the
// backend roundtrip. On a stale hit it serves stale and fires a background
// revalidation. On a miss it stores the computed key in the state bag so
// Response() can populate the cache.
func (f *cacheFilter) Request(ctx filters.FilterContext) {
	baseKey := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
	key := baseKey
	if sentinel, _ := f.storage.Get(ctx.Request().Context(), "vary:"+baseKey); sentinel != nil && len(sentinel.VaryHeaders) > 0 {
		key = varyKey(baseKey, ctx.Request(), sentinel.VaryHeaders)
	}
	ctx.StateBag()[stateBagKey] = key
	ctx.StateBag()["cache:base-key"] = baseKey
	ctx.StateBag()["cache:request-time"] = time.Now()

	// Unsafe methods skip cache lookup; Response() will invalidate the entry.
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
	if reqDir.OnlyIfCached {
		entry, err := f.storage.Get(ctx.Request().Context(), key)
		if err != nil || entry == nil || entry.IsStale(time.Now()) {
			ctx.Serve(&http.Response{
				StatusCode: http.StatusGatewayTimeout,
				Header:     http.Header{"X-Cache-Status": {"MISS"}},
				Body:       http.NoBody,
			})
			return
		}
	} else if reqDir.NoCache || reqDir.NoStore {
		if reqDir.NoStore {
			ctx.StateBag()["cache:no-store"] = true
		}
		return
	}

	entry, err := f.storage.Get(ctx.Request().Context(), key)
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
		if d.MustRevalidate || d.ProxyRevalidate || d.NoCache || d.SMaxAge >= 0 {
			f.coalesce(ctx, key)
			return
		}
		// RFC 9111 §5.2.1 max-stale.
		if reqDir.MaxStale >= 0 {
			staleness := time.Since(entry.CreatedAt) - entry.TTL
			if staleness > time.Duration(reqDir.MaxStale)*time.Second {
				f.coalesce(ctx, key)
				return
			}
		}
		rsp.Header.Set("X-Cache-Status", "STALE")
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
			go f.revalidate(key, ctx.Request())
			return
		}
		ctx.Serve(headBodyOmitted(method, rsp))
		go f.revalidate(key, ctx.Request())
		return
	}

	// Entry is past TTL+SWR but still in storage (kept alive by stale-if-error window).
	// Treat as a miss so the upstream is hit; Response() will apply stale-if-error on 5xx.
	if pastTTL {
		f.coalesce(ctx, key)
		return
	}

	// RFC 9111 §5.2.1 min-fresh.
	if reqDir.MinFresh >= 0 {
		elapsed := time.Since(entry.CreatedAt)
		remaining := entry.TTL - elapsed
		if remaining < time.Duration(reqDir.MinFresh)*time.Second {
			f.coalesce(ctx, key)
			return
		}
	}

	rsp.Header.Set("X-Cache-Status", "HIT")
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

// coalesce gates concurrent cold misses for the same key behind a single
// upstream fetch. All waiters block until the leader's fetch completes, then
// all are served the same response. This prevents the thundering herd on a
// cache miss.
func (f *cacheFilter) coalesce(ctx filters.FilterContext, key string) {
	req := ctx.Request().Clone(context.Background())

	ch := f.sf.DoChan(key, func() (interface{}, error) {
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
		if f.rfcMode && (directives.NoStore || directives.Private) {
			cia := correctedInitialAge(requestTime, responseTime, resp.Header)
			coalescedHeader := resp.Header.Clone()
			stripHopByHop(coalescedHeader)
			return &Entry{
				StatusCode:          resp.StatusCode,
				Header:              coalescedHeader,
				Payload:             body,
				CreatedAt:           responseTime,
				TTL:                 0,
				CorrectedInitialAge: cia,
				ResponseTime:        responseTime,
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
			_ = f.storage.Set(context.Background(), key, entry)
		}
		return entry, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil || res.Val == nil {
			ctx.Metrics().IncCounter("coalesce_error")
			return
		}
		entry := res.Val.(*Entry)
		rsp := &http.Response{
			StatusCode: entry.StatusCode,
			Header:     entry.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(entry.Payload)),
		}
		rsp.Header.Set("X-Cache-Status", "MISS")
		ctx.Metrics().IncCounter("miss")
		ctx.Serve(headBodyOmitted(ctx.Request().Method, rsp))
	case <-ctx.Request().Context().Done():
		// Client disconnected. Remaining waiters still receive their result.
	}
}

// Response stores the upstream response in the cache, freshens HEAD entries,
// invalidates on unsafe methods, and applies stale-if-error on 5xx.
func (f *cacheFilter) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()
	key := ctx.StateBag()[stateBagKey].(string)

	// RFC 5861 §4.
	if f.staleIfError > 0 && rsp.StatusCode >= 500 {
		if stored, err := f.storage.Get(ctx.Request().Context(), key); err == nil && stored != nil {
			staleAge := time.Since(stored.CreatedAt) - stored.TTL
			if staleAge <= f.staleIfError {
				staleRsp := &http.Response{
					StatusCode: stored.StatusCode,
					Header:     stored.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(stored.Payload)),
				}
				staleRsp.Header.Set("X-Cache-Status", "STALE")
				setAgeHeader(staleRsp, stored, time.Now())
				ctx.Serve(headBodyOmitted(ctx.Request().Method, staleRsp))
				return
			}
		}
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
			_ = f.storage.Set(ctx.Request().Context(), key, stored)
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
		_ = f.storage.Delete(ctx.Request().Context(), key)
		for _, hdrName := range []string{"Location", "Content-Location"} {
			if loc := rsp.Header.Get(hdrName); loc != "" && sameOrigin(ctx.Request(), loc) {
				if locKey := cacheKeyForURL(ctx.RouteId(), ctx.Request(), loc, f.keyHeaders); locKey != "" {
					_ = f.storage.Delete(ctx.Request().Context(), locKey)
					_ = f.storage.Delete(ctx.Request().Context(), "vary:"+locKey)
				}
			}
		}
		return
	}

	if ctx.StateBag()["cache:no-store"] == true {
		rsp.Header.Set("X-Cache-Status", "MISS")
		ctx.Metrics().IncCounter("miss")
		return
	}

	rsp.Header.Set("X-Cache-Status", "MISS")
	ctx.Metrics().IncCounter("miss")

	// Vary: * means every response is unique — never cache.
	varyHeader := rsp.Header.Get("Vary")
	if varyHeader == "*" {
		return
	}

	directives := parseCacheControl(rsp.Header)
	// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
	if f.rfcMode && (directives.NoStore || directives.Private) {
		return
	}

	ttl, shouldStore := f.resolveTTL(rsp.StatusCode, rsp.Header, directives)
	if !shouldStore {
		return
	}

	if ctx.Request().Header.Get("Authorization") != "" && !directives.Public && !directives.MustRevalidate {
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

	baseKey, _ := ctx.StateBag()["cache:base-key"].(string)
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
		_ = f.storage.Set(ctx.Request().Context(), "vary:"+baseKey, sentinel)
	}

	swr := f.swrWindow
	if rsp.StatusCode != http.StatusOK {
		swr = 0
	}
	responseTime := time.Now()
	var requestTime time.Time
	if rt, ok := ctx.StateBag()["cache:request-time"].(time.Time); ok {
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
	_ = f.storage.Set(ctx.Request().Context(), storeKey, entry)
}

// revalidate fetches the upstream resource in the background and refreshes the
// cache entry. Uses singleflight to coalesce concurrent revalidations for the
// same key. Uses a detached context so the fetch outlives the current request.
func (f *cacheFilter) revalidate(key string, orig *http.Request) {
	f.revalSF.Do(key, func() (interface{}, error) {
		req := orig.Clone(context.Background())
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
				"url": orig.URL.String(),
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
					"url": orig.URL.String(),
				}).WithError(rerr).Warn("cache: failed to read response body during background revalidation")
				return nil, nil
			}
			responseHeader = resp.Header.Clone()
			stripHopByHop(responseHeader)
			statusCode = resp.StatusCode
		}

		directives := parseCacheControl(responseHeader)
		// RFC mode only: no-store and private are semantic blockers (RFC 9111 §5.2.2).
		if f.rfcMode && (directives.NoStore || directives.Private) {
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
		_ = f.storage.Set(context.Background(), key, entry)
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
		if directives.NoCache {
			return 0, true
		}
		baseTTL := f.ttlForStatus(statusCode)
		return baseTTL, baseTTL > 0
	}

	if directives.NoCache {
		return 0, true
	}
	hasExplicitDirective := directives.MaxAge >= 0 || directives.SMaxAge >= 0 || header.Get("Expires") != ""
	if !hasExplicitDirective {
		if h := heuristicTTL(header, directives, time.Now()); h > 0 {
			return h, true
		}
		return 0, false
	}
	// s-maxage takes precedence over max-age (RFC 9111 §5.2.2.10), then Expires.
	if directives.SMaxAge >= 0 {
		ttl = time.Duration(directives.SMaxAge) * time.Second
	} else if directives.MaxAge >= 0 {
		ttl = time.Duration(directives.MaxAge) * time.Second
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
	for _, token := range strings.Split(ifNoneMatch, ",") {
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
	if d.MaxAge >= 0 || d.SMaxAge >= 0 || header.Get("Expires") != "" {
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
	if d.MaxAge >= 0 || d.SMaxAge >= 0 {
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
		for _, field := range strings.Split(name, ",") {
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
	parts := strings.Split(varyHeader, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
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
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("expected string, got %T", v)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %s", s)
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
