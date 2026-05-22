package cache

import (
	"math"
	"net/http"
	"testing"
)

func TestParseRequestCacheControl(t *testing.T) {
	cases := []struct {
		header       string
		noStore      bool
		noCache      bool
		onlyIfCached bool
		maxStale     int64
		minFresh     int64
	}{
		{"no-store", true, false, false, -1, -1},
		{"no-cache", false, true, false, -1, -1},
		{"max-age=0", false, true, false, -1, -1},
		{"only-if-cached", false, false, true, -1, -1},
		{"no-cache, no-store", true, true, false, -1, -1},
		{"max-stale=60", false, false, false, 60, -1},
		{"max-stale", false, false, false, math.MaxInt32, -1},
		{"max-stale=3.4", false, false, false, -1, -1}, // malformed: ParseInt fails, sentinel unchanged
		{"min-fresh=30", false, false, false, -1, 30},
		{"max-age=0,max-age=5", false, true, false, -1, -1}, // =0 sets noCache; =5 ignored (only =0 handled)
		{"", false, false, false, -1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Cache-Control", tc.header)
			}
			d := parseRequestCacheControl(h)
			if d.noStore != tc.noStore {
				t.Errorf("noStore: want %v got %v", tc.noStore, d.noStore)
			}
			if d.noCache != tc.noCache {
				t.Errorf("noCache: want %v got %v", tc.noCache, d.noCache)
			}
			if d.onlyIfCached != tc.onlyIfCached {
				t.Errorf("onlyIfCached: want %v got %v", tc.onlyIfCached, d.onlyIfCached)
			}
			if d.maxStale != tc.maxStale {
				t.Errorf("maxStale: want %v got %v", tc.maxStale, d.maxStale)
			}
			if d.minFresh != tc.minFresh {
				t.Errorf("minFresh: want %v got %v", tc.minFresh, d.minFresh)
			}
		})
	}
}

func TestParseCacheControl(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
		want   cacheDirectives
	}{
		{"no-store", http.Header{"Cache-Control": {"no-store"}}, cacheDirectives{noStore: true, maxAge: -1, sMaxAge: -1}},
		{"no-cache", http.Header{"Cache-Control": {"no-cache"}}, cacheDirectives{noCache: true, maxAge: -1, sMaxAge: -1}},
		{"private", http.Header{"Cache-Control": {"private"}}, cacheDirectives{private: true, maxAge: -1, sMaxAge: -1}},
		{"must-revalidate", http.Header{"Cache-Control": {"must-revalidate"}}, cacheDirectives{mustRevalidate: true, maxAge: -1, sMaxAge: -1}},
		{"comma-separated", http.Header{"Cache-Control": {"no-store, must-revalidate"}}, cacheDirectives{noStore: true, mustRevalidate: true, maxAge: -1, sMaxAge: -1}},
		{"multiple lines", http.Header{"Cache-Control": {"no-cache", "must-revalidate"}}, cacheDirectives{noCache: true, mustRevalidate: true, maxAge: -1, sMaxAge: -1}},
		{"case-insensitive", http.Header{"Cache-Control": {"NO-STORE"}}, cacheDirectives{noStore: true, maxAge: -1, sMaxAge: -1}},
		{"value suffix stripped", http.Header{"Cache-Control": {`no-cache="x-private"`}}, cacheDirectives{noCache: true, maxAge: -1, sMaxAge: -1}},
		{"empty", http.Header{}, cacheDirectives{maxAge: -1, sMaxAge: -1}},
		{"max-age=3600", http.Header{"Cache-Control": {"max-age=3600"}}, cacheDirectives{maxAge: 3600, sMaxAge: -1}},
		{"s-maxage=60", http.Header{"Cache-Control": {"s-maxage=60"}}, cacheDirectives{maxAge: -1, sMaxAge: 60}},
		{"max-age=3.4", http.Header{"Cache-Control": {"max-age=3.4"}}, cacheDirectives{maxAge: -1, sMaxAge: -1}}, // malformed: ParseInt fails, sentinel unchanged
		{"max-age=0,max-age=5", http.Header{"Cache-Control": {"max-age=0, max-age=5"}}, cacheDirectives{maxAge: 5, sMaxAge: -1}}, // last-write-wins; no duplicate guard
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCacheControl(tc.header); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
