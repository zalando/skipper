package tee

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/predicates"
)

func TestTee(t *testing.T) {
	p := New()

	for _, tc := range []struct {
		name  string
		args  []any
		match bool
		err   error
	}{
		{
			name: "no args",
			args: []any{},
			err:  predicates.ErrInvalidPredicateParameters,
		},
		{
			name: "wrong args",
			args: []any{1.2},
			err:  predicates.ErrInvalidPredicateParameters,
		},
		{
			name:  "match",
			args:  []any{"goodmatch"},
			match: true,
		},
		{
			name:  "no match",
			args:  []any{"nomatch"},
			match: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := p.Create(tc.args)
			if err != nil {
				if err == tc.err {
					return
				}
				t.Fatalf("Failed with not expected err want: %v, got: %v", tc.err, err)
			} else {
				if tc.err != nil {
					t.Fatalf("Failed to get expected error: %v", tc.err)
				}
			}
			req, _ := http.NewRequest("GET", "http://my-app.test", nil)
			req.Header.Add(HeaderKey, "goodmatch")
			res := pred.Match(req)
			if res != tc.match {
				t.Fatalf("Failed to get expected result, want %v, got %v", tc.match, res)
			}

			if p.Name() == "" {
				t.Fatal("predicate name is empty")
			}
		})
	}

}
