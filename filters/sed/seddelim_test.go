package sed_test

import (
	"fmt"
	"testing"

	"github.com/zalando/skipper/filters/sed"
)

func TestSedDelim(t *testing.T) {
	args := func(a ...interface{}) []interface{} { return a }
	for _, test := range []struct {
		title           string
		args            []interface{}
		body            string
		expect          string
		forceReadBuffer int
	}{{
		title:  "no match",
		args:   args("foo", "bar", "\n"),
		body:   "barbaz\nquxquux",
		expect: "barbaz\nquxquux",
	}, {
		title:  "not consumable match",
		args:   args("[0-9]*", "this was a number", "\n"),
		body:   "foobar\nbazqux",
		expect: "foobar\nbazqux",
	}, {
		title:  "has match",
		args:   args("foo", "bar", "\n"),
		body:   "foobarbazqux\nfoobarbazqux",
		expect: "barbarbazqux\nbarbarbazqux",
	}, {
		title:  "has match, mid-line",
		args:   args("bar", "baz", "\n"),
		body:   "foobarbazqux\nfoobarbazqux",
		expect: "foobazbazqux\nfoobazbazqux",
	}, {
		title:  "whole lines are replaced",
		args:   args("foo", "bar", "\n"),
		body:   "foofoo\nfoofoo",
		expect: "barbar\nbarbar",
	}, {
		title:  "every line is cleared",
		args:   args("foo", "", "\n"),
		body:   "foofoo\nfoofoo",
		expect: "\n",
	}, {
		title:  "consume and discard",
		args:   args("(?s).*", "", "\n"),
		body:   "foobar\nbazqux",
		expect: "",
	}, {
		title:  "replace delimiter",
		args:   args("foo\n", "bar", "\n"),
		body:   "foo\nbar",
		expect: "barbar",
	}, {
		title:  "replace delimiter only",
		args:   args("\n", "bar", "\n"),
		body:   "foo\nbaz",
		expect: "foobarbaz",
	}, {
		title:  "capture groups are ignored but ok",
		args:   args("foo(bar)", "qux", "\n"),
		body:   "foobar\nbazqux",
		expect: "qux\nbazqux",
	}, {
		title:  "default max buffer",
		args:   args("foo", "bar", "\n"),
		body:   "foobarbaz",
		expect: "barbarbaz",
	}, {
		title:  "small max buffer",
		args:   args("a", "X", "\n", 1),
		body:   "foobarbaz",
		expect: "foobXrbXz",
	}} {
		t.Run(
			fmt.Sprintf("%s/%s", sed.NameRequestDelimit, test.title),
			testRequest(sed.NameRequestDelimit, test),
		)

		t.Run(fmt.Sprintf("%s/%s", sed.NameDelimit, test.title), testResponse(sed.NameDelimit, test))
	}
}

func TestSedDelimNoDelim(t *testing.T) {
	args := func(a ...interface{}) []interface{} { return a }
	for _, test := range []struct {
		title           string
		args            []interface{}
		body            string
		expect          string
		forceReadBuffer int
	}{{
		title: "empty body",
		args:  args("foo", "bar", "\n"),
	}, {
		title:  "no match",
		args:   args("foo", "bar", "\n"),
		body:   "barbazqux",
		expect: "barbazqux",
	}, {
		title:  "not consumable match",
		args:   args("[0-9]*", "this was a number", "\n"),
		body:   "foobarbaz",
		expect: "foobarbaz",
	}, {
		title:  "has match",
		args:   args("foo", "bar", "\n"),
		body:   "foobarbazquxfoobarbazqux",
		expect: "barbarbazquxbarbarbazqux",
	}, {
		title:  "the whole body is replaced",
		args:   args("foo", "bar", "\n"),
		body:   "foofoofoo",
		expect: "barbarbar",
	}, {
		title:  "the whole body is deleted",
		args:   args("foo", "", "\n"),
		body:   "foofoofoo",
		expect: "",
	}, {
		title:  "consume and discard",
		args:   args(".*", "", "\n"),
		body:   "foobarbaz",
		expect: "",
	}, {
		title:  "capture groups are ignored but ok",
		args:   args("foo(bar)baz", "qux", "\n"),
		body:   "foobarbaz",
		expect: "qux",
	}, {
		title:  "default max buffer",
		args:   args("foo", "bar", "\n"),
		body:   "foobarbaz",
		expect: "barbarbaz",
	}, {
		title:  "small max buffer",
		args:   args("a", "X", "\n", 1),
		body:   "foobarbaz",
		expect: "foobXrbXz",
	}} {
		t.Run(
			fmt.Sprintf("%s/%s", sed.NameRequestDelimit, test.title),
			testRequest(sed.NameRequestDelimit, test),
		)

		t.Run(fmt.Sprintf("%s/%s", sed.NameDelimit, test.title), testResponse(sed.NameDelimit, test))
	}
}
