package cache

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

type cacheDirectives struct {
	noStore         bool
	noCache         bool
	private         bool
	mustRevalidate  bool
	proxyRevalidate bool
	public          bool
	maxAge          int64 // -1 = not present; 0 means max-age=0
	sMaxAge         int64 // -1 = not present
}

type requestCacheDirectives struct {
	noStore      bool
	noCache      bool // includes max-age=0
	onlyIfCached bool
	maxStale     int64 // -1 = not present; >= 0 = max staleness seconds client accepts
	minFresh     int64 // -1 = not present; >= 0 = min remaining freshness seconds required
}

// parseRequestCacheControl parses Cache-Control request directives from h.
func parseRequestCacheControl(h http.Header) requestCacheDirectives {
	d := requestCacheDirectives{maxStale: -1, minFresh: -1}
	for _, line := range h.Values("Cache-Control") {
		for token := range strings.SplitSeq(line, ",") {
			parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
			directive := strings.ToLower(strings.TrimSpace(parts[0]))
			switch directive {
			case "no-store":
				d.noStore = true
			case "no-cache":
				d.noCache = true
			case "max-age":
				if len(parts) == 2 && strings.TrimSpace(parts[1]) == "0" {
					d.noCache = true
				}
			case "only-if-cached":
				d.onlyIfCached = true
			case "max-stale":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && v >= 0 {
						d.maxStale = v
					}
				} else {
					d.maxStale = math.MaxInt32 // ~68 years; safe upper bound for *time.Second conversion
				}
			case "min-fresh":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && v >= 0 {
						d.minFresh = v
					}
				}
			}
		}
	}
	return d
}

// parseCacheControl parses Cache-Control response directives from h.
// Uses Header.Values to handle multiple header lines; matches names
// case-insensitively per RFC 9111 §5.2.
func parseCacheControl(h http.Header) cacheDirectives {
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	for _, line := range h.Values("Cache-Control") {
		for token := range strings.SplitSeq(line, ",") {
			parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
			directive := strings.ToLower(strings.TrimSpace(parts[0]))
			switch directive {
			case "no-store":
				d.noStore = true
			case "no-cache":
				d.noCache = true
			case "private":
				d.private = true
			case "must-revalidate":
				d.mustRevalidate = true
			case "proxy-revalidate":
				d.proxyRevalidate = true
			case "public":
				d.public = true
			case "max-age":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						d.maxAge = v
					}
				}
			case "s-maxage":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						d.sMaxAge = v
					}
				}
			}
		}
	}
	return d
}
