package auth

import (
	"net/http"
	"testing"
)

func TestHeaderSHA256Args(t *testing.T) {
	s := NewHeaderSHA256()
	for _, tc := range []struct {
		args []any
	}{
		{
			args: []any{},
		},
		{
			args: []any{"X-Secret", "xyz"},
		},
		{
			args: []any{"X-Secret", "00112233"},
		},
		{
			args: []any{"X-Secret", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", "AA"},
		},
	} {
		if _, err := s.Create(tc.args); err == nil {
			t.Errorf("expected error for arguments: %v", tc.args)
		}
	}
}

func TestHeaderSHA256Match(t *testing.T) {
	s := NewHeaderSHA256()
	p, err := s.Create([]any{
		"X-Secret",
		"2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b", // "secret"
		"5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8", // "password"
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		header http.Header
		match  bool
	}{
		{
			header: http.Header{
				"X-Test": []string{"foo"},
			},
			match: false,
		},
		{
			header: http.Header{
				"X-Secret": []string{"foo"},
			},
			match: false,
		},
		{
			header: http.Header{
				"X-Secret": []string{"SECRET"},
			},
			match: false,
		},
		{
			header: http.Header{
				"X-Secret": []string{"PASSWORD"},
			},
			match: false,
		},
		{
			header: http.Header{
				"X-Secret": []string{"secret"},
			},
			match: true,
		},
		{
			header: http.Header{
				"X-Secret": []string{"password"},
			},
			match: true,
		},
	} {
		if p.Match(&http.Request{Header: tc.header}) != tc.match {
			t.Errorf("expected match: %v", tc.match)
		}
	}
}

func BenchmarkHeaderSHA256Match(b *testing.B) {
	s := NewHeaderSHA256()
	p, err := s.Create([]any{"X-Secret", "2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b"})
	if err != nil {
		b.Fatal(err)
	}
	r := &http.Request{Header: http.Header{"X-Secret": []string{"secret"}}}
	if !p.Match(r) {
		b.Fatal("match expected")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Match(r)
	}
}
