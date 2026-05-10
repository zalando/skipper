package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
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

// NewCacheFilter returns a Spec for the cache() filter. maxBytes is the L1
// in-memory storage budget per route, derived from cgroup memory limits in skipper.go.
// listenAddr is Skipper's own listener address (e.g. ":9090"); revalidation requests
// are sent back through Skipper so the full filter chain runs.
//
// Route usage:
//
//	-> cache("5m", "15s", "30s") -> "https://example.org"
func NewCacheFilter(maxBytes int64, listenAddr string) filters.Spec {
	storage := NewLRUStorage(maxBytes)
	for i := range storage.lru.shards {
		storage.lru.shards[i].onEvict = func() {
			// lru_eviction fires outside a request context, so metrics.Default is correct here.
			metrics.Default.IncCounter("lru_eviction")
		}
	}
	return &cacheSpec{
		storage:    storage,
		listenAddr: listenAddr,
		client:     sknet.NewClient(sknet.Options{}),
	}
}

type cacheSpec struct {
	storage    *LRUStorage
	listenAddr string
	client     *sknet.Client
}

func (s *cacheSpec) Name() string { return filterName }

func (s *cacheSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 3 || len(args) > 4 {
		return nil, fmt.Errorf("cache: expected 3 or 4 args (ttl, errorTTL, swrWindow[, keyHeaders]), got %d", len(args))
	}

	ttl, err := toDuration(args[0])
	if err != nil {
		return nil, fmt.Errorf("cache: arg 0 (ttl): %w", err)
	}

	errorTTL, err := toDuration(args[1])
	if err != nil {
		return nil, fmt.Errorf("cache: arg 1 (errorTTL): %w", err)
	}

	swr, err := toDuration(args[2])
	if err != nil {
		return nil, fmt.Errorf("cache: arg 2 (swrWindow): %w", err)
	}

	var keyHeaders []string
	if len(args) == 4 {
		raw, ok := args[3].(string)
		if !ok {
			return nil, fmt.Errorf("cache: arg 3 (keyHeaders): expected string, got %T", args[3])
		}
		for _, h := range strings.Split(raw, ",") {
			if name := strings.TrimSpace(h); name != "" {
				keyHeaders = append(keyHeaders, http.CanonicalHeaderKey(name))
			}
		}
	}

	cf := &cacheFilter{
		storage:    s.storage,
		listenAddr: s.listenAddr,
		ttl:        ttl,
		errorTTL:   errorTTL,
		swrWindow:  swr,
		keyHeaders: keyHeaders,
	}
	cf.fetch = s.client.Do
	return cf, nil
}

type cacheFilter struct {
	storage    Storage
	listenAddr string
	ttl        time.Duration
	errorTTL   time.Duration
	swrWindow  time.Duration
	keyHeaders []string           // request headers folded into the base cache key
	sf         singleflight.Group // cold-miss coalescing
	revalSF    singleflight.Group // background revalidation coalescing
	fetch      func(*http.Request) (*http.Response, error)
}

// Request checks the cache. On a hit it calls ctx.Serve() to short-circuit the
// backend roundtrip. On a stale hit it serves stale and fires a background
// revalidation. On a miss it stores the computed key in the state bag so
// Response() can populate the cache.
func (f *cacheFilter) Request(ctx filters.FilterContext) {
	baseKey := cacheKey(ctx.Request(), f.keyHeaders)
	key := baseKey
	if sentinel, _ := f.storage.Get(ctx.Request().Context(), "vary:"+baseKey); sentinel != nil && len(sentinel.VaryHeaders) > 0 {
		key = varyKey(baseKey, ctx.Request(), sentinel.VaryHeaders)
	}
	ctx.StateBag()[stateBagKey] = key
	ctx.StateBag()["cache:base-key"] = baseKey

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
		f.serveEntry(ctx, key, entry)
		return
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

	f.serveEntry(ctx, key, entry)
}

// serveEntry serves entry from cache, handling the stale and fresh cases.
func (f *cacheFilter) serveEntry(ctx filters.FilterContext, key string, entry *Entry) {
	rsp := &http.Response{
		StatusCode: entry.StatusCode,
		Header:     entry.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(entry.Payload)),
	}

	if entry.IsStale(time.Now()) {
		d := parseCacheControl(entry.Header)
		applySMaxAge(entry.TTL, &d)
		if d.MustRevalidate {
			f.coalesce(ctx, key)
			return
		}
		rsp.Header.Set("X-Cache-Status", "STALE")
		setAgeHeader(rsp, entry, time.Now())
		ctx.Metrics().IncCounter("stale")
		ctx.Serve(rsp)
		go f.revalidate(key, ctx.Request(), ctx.Metrics())
		return
	}

	rsp.Header.Set("X-Cache-Status", "HIT")
	setAgeHeader(rsp, entry, time.Now())
	ctx.Metrics().IncCounter("hit")
	ctx.Serve(rsp)
}

// coalesce gates concurrent cold misses for the same key behind a single
// upstream fetch. All waiters block until the leader's fetch completes, then
// all are served the same response. This prevents the thundering herd on a
// cache miss.
func (f *cacheFilter) coalesce(ctx filters.FilterContext, key string) {
	req := ctx.Request().Clone(context.Background())

	ch := f.sf.DoChan(key, func() (interface{}, error) {
		resp, err := f.fetch(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		ttl := capTTLByExpires(f.ttlForStatus(resp.StatusCode), resp.Header)
		directives := parseCacheControl(resp.Header)
		ttl = applySMaxAge(ttl, &directives)
		swr := f.swrWindow
		if resp.StatusCode != http.StatusOK {
			swr = 0
		}
		entry := &Entry{
			StatusCode:           resp.StatusCode,
			Header:               resp.Header.Clone(),
			Payload:              body,
			CreatedAt:            time.Now(),
			TTL:                  ttl,
			StaleWhileRevalidate: swr,
			ETag:                 resp.Header.Get("ETag"),
			LastModified:         resp.Header.Get("Last-Modified"),
		}
		if ttl > 0 && !directives.NoStore && !directives.Private && !directives.NoCache {
			_ = f.storage.Set(context.Background(), key, entry)
		}
		return entry, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			ctx.Metrics().IncCounter("fetch_error")
			return
		}
		if res.Val == nil {
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
		ctx.Serve(rsp)
	case <-ctx.Request().Context().Done():
		// Client disconnected. Remaining waiters still receive their result.
	}
}

// Response populates the cache on a miss.
func (f *cacheFilter) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()
	key := ctx.StateBag()[stateBagKey].(string)

	// Hit or stale path: already served from cache in Request(); nothing to do.
	if ctx.Served() {
		return
	}

	// RFC 9111 §4.4: invalidate cached entry on successful unsafe method.
	if isUnsafeMethod(ctx.Request().Method) && rsp.StatusCode < 400 {
		_ = f.storage.Delete(ctx.Request().Context(), key)
		return
	}

	// Only cache responses to GET requests. HEAD/OPTIONS/CONNECT/TRACE must not be stored.
	if ctx.Request().Method != http.MethodGet {
		return
	}

	// RFC 9111 §3: MUST NOT store partial content (206) without Range support.
	if rsp.StatusCode == http.StatusPartialContent {
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

	ttl := capTTLByExpires(f.ttlForStatus(rsp.StatusCode), rsp.Header)
	directives := parseCacheControl(rsp.Header)
	ttl = applySMaxAge(ttl, &directives)
	if ttl == 0 {
		return
	}

	if directives.NoStore || directives.Private || directives.NoCache {
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

	// Resolve Vary-aware storage key.
	baseKey, _ := ctx.StateBag()["cache:base-key"].(string)
	if baseKey == "" {
		baseKey = key
	}
	varyNames := parseVaryNames(varyHeader)
	storeKey := key
	if len(varyNames) > 0 {
		storeKey = varyKey(baseKey, ctx.Request(), varyNames)
		sentinel := &Entry{
			CreatedAt:   time.Now(),
			TTL:         ttl,
			VaryHeaders: varyNames,
		}
		_ = f.storage.Set(ctx.Request().Context(), "vary:"+baseKey, sentinel)
	}

	swr := f.swrWindow
	if rsp.StatusCode != http.StatusOK {
		swr = 0
	}
	entry := &Entry{
		StatusCode:           rsp.StatusCode,
		Header:               rsp.Header.Clone(),
		Payload:              body,
		CreatedAt:            time.Now(),
		TTL:                  ttl,
		StaleWhileRevalidate: swr,
		ETag:                 rsp.Header.Get("ETag"),
		LastModified:         rsp.Header.Get("Last-Modified"),
		VaryHeaders:          varyNames,
	}
	_ = f.storage.Set(ctx.Request().Context(), storeKey, entry)
}

// revalidate fetches the upstream resource in the background and refreshes the
// cache entry. Uses singleflight to coalesce concurrent revalidations for the
// same key. Uses a detached context so the fetch outlives the current request.
func (f *cacheFilter) revalidate(key string, orig *http.Request, m filters.Metrics) {
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

		resp, err := f.fetch(req)
		if err != nil {
			m.IncCounter("reval_error")
			log.WithFields(log.Fields{
				"url": orig.URL.String(),
			}).WithError(err).Warn("cache: background revalidation fetch failed")
			return nil, nil
		}
		defer resp.Body.Close()

		var body []byte
		var responseHeader http.Header
		var statusCode int

		if resp.StatusCode == http.StatusNotModified {
			// Reuse stored payload and merge any updated headers from 304.
			if stored, serr := f.storage.Get(context.Background(), key); serr == nil && stored != nil {
				body = stored.Payload
				responseHeader = stored.Header.Clone()
				for k, vv := range resp.Header {
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
				m.IncCounter("reval_error")
				log.WithFields(log.Fields{
					"url": orig.URL.String(),
				}).WithError(rerr).Warn("cache: failed to read response body during background revalidation")
				return nil, nil
			}
			responseHeader = resp.Header.Clone()
			statusCode = resp.StatusCode
		}

		ttl := capTTLByExpires(f.ttlForStatus(statusCode), responseHeader)
		directives := parseCacheControl(responseHeader)
		ttl = applySMaxAge(ttl, &directives)
		if ttl == 0 {
			return nil, nil
		}

		if directives.NoStore || directives.Private || directives.NoCache {
			return nil, nil
		}

		entry := &Entry{
			StatusCode:           statusCode,
			Header:               responseHeader,
			Payload:              body,
			CreatedAt:            time.Now(),
			TTL:                  ttl,
			StaleWhileRevalidate: f.swrWindow,
			ETag:                 responseHeader.Get("ETag"),
			LastModified:         responseHeader.Get("Last-Modified"),
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

// cacheKey builds a deterministic, URL-safe cache key from the request.
// SHA-256 is used for uniform distribution and to satisfy the lru.go requirement
// that keys passed to ShardedByteLRU are pre-hashed by the caller.
// keyHeaders is an optional list of request header names whose values are folded
// into the key, allowing per-header cache isolation (e.g. "Authorization").
func cacheKey(r *http.Request, keyHeaders []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s://%s%s?%s", r.URL.Scheme, r.Host, r.URL.Path, r.URL.RawQuery)
	for _, name := range keyHeaders {
		fmt.Fprintf(h, "\n%s: %s", name, r.Header.Get(name))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// setAgeHeader computes and sets the Age header per RFC 9111 §5.1.
// If the stored response already carries an Age value from an upstream cache, the two are summed.
func setAgeHeader(rsp *http.Response, entry *Entry, now time.Time) {
	elapsed := int64(now.Sub(entry.CreatedAt).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	if existing := rsp.Header.Get("Age"); existing != "" {
		if v, err := strconv.ParseInt(existing, 10, 64); err == nil {
			elapsed += v
		}
	}
	rsp.Header.Set("Age", strconv.FormatInt(elapsed, 10))
}

// applySMaxAge returns the effective TTL after applying s-maxage (RFC 9111 §5.2.2.10).
// s-maxage overrides Expires and the operator TTL for shared caches.
// It also sets MustRevalidate on the directives struct because s-maxage implies proxy-revalidate.
func applySMaxAge(ttl time.Duration, d *cacheDirectives) time.Duration {
	if d.SMaxAge == nil {
		return ttl
	}
	d.MustRevalidate = true
	if *d.SMaxAge < ttl {
		return *d.SMaxAge
	}
	return ttl
}

func capTTLByExpires(ttl time.Duration, header http.Header) time.Duration {
	exp := header.Get("Expires")
	if exp == "" {
		return ttl
	}
	t, err := http.ParseTime(exp)
	if err != nil {
		log.WithFields(log.Fields{
			"expires": exp,
		}).Warn("cache: unparseable Expires header, ignoring")
		return ttl
	}
	remaining := time.Until(t)
	if remaining <= 0 {
		return 0
	}
	if remaining < ttl {
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
