package path

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/zalando/skipper/routing"
)

func TestArgs(t *testing.T) {
	for _, tc := range []struct {
		spec routing.PredicateSpec
		args []any
	}{
		// UrlPathPrefix
		{spec: NewUrlPathPrefix(), args: []any{}},
		{spec: NewUrlPathPrefix(), args: []any{"/foo", "/bar"}},
		{spec: NewUrlPathPrefix(), args: []any{42}},
		// UrlPathRegexp
		{spec: NewUrlPathRegexp(), args: []any{}},
		{spec: NewUrlPathRegexp(), args: []any{"^/foo", "extra"}},
		{spec: NewUrlPathRegexp(), args: []any{42}},
		{spec: NewUrlPathRegexp(), args: []any{`\`}}, // invalid regexp
		// UrlRawPathPrefix
		{spec: NewUrlRawPathPrefix(), args: []any{}},
		{spec: NewUrlRawPathPrefix(), args: []any{"/foo", "/bar"}},
		{spec: NewUrlRawPathPrefix(), args: []any{42}},
		// UrlRawPathRegexp
		{spec: NewUrlRawPathRegexp(), args: []any{}},
		{spec: NewUrlRawPathRegexp(), args: []any{"^/foo", "extra"}},
		{spec: NewUrlRawPathRegexp(), args: []any{42}},
		{spec: NewUrlRawPathRegexp(), args: []any{`\`}}, // invalid regexp
	} {
		t.Run(fmt.Sprintf("%s(%v)", tc.spec.Name(), tc.args), func(t *testing.T) {
			if _, err := tc.spec.Create(tc.args); err == nil {
				t.Errorf("expected error for args: %v", tc.args)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		spec    routing.PredicateSpec
		arg     string
		path    string
		rawPath string
		match   bool
	}{
		// UrlPathPrefix: matches on URL.Path
		{spec: NewUrlPathPrefix(), arg: "/foo", path: "/foo/bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "/foo/", path: "/foo/bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "/foo/bar", path: "/foo/bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "/foo/bar/", path: "/foo/bar", match: false},
		{spec: NewUrlPathPrefix(), arg: "/baz", path: "/foo/bar", match: false},
		{spec: NewUrlPathPrefix(), arg: "", path: "/foo/bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "//foo", path: "//foo/bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "/foo//", path: "/foo//bar", match: true},
		{spec: NewUrlPathPrefix(), arg: "//foo", path: "/foo/bar", match: false},

		// UrlPathRegexp: matches on URL.Path
		{spec: NewUrlPathRegexp(), arg: "^/foo", path: "/foo/bar", match: true},
		{spec: NewUrlPathRegexp(), arg: "/bar$", path: "/foo/bar", match: true},
		{spec: NewUrlPathRegexp(), arg: "^/baz", path: "/foo/bar", match: false},
		{spec: NewUrlPathRegexp(), arg: "^/foo/bar$", path: "/foo/bar", match: true},
		{spec: NewUrlPathRegexp(), arg: "^//foo", path: "//foo/bar", match: true},
		{spec: NewUrlPathRegexp(), arg: "/foo//bar", path: "/foo//bar", match: true},
		{spec: NewUrlPathRegexp(), arg: "^//foo", path: "/foo/bar", match: false},
		// %2F in URL.Path is decoded to /, UrlPathRegexp sees /
		{spec: NewUrlPathRegexp(), arg: "%2F", path: "/foo/bar", match: false},

		// UrlRawPathPrefix: matches on URL.RawPath, fallback to URL.Path
		{spec: NewUrlRawPathPrefix(), arg: "/foo%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewUrlRawPathPrefix(), arg: "/foo%2Fbar", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewUrlRawPathPrefix(), arg: "/foo/", path: "/foo/bar", rawPath: "/foo%2Fbar", match: false},
		// RawPath empty: fall back to URL.Path
		{spec: NewUrlRawPathPrefix(), arg: "/foo", path: "/foo/bar", match: true},
		{spec: NewUrlRawPathPrefix(), arg: "/baz", path: "/foo/bar", match: false},
		// double slash preserved in URL.Path (no encoding, so RawPath is empty)
		{spec: NewUrlRawPathPrefix(), arg: "//foo", path: "//foo/bar", match: true},
		// decoded path does not match encoded prefix — key differentiator
		{spec: NewUrlRawPathPrefix(), arg: "/foo%2F", path: "/foo/bar", match: false},

		// UrlRawPathRegexp: matches on URL.RawPath, fallback to URL.Path
		{spec: NewUrlRawPathRegexp(), arg: "%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewUrlRawPathRegexp(), arg: "^/foo%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewUrlRawPathRegexp(), arg: "^/foo/", path: "/foo/bar", rawPath: "/foo%2Fbar", match: false},
		// RawPath empty: fall back to URL.Path
		{spec: NewUrlRawPathRegexp(), arg: "^/foo", path: "/foo/bar", match: true},
		{spec: NewUrlRawPathRegexp(), arg: "%2F", path: "/foo/bar", match: false},
		// double slash with no encoding
		{spec: NewUrlRawPathRegexp(), arg: "^//foo", path: "//foo/bar", match: true},
	} {
		t.Run(fmt.Sprintf("%s(%q) path=%s raw=%s", tc.spec.Name(), tc.arg, tc.path, tc.rawPath), func(t *testing.T) {
			p, err := tc.spec.Create([]any{tc.arg})
			if err != nil {
				t.Fatal(err)
			}
			req := &http.Request{URL: &url.URL{Path: tc.path, RawPath: tc.rawPath}}
			if p.Match(req) != tc.match {
				t.Errorf("expected match=%v", tc.match)
			}
		})
	}
}

var matchSink bool

func BenchmarkUrlPathPrefix(b *testing.B) {
	s := NewUrlPathPrefix()
	p, err := s.Create([]any{"/api/v1"})
	if err != nil {
		b.Fatal(err)
	}
	req := &http.Request{URL: &url.URL{Path: "/api/v1/users"}}
	matchSink = p.Match(req)
	if !matchSink {
		b.Fatal("expected match")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchSink = p.Match(req)
	}
}

func BenchmarkUrlPathRegexp(b *testing.B) {
	s := NewUrlPathRegexp()
	p, err := s.Create([]any{"^/api/v1"})
	if err != nil {
		b.Fatal(err)
	}
	req := &http.Request{URL: &url.URL{Path: "/api/v1/users"}}
	matchSink = p.Match(req)
	if !matchSink {
		b.Fatal("expected match")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchSink = p.Match(req)
	}
}

func BenchmarkUrlRawPathPrefix(b *testing.B) {
	s := NewUrlRawPathPrefix()
	p, err := s.Create([]any{"/api%2Fv1"})
	if err != nil {
		b.Fatal(err)
	}
	req := &http.Request{URL: &url.URL{Path: "/api/v1/users", RawPath: "/api%2Fv1/users"}}
	matchSink = p.Match(req)
	if !matchSink {
		b.Fatal("expected match")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchSink = p.Match(req)
	}
}

func BenchmarkUrlRawPathRegexp(b *testing.B) {
	s := NewUrlRawPathRegexp()
	p, err := s.Create([]any{"^/api%2F"})
	if err != nil {
		b.Fatal(err)
	}
	req := &http.Request{URL: &url.URL{Path: "/api/v1/users", RawPath: "/api%2Fv1/users"}}
	matchSink = p.Match(req)
	if !matchSink {
		b.Fatal("expected match")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchSink = p.Match(req)
	}
}
