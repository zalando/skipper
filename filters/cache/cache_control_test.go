package cache

import (
	"net/http"
	"testing"
)

func TestParseRequestCacheControl(t *testing.T) {
	cases := []struct {
		header       string
		noStore      bool
		noCache      bool
		onlyIfCached bool
	}{
		{"no-store", true, false, false},
		{"no-cache", false, true, false},
		{"max-age=0", false, true, false},
		{"only-if-cached", false, false, true},
		{"no-cache, no-store", true, true, false},
		{"max-stale=60", false, false, false},
		{"", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Cache-Control", tc.header)
			}
			d := parseRequestCacheControl(h)
			if d.NoStore != tc.noStore {
				t.Errorf("NoStore: want %v got %v", tc.noStore, d.NoStore)
			}
			if d.NoCache != tc.noCache {
				t.Errorf("NoCache: want %v got %v", tc.noCache, d.NoCache)
			}
			if d.OnlyIfCached != tc.onlyIfCached {
				t.Errorf("OnlyIfCached: want %v got %v", tc.onlyIfCached, d.OnlyIfCached)
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
		{"no-store", http.Header{"Cache-Control": {"no-store"}}, cacheDirectives{NoStore: true, MaxAge: -1, SMaxAge: -1}},
		{"no-cache", http.Header{"Cache-Control": {"no-cache"}}, cacheDirectives{NoCache: true, MaxAge: -1, SMaxAge: -1}},
		{"private", http.Header{"Cache-Control": {"private"}}, cacheDirectives{Private: true, MaxAge: -1, SMaxAge: -1}},
		{"must-revalidate", http.Header{"Cache-Control": {"must-revalidate"}}, cacheDirectives{MustRevalidate: true, MaxAge: -1, SMaxAge: -1}},
		{"comma-separated", http.Header{"Cache-Control": {"no-store, must-revalidate"}}, cacheDirectives{NoStore: true, MustRevalidate: true, MaxAge: -1, SMaxAge: -1}},
		{"multiple lines", http.Header{"Cache-Control": {"no-cache", "must-revalidate"}}, cacheDirectives{NoCache: true, MustRevalidate: true, MaxAge: -1, SMaxAge: -1}},
		{"case-insensitive", http.Header{"Cache-Control": {"NO-STORE"}}, cacheDirectives{NoStore: true, MaxAge: -1, SMaxAge: -1}},
		{"value suffix stripped", http.Header{"Cache-Control": {`no-cache="x-private"`}}, cacheDirectives{NoCache: true, MaxAge: -1, SMaxAge: -1}},
		{"empty", http.Header{}, cacheDirectives{MaxAge: -1, SMaxAge: -1}},
		{"max-age=3600", http.Header{"Cache-Control": {"max-age=3600"}}, cacheDirectives{MaxAge: 3600, SMaxAge: -1}},
		{"s-maxage=60", http.Header{"Cache-Control": {"s-maxage=60"}}, cacheDirectives{MaxAge: -1, SMaxAge: 60}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCacheControl(tc.header); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
