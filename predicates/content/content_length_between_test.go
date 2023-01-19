package content

import (
	"fmt"
	"net/http"
	"testing"
)

func TestContentLengthMatch(t *testing.T) {
	s := NewContentLengthBetween()
	for _, tc := range []struct {
		minLength int64
		maxLength int64
		args      []interface{}
		match     bool
	}{
		{
			minLength: 0,
			maxLength: 5000,
			args:      []interface{}{3000},
			match:     true,
		},
		{
			minLength: 0,
			maxLength: 5000,
			args:      []interface{}{6000},
			match:     false,
		},
		{
			minLength: 5000,
			maxLength: 50000,
			args:      []interface{}{30000},
			match:     true,
		},
		{
			minLength: 50000,
			maxLength: 5000000,
			args:      []interface{}{300000},
			match:     true,
		},
		{
			minLength: 0,
			maxLength: 5000,
			args:      []interface{}{-1},
			match:     false,
		},
	} {
		t.Run(fmt.Sprintf("predicate %v - %v, match %s", tc.minLength, tc.maxLength, tc.args),
			func(t *testing.T) {
				p, err := s.Create(tc.args)
				if err != nil {
					t.Error(err)
				}
				if p.Match(&http.Request{ContentLength: tc.maxLength}) != tc.match {
					t.Errorf("expected match: %v", tc.match)
				}
			})
	}
}
