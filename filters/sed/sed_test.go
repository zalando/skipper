package sed_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

type testItem struct {
	title      string
	args       []any
	body       string
	bodyReader io.Reader
	expect     string
}

func testResponse(name string, test testItem) func(*testing.T) {
	return func(t *testing.T) {
		b := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.bodyReader == nil {
					w.Write([]byte(test.body))
					return
				}

				if _, err := io.Copy(w, test.bodyReader); err != nil {
					t.Log(err)
				}
			}),
		)
		defer b.Close()

		fr := builtin.MakeRegistry()
		p := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: name, Args: test.args}},
			Backend: b.URL,
		})

		defer p.Close()
		rsp, err := http.Get(p.URL)
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()
		d, err := io.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if string(d) != test.expect {
			t.Error("Failed to edit stream.")
			t.Log("Got:     ", string(d))
			t.Log("Expected:", test.expect)
		}
	}
}

func testRequest(name string, test testItem) func(*testing.T) {
	return func(t *testing.T) {
		b := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintln(w, err)
					return
				}

				if string(b) != test.expect {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintln(w)
					fmt.Fprintf(w, "Got:      %v\n", string(b))
					fmt.Fprintf(w, "Expected: %v\n", test.expect)
					return
				}
			}),
		)
		defer b.Close()

		fr := builtin.MakeRegistry()
		p := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: name, Args: test.args}},
			Backend: b.URL,
		})

		defer p.Close()
		body := test.bodyReader
		if body == nil {
			body = bytes.NewBufferString(test.body)
		}

		rsp, err := http.Post(p.URL, "text/plain", body)
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Error("Failed to edit stream.")
			d, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(err)
			}

			t.Log(string(d))
		}
	}
}

func TestSed(t *testing.T) {
	args := func(a ...any) []any { return a }
	for _, test := range []testItem{{
		title: "empty body",
		args:  args("foo", "bar"),
	}, {
		title:  "no match",
		args:   args("foo", "bar"),
		body:   "barbazqux",
		expect: "barbazqux",
	}, {
		title:  "no match, not prefixable",
		args:   args("[0-9]+", "this was a number"),
		body:   "foobarbaz",
		expect: "foobarbaz",
	}, {
		title:  "not consumable match",
		args:   args("[0-9]*", "this was a number"),
		body:   "foobarbaz",
		expect: "foobarbaz",
	}, {
		title:  "has match",
		args:   args("foo", "bar"),
		body:   "foobarbazquxfoobarbazqux",
		expect: "barbarbazquxbarbarbazqux",
	}, {
		title:  "non-prefixable match",
		args:   args("[0-9]+", "_"),
		body:   "foobar123bazqux",
		expect: "foobar_bazqux",
	}, {
		title:  "the whole body is replaced",
		args:   args("foo", "bar"),
		body:   "foofoofoo",
		expect: "barbarbar",
	}, {
		title:  "the whole body is deleted",
		args:   args("foo", ""),
		body:   "foofoofoo",
		expect: "",
	}, {
		title:  "consume and discard",
		args:   args(".*", ""),
		body:   "foobarbaz",
		expect: "",
	}, {
		title:  "capture groups are ignored but ok",
		args:   args("foo(bar)baz", "qux"),
		body:   "foobarbaz",
		expect: "qux",
	}, {
		title:  "expand the body to make it longer",
		args:   args("foo", "foobarbaz"),
		body:   "foobarbazfoobarbazfoobarbaz",
		expect: "foobarbazbarbazfoobarbazbarbazfoobarbazbarbaz",
	}} {
		t.Run(fmt.Sprintf("%s/%s", filters.SedRequestName, test.title), testRequest(filters.SedRequestName, test))
		t.Run(fmt.Sprintf("%s/%s", filters.SedName, test.title), testResponse(filters.SedName, test))
	}
}

func TestSedLongStream(t *testing.T) {
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

	baseArgs := []any{pattern, outputString}

	t.Run("below max buffer size", testResponse(filters.SedName, testItem{
		args:       append(baseArgs, bodySize*2),
		bodyReader: createBody(),
		expect:     "qux",
	}))

	t.Run("above max buffer size, abort", testResponse(filters.SedName, testItem{
		args:       append(baseArgs, bodySize/2, "abort"),
		bodyReader: createBody(),
	}))

	t.Run("above max buffer size, best effort", testResponse(filters.SedName, testItem{
		args:       append(baseArgs, bodySize/2),
		bodyReader: createBody(),
		expect:     "quxqux",
	}))
}
