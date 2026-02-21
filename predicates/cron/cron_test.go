package cron

import (
	"testing"
	"time"

	"github.com/zalando/skipper/predicates"
)

func TestCreate(t *testing.T) {
	testCases := []struct {
		msg     string
		args    []any
		isError bool
	}{
		{
			"wrong number of arguments",
			nil,
			true,
		},
		{
			"wrong number of arguments",
			[]any{"* * * * *", "unexpected argument"},
			true,
		},
		{
			"argument with mismatched type",
			[]any{1},
			true,
		},
		{
			"invalid cron-like expression",
			[]any{"a * * * *"},
			true,
		},
		{
			"valid arguments",
			[]any{"* * * * *"},
			false,
		},
	}

	for _, tc := range testCases {
		_, err := New().Create(tc.args)

		if err == nil && tc.isError {
			t.Errorf("expected an error and got none for test case [%s]", tc.msg)
		} else if err != nil && !tc.isError {
			t.Errorf("expected no error and got %v for test case [%s]", err, tc.msg)
		}
	}
}

func TestPredicateName(t *testing.T) {
	if name := New().Name(); name != predicates.CronName {
		t.Errorf("predicate name does not match expectation: %s", name)
	}
}

func TestPredicateMatch(t *testing.T) {
	testCases := []struct {
		msg     string
		args    []any
		matches bool
		clock   clock
	}{
		{
			"match everything",
			[]any{"* * * * *"},
			true,
			time.Now,
		},
	}

	for _, tc := range testCases {
		p, err := New().Create(tc.args)
		if err != nil {
			t.Error(err)
			continue
		}

		if got := p.Match(nil); got != tc.matches {
			t.Errorf("expected %t and got %t for the predicate on test case: %s", tc.matches, got, tc.msg)
		}
	}
}
