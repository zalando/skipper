package cache

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type cacheDirectives struct {
	NoStore        bool
	NoCache        bool
	Private        bool
	MustRevalidate bool
	Public         bool
	SMaxAge        *time.Duration // RFC 9111 §5.2.2.10; nil means not present
}

type requestCacheDirectives struct {
	NoStore      bool
	NoCache      bool // includes max-age=0
	OnlyIfCached bool
}

func parseRequestCacheControl(h http.Header) requestCacheDirectives {
	var d requestCacheDirectives
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
			}
		}
	}
	return d
}

// parseCacheControl parses Cache-Control response directives from h.
// Uses Header.Values to handle multiple header lines; matches names
// case-insensitively per RFC 7234 §5.2.
func parseCacheControl(h http.Header) cacheDirectives {
	var d cacheDirectives
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
			case "public":
				d.Public = true
			case "s-maxage":
				if len(parts) == 2 {
					if secs, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && secs >= 0 {
						v := time.Duration(secs) * time.Second
						d.SMaxAge = &v
					}
				}
			}
		}
	}
	return d
}
