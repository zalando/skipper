package cache

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

type cacheDirectives struct {
	NoStore         bool
	NoCache         bool
	Private         bool
	MustRevalidate  bool
	ProxyRevalidate bool
	Public          bool
	MaxAge          int64 // -1 = not present; 0 means max-age=0
	SMaxAge         int64 // -1 = not present
}

type requestCacheDirectives struct {
	NoStore      bool
	NoCache      bool // includes max-age=0
	OnlyIfCached bool
	MaxStale     int64 // -1 = not present; >= 0 = max staleness seconds client accepts
	MinFresh     int64 // -1 = not present; >= 0 = min remaining freshness seconds required
}

// parseRequestCacheControl parses Cache-Control request directives from h.
func parseRequestCacheControl(h http.Header) requestCacheDirectives {
	d := requestCacheDirectives{MaxStale: -1, MinFresh: -1}
	for _, line := range h.Values("Cache-Control") {
		for token := range strings.SplitSeq(line, ",") {
			parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
			directive := strings.ToLower(strings.TrimSpace(parts[0]))
			switch directive {
			case "no-store":
				d.NoStore = true
			case "no-cache":
				d.NoCache = true
			case "max-age":
				if len(parts) == 2 && strings.TrimSpace(parts[1]) == "0" {
					d.NoCache = true
				}
			case "only-if-cached":
				d.OnlyIfCached = true
			case "max-stale":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && v >= 0 {
						d.MaxStale = v
					}
				} else {
					d.MaxStale = math.MaxInt32 // ~68 years; safe upper bound for *time.Second conversion
				}
			case "min-fresh":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && v >= 0 {
						d.MinFresh = v
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
	d := cacheDirectives{MaxAge: -1, SMaxAge: -1}
	for _, line := range h.Values("Cache-Control") {
		for token := range strings.SplitSeq(line, ",") {
			parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
			directive := strings.ToLower(strings.TrimSpace(parts[0]))
			switch directive {
			case "no-store":
				d.NoStore = true
			case "no-cache":
				d.NoCache = true
			case "private":
				d.Private = true
			case "must-revalidate":
				d.MustRevalidate = true
			case "proxy-revalidate":
				d.ProxyRevalidate = true
			case "public":
				d.Public = true
			case "max-age":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						d.MaxAge = v
					}
				}
			case "s-maxage":
				if len(parts) == 2 {
					if v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						d.SMaxAge = v
					}
				}
			}
		}
	}
	return d
}
