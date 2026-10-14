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
		// PathPrefix
		{spec: NewPathPrefix(), args: []any{}},
		{spec: NewPathPrefix(), args: []any{"/foo", "/bar"}},
		{spec: NewPathPrefix(), args: []any{42}},
		// PathRegexpMatch
		{spec: NewPathRegexpMatch(), args: []any{}},
		{spec: NewPathRegexpMatch(), args: []any{"^/foo", "extra"}},
		{spec: NewPathRegexpMatch(), args: []any{42}},
		{spec: NewPathRegexpMatch(), args: []any{`\`}}, // invalid regexp
		// RawPathPrefix
		{spec: NewRawPathPrefix(), args: []any{}},
		{spec: NewRawPathPrefix(), args: []any{"/foo", "/bar"}},
		{spec: NewRawPathPrefix(), args: []any{42}},
		// RawPathRegexpMatch
		{spec: NewRawPathRegexpMatch(), args: []any{}},
		{spec: NewRawPathRegexpMatch(), args: []any{"^/foo", "extra"}},
		{spec: NewRawPathRegexpMatch(), args: []any{42}},
		{spec: NewRawPathRegexpMatch(), args: []any{`\`}}, // invalid regexp
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
		// PathPrefix: matches on URL.Path
		{spec: NewPathPrefix(), arg: "/foo", path: "/foo/bar", match: true},
		{spec: NewPathPrefix(), arg: "/foo/", path: "/foo/bar", match: true},
		{spec: NewPathPrefix(), arg: "/foo/bar", path: "/foo/bar", match: true},
		{spec: NewPathPrefix(), arg: "/foo/bar/", path: "/foo/bar", match: false},
		{spec: NewPathPrefix(), arg: "/baz", path: "/foo/bar", match: false},
		{spec: NewPathPrefix(), arg: "", path: "/foo/bar", match: true},
		{spec: NewPathPrefix(), arg: "//foo", path: "//foo/bar", match: true},
		{spec: NewPathPrefix(), arg: "/foo//", path: "/foo//bar", match: true},
		{spec: NewPathPrefix(), arg: "//foo", path: "/foo/bar", match: false},

		// PathRegexpMatch: matches on URL.Path
		{spec: NewPathRegexpMatch(), arg: "^/foo", path: "/foo/bar", match: true},
		{spec: NewPathRegexpMatch(), arg: "/bar$", path: "/foo/bar", match: true},
		{spec: NewPathRegexpMatch(), arg: "^/baz", path: "/foo/bar", match: false},
		{spec: NewPathRegexpMatch(), arg: "^/foo/bar$", path: "/foo/bar", match: true},
		{spec: NewPathRegexpMatch(), arg: "^//foo", path: "//foo/bar", match: true},
		{spec: NewPathRegexpMatch(), arg: "/foo//bar", path: "/foo//bar", match: true},
		{spec: NewPathRegexpMatch(), arg: "^//foo", path: "/foo/bar", match: false},
		// %2F in URL.Path is decoded to /, PathRegexpMatch sees /
		{spec: NewPathRegexpMatch(), arg: "%2F", path: "/foo/bar", match: false},

		// RawPathPrefix: matches on URL.RawPath, fallback to URL.Path
		{spec: NewRawPathPrefix(), arg: "/foo%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewRawPathPrefix(), arg: "/foo%2Fbar", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewRawPathPrefix(), arg: "/foo/", path: "/foo/bar", rawPath: "/foo%2Fbar", match: false},
		// RawPath empty: fall back to URL.Path
		{spec: NewRawPathPrefix(), arg: "/foo", path: "/foo/bar", match: true},
		{spec: NewRawPathPrefix(), arg: "/baz", path: "/foo/bar", match: false},
		// double slash preserved in URL.Path (no encoding, so RawPath is empty)
		{spec: NewRawPathPrefix(), arg: "//foo", path: "//foo/bar", match: true},
		// decoded path does not match encoded prefix — key differentiator
		{spec: NewRawPathPrefix(), arg: "/foo%2F", path: "/foo/bar", match: false},

		// RawPathRegexpMatch: matches on URL.RawPath, fallback to URL.Path
		{spec: NewRawPathRegexpMatch(), arg: "%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewRawPathRegexpMatch(), arg: "^/foo%2F", path: "/foo/bar", rawPath: "/foo%2Fbar", match: true},
		{spec: NewRawPathRegexpMatch(), arg: "^/foo/", path: "/foo/bar", rawPath: "/foo%2Fbar", match: false},
		// RawPath empty: fall back to URL.Path
		{spec: NewRawPathRegexpMatch(), arg: "^/foo", path: "/foo/bar", match: true},
		{spec: NewRawPathRegexpMatch(), arg: "%2F", path: "/foo/bar", match: false},
		// double slash with no encoding
		{spec: NewRawPathRegexpMatch(), arg: "^//foo", path: "//foo/bar", match: true},
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

func BenchmarkPathPrefix(b *testing.B) {
	s := NewPathPrefix()
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

func BenchmarkPathRegexpMatch(b *testing.B) {
	s := NewPathRegexpMatch()
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

func BenchmarkRawPathPrefix(b *testing.B) {
	s := NewRawPathPrefix()
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

func BenchmarkRawPathRegexpMatch(b *testing.B) {
	s := NewRawPathRegexpMatch()
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
