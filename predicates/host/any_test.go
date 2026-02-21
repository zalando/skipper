package host

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHostAnyArgs(t *testing.T) {
	s := NewAny()
	for _, tc := range []struct {
		args []any
	}{
		{
			args: []any{},
		},
		{
			args: []any{1.2},
		},
		{
			args: []any{"example.org", 3.4},
		},
	} {
		if _, err := s.Create(tc.args); err == nil {
			t.Errorf("expected error for arguments: %v", tc.args)
		}
	}
}

func TestHostAnyMatch(t *testing.T) {
	s := NewAny()
	for _, tc := range []struct {
		host  string
		args  []any
		match bool
	}{
		{
			host:  "example.org",
			args:  []any{"example.com"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []any{"example.com", "example.net"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []any{"www.example.org"},
			match: false,
		},
		{
			host:  "www.example.org",
			args:  []any{"example.org"},
			match: false,
		},
		{
			host:  "example.org.",
			args:  []any{"example.org"},
			match: false,
		},
		{
			host:  "example.org:8080",
			args:  []any{"example.org"},
			match: false,
		},
		{
			host:  "example.org.:8080",
			args:  []any{"example.org"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []any{"example.org"},
			match: true,
		},
		{
			host:  "example.org:8080",
			args:  []any{"example.org:8080"},
			match: true,
		},
		{
			host:  "example.org",
			args:  []any{"example.org", "example.com"},
			match: true,
		},
		{
			host:  "example.org",
			args:  []any{"example.com", "example.org"},
			match: true,
		},
		{
			host:  "example.org:8080",
			args:  []any{"example.org", "example.org:8080"},
			match: true,
		},
	} {
		t.Run(fmt.Sprintf("%s->%v", tc.host, tc.args), func(t *testing.T) {
			p, err := s.Create(tc.args)
			if err != nil {
				t.Error(err)
			}
			if p.Match(&http.Request{Host: tc.host}) != tc.match {
				t.Errorf("expected match: %v", tc.match)
			}
		})
	}
}

var matchSink bool

func BenchmarkHostAny(b *testing.B) {
	for _, n := range []int{1, 2, 5, 10, 20, 50, 100} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			s := NewAny()
			args := make([]any, n)
			for i := range n {
				args[i] = fmt.Sprintf("example%d.org", i)
			}
			p, err := s.Create(args)
			if err != nil {
				b.Fatal(err)
			}

			req := &http.Request{Host: args[len(args)/2].(string)}
			matchSink = p.Match(req)
			if !matchSink {
				b.Fatal("expected to match")
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				matchSink = p.Match(req)
			}
		})
	}
}
