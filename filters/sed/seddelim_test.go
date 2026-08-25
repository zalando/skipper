package sed_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/zalando/skipper/filters"
)

func TestSedDelim(t *testing.T) {
	args := func(a ...any) []any { return a }
	for _, test := range []testItem{{
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
		title:  "expand the body to make it longer",
		args:   args("foo", "foobarbaz", "\n"),
		body:   "foobarbaz\nfoobarbaz\nfoobarbaz",
		expect: "foobarbazbarbaz\nfoobarbazbarbaz\nfoobarbazbarbaz",
	}} {
		t.Run(
			fmt.Sprintf("%s/%s", filters.SedRequestDelimName, test.title),
			testRequest(filters.SedRequestDelimName, test),
		)

		t.Run(fmt.Sprintf("%s/%s", filters.SedDelimName, test.title), testResponse(filters.SedDelimName, test))
	}
}

func TestSedDelimNoDelim(t *testing.T) {
	args := func(a ...any) []any { return a }
	for _, test := range []testItem{{
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
		title:  "expand the body to make it longer",
		args:   args("foo", "foobarbaz", "\n"),
		body:   "foobarbazfoobarbazfoobarbaz",
		expect: "foobarbazbarbazfoobarbazbarbazfoobarbazbarbaz",
	}} {
		t.Run(
			fmt.Sprintf("%s/%s", filters.SedRequestDelimName, test.title),
			testRequest(filters.SedRequestDelimName, test),
		)

		t.Run(fmt.Sprintf("%s/%s", filters.SedDelimName, test.title), testResponse(filters.SedDelimName, test))
	}
}

func TestSedDelimLongStream(t *testing.T) {
	const (
		inputString  = "f"
		pattern      = inputString + "*"
		outputString = "qux"
		bodySize     = 1 << 15
	)

	createBody := func() io.Reader {
		b := bytes.NewBuffer(nil)
		for b.Len() < bodySize {
			b.WriteString(inputString)
		}

		return b
	}

	baseArgs := []any{pattern, outputString, "\n"}

	t.Run("below max buffer size", testResponse(filters.SedDelimName, testItem{
		args:       append(baseArgs, bodySize*2),
		bodyReader: createBody(),
		expect:     "qux",
	}))

	t.Run("above max buffer size, abort", testResponse(filters.SedDelimName, testItem{
		args:       append(baseArgs, bodySize/2, "abort"),
		bodyReader: createBody(),
	}))

	t.Run("above max buffer size, best effort", testResponse(filters.SedDelimName, testItem{
		args:       append(baseArgs, bodySize/2),
		bodyReader: createBody(),
		expect:     "quxqux",
	}))
}
