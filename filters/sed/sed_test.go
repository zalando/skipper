package sed_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/proxy/proxytest"
)

type testItem struct {
	title           string
	args            []interface{}
	body            string
	expect          string
	forceReadBuffer int
}

func testResponse(name string, test testItem) func(*testing.T) {
	return func(t *testing.T) {
		b := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(test.body))
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
		d, err := ioutil.ReadAll(rsp.Body)
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
				b, err := ioutil.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintln(w, err)
					return
				}

				if string(b) != test.expect {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintln(w)
					fmt.Fprintf(w, "Got:      %v\n", string(b))
					fmt.Fprintf(w, "Expected: %v\n", string(b))
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
		rsp, err := http.Post(p.URL, "text/plain", bytes.NewBufferString(test.body))
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Error("Failed to edit stream.")
			d, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(err)
			}

			t.Log(string(d))
		}
	}
}

func TestSed(t *testing.T) {
	args := func(a ...interface{}) []interface{} { return a }
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
		title:  "non prefixable match",
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
		title:  "default max buffer",
		args:   args("foo", "bar"),
		body:   "foobarbaz",
		expect: "barbarbaz",
	}, {
		title:  "small max buffer",
		args:   args("a", "X", 1),
		body:   "foobarbaz",
		expect: "foobXrbXz",
	}} {
		t.Run(fmt.Sprintf("%s/%s", sed.NameRequest, test.title), testRequest(sed.NameRequest, test))
		t.Run(fmt.Sprintf("%s/%s", sed.Name, test.title), testResponse(sed.Name, test))
	}
}
