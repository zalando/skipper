package sed

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/zalando/skipper/filters"
)

func TestSedInit(t *testing.T) {
	args := func(a ...interface{}) []interface{} { return a }
	for _, test := range []struct {
		title  string
		spec   func() filters.Spec
		args   []interface{}
		fail   bool
		expect filter
	}{{
		title: "not enough args",
		args:  args("foo"),
		fail:  true,
	}, {
		title: "pattern not string",
		args:  args(3.14, "foo"),
		fail:  true,
	}, {
		title: "invalid regexp",
		args:  args("[", "bar"),
		fail:  true,
	}, {
		title: "replacement not string",
		args:  args("foo", 3.14),
		fail:  true,
	}, {
		title: "missing delimiter",
		spec:  NewDelimited,
		args:  args("foo", "bar"),
		fail:  true,
	}, {
		title: "delimited 5 args",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz", 1024, "qux"),
		fail:  true,
	}, {
		title: "delimiter not string",
		spec:  NewDelimited,
		args:  args("foo", "bar", 3.14, 1024),
		fail:  true,
	}, {
		title: "delimited max buf not a number",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz", "qux"),
		fail:  true,
	}, {
		title: "4 args",
		args:  args("foo", "bar", 1024, "baz"),
		fail:  true,
	}, {
		title: "max buf not a number",
		args:  args("foo", "bar", "baz"),
		fail:  true,
	}, {
		title: "sed",
		args:  args("foo", "bar"),
		expect: filter{
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar"),
		},
	}, {
		title: "sedDelim",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz"),
		expect: filter{
			typ:         delimited,
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar"),
			delimiter:   []byte("baz"),
		},
	}, {
		title: "sedRequest",
		spec:  NewRequest,
		args:  args("foo", "bar"),
		expect: filter{
			typ:         simpleRequest,
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar"),
		},
	}, {
		title: "sedRequestDelim",
		spec:  NewDelimitedRequest,
		args:  args("foo", "bar", "baz"),
		expect: filter{
			typ:         delimitedRequest,
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar"),
			delimiter:   []byte("baz"),
		},
	}, {
		title: "delimited max buf as float",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz", 1024.0),
		expect: filter{
			typ:             delimited,
			pattern:         regexp.MustCompile("foo"),
			replacement:     []byte("bar"),
			delimiter:       []byte("baz"),
			maxEditorBuffer: 1024,
		},
	}, {
		title: "delimited max buf as int",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz", 1024),
		expect: filter{
			typ:             delimited,
			pattern:         regexp.MustCompile("foo"),
			replacement:     []byte("bar"),
			delimiter:       []byte("baz"),
			maxEditorBuffer: 1024,
		},
	}, {
		title: "max buf as float",
		args:  args("foo", "bar", 1024.0),
		expect: filter{
			pattern:         regexp.MustCompile("foo"),
			replacement:     []byte("bar"),
			maxEditorBuffer: 1024,
		},
	}, {
		title: "max buf as int",
		args:  args("foo", "bar", 1024),
		expect: filter{
			pattern:         regexp.MustCompile("foo"),
			replacement:     []byte("bar"),
			maxEditorBuffer: 1024,
		},
	}, {
		title: "\\n in pattern",
		args:  args("foo\n", "bar"),
		expect: filter{
			pattern:     regexp.MustCompile("foo\n"),
			replacement: []byte("bar"),
		},
	}, {
		title: "\\\\n in pattern",
		args:  args("foo\\n", "bar"),
		expect: filter{
			pattern:     regexp.MustCompile(`foo\n`),
			replacement: []byte("bar"),
		},
	}, {
		title: "\\n in replacement",
		args:  args("foo", "bar\n"),
		expect: filter{
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar\n"),
		},
	}, {
		title: "\\n in delimiter",
		spec:  NewDelimited,
		args:  args("foo", "bar", "baz\n"),
		expect: filter{
			typ:         delimited,
			pattern:     regexp.MustCompile("foo"),
			replacement: []byte("bar"),
			delimiter:   []byte("baz\n"),
		},
	}} {
		t.Run(test.title, func(t *testing.T) {
			var s filters.Spec
			if test.spec == nil {
				s = New()
			} else {
				s = test.spec()
			}

			f, err := s.CreateFilter(test.args)
			if err == nil && test.fail {
				t.Fatal("Failed to fail.")
			} else if err != nil && !test.fail {
				t.Fatal(err)
			} else if err != nil {
				return
			}

			ff := f.(filter)

			if ff.typ != test.expect.typ {
				t.Error("Type doesn't match.")
			}

			if ff.pattern.String() != test.expect.pattern.String() {
				t.Error("Pattern doesn't match.")
			}

			if !bytes.Equal(ff.replacement, test.expect.replacement) {
				t.Error("Replacement doesn't match.")
			}

			if !bytes.Equal(ff.delimiter, test.expect.delimiter) {
				t.Error("Delimiter doesn't match.")
			}

			if ff.maxEditorBuffer != test.expect.maxEditorBuffer {
				t.Error("Max editor buffer doesn't match.")
			}
		})
	}
}
