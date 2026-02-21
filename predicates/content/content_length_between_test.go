package content

import (
	"fmt"
	"net/http"
	"testing"
)

func TestContentLengthCreate(t *testing.T) {
	s := NewContentLengthBetween()

	_, err := s.Create([]any{1000.0, 1000.0})

	if err == nil {
		t.Fatal("expected error here, lower bound equals upper bound")
	}
}

func TestContentLengthMatch(t *testing.T) {
	s := NewContentLengthBetween()
	for _, tc := range []struct {
		length int64
		args   []any
		match  bool
	}{
		{
			length: 3000,
			args:   []any{0.0, 5000.0},
			match:  true,
		},
		{
			length: 6000,
			args:   []any{0.0, 5000.0},
			match:  false,
		},
		{
			length: 30000,
			args:   []any{5000.0, 50000.0},
			match:  true,
		},
		{
			length: 300000,
			args:   []any{50000.0, 5000000.0},
			match:  true,
		},
		{
			length: -1,
			args:   []any{0.0, 5000.0},
			match:  false,
		},
		{
			length: 999,
			args:   []any{0.0, 1000.0},
			match:  true,
		},
		{
			length: 1000,
			args:   []any{0.0, 1000.0},
			match:  false,
		},
	} {
		t.Run(fmt.Sprintf("predicate %v match %v", tc.args, tc.length),
			func(t *testing.T) {
				p, err := s.Create(tc.args)
				if err != nil {
					t.Fatal(err)
				}

				if p.Match(&http.Request{ContentLength: tc.length}) != tc.match {
					t.Errorf("expected match: %v", tc.match)
				}
			})
	}
}
