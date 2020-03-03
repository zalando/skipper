package sed_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestSed(t *testing.T) {
	// zero max buffer
	// small max buffer
	// large max buffer

	// infinite response
	// infinite response with prefixable expression

	for _, test := range []struct {
		title  string
		args   []interface{}
		body   string
		expect string
	}{{
		title: "empty body",
		args:  []interface{}{"foo", "bar"},
	}, {
		title:  "no match",
		args:   []interface{}{"foo", "bar"},
		body:   "barbazqux",
		expect: "barbazqux",
	}, {
		title:  "has match",
		args:   []interface{}{"foo", "bar"},
		body:   "foobarbazquxfoobarbazqux",
		expect: "barbarbazquxbarbarbazqux",
	}, {
		title:  "the whole body is replaced",
		args:   []interface{}{"foo", "bar"},
		body:   "foofoofoo",
		expect: "barbarbar",
	}, {
		title:  "the whole body is deleted",
		args:   []interface{}{"foo", ""},
		body:   "foofoofoo",
		expect: "",
	}, {
		title:  "consume and discard",
		args:   []interface{}{".*", ""},
		body:   "foobarbaz",
		expect: "",
	}} {
		t.Run(test.title, func(t *testing.T) {
			b := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Write([]byte(test.body))
				}),
			)
			defer b.Close()

			fr := builtin.MakeRegistry()
			p := proxytest.New(fr, &eskip.Route{
				Filters: []*eskip.Filter{{Name: sed.Name, Args: test.args}},
				Backend: b.URL,
			})

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
		})
	}
}

const text = `foobar
baz`

func TestSed1(t *testing.T) {
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(text))
	}))

	fr := builtin.MakeRegistry()
	p := proxytest.New(fr, &eskip.Route{
		Filters: []*eskip.Filter{{Name: sed.Name, Args: []interface{}{
			"[a-z][ac-z]*",
			"hoo",
		}}},
		Backend: b.URL,
	})

	rsp, err := http.Get(p.URL)
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()
	d, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}

	const expect = "hoohoo\nhoo"
	if string(d) != expect {
		t.Error("failed to edit stream")
		t.Log("expected:", expect)
		t.Log("got:", string(d))
	}
}
