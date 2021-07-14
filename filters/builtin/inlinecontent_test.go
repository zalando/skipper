package builtin

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestInlineContent(t *testing.T) {
	for _, test := range []struct {
		title               string
		route               string
		invalidArgs         bool
		expectedContent     string
		expectedContentType string
	}{{
		title:       "no args",
		route:       `* -> inlineContent() -> <shunt>`,
		invalidArgs: true,
	}, {
		title:       "too many args",
		route:       `* -> inlineContent("foo", "bar", "baz") -> <shunt>`,
		invalidArgs: true,
	}, {
		title:       "not string for text",
		route:       `* -> inlineContent(42, "bar") -> <shunt>`,
		invalidArgs: true,
	}, {
		title:       "not string for mime",
		route:       `* -> inlineContent("foo", 42) -> <shunt>`,
		invalidArgs: true,
	}, {
		title: "empty",
		route: `* -> inlineContent("") -> <shunt>`,
	}, {
		title:               "some text, automatic",
		route:               `* -> inlineContent("foo bar baz") -> <shunt>`,
		expectedContent:     "foo bar baz",
		expectedContentType: "text/plain; charset=utf-8",
	}, {
		title:               "html, detected",
		route:               `* -> inlineContent("<!doctype html><html>foo</html>") -> <shunt>`,
		expectedContent:     "<!doctype html><html>foo</html>",
		expectedContentType: "text/html; charset=utf-8",
	}, {
		title: "some text, custom",
		route: `*
			-> inlineContent("foo bar baz", "application/foo")
			-> <shunt>`,
		expectedContent:     "foo bar baz",
		expectedContentType: "application/foo",
	}, {
		title: "some json",
		route: `*
			-> inlineContent("{\"foo\": [\"bar\", \"baz\"]}", "application/json")
			-> <shunt>`,
		expectedContent:     "{\"foo\": [\"bar\", \"baz\"]}",
		expectedContentType: "application/json",
	}, {
		title:               "template variable",
		route:               `* -> inlineContent("Hello ${request.query.name}") -> <shunt>`,
		expectedContent:     "Hello world",
		expectedContentType: "text/plain",
	}, {
		title:               "missing template variable",
		route:               `* -> inlineContent("Bye ${request.query.missing}") -> <shunt>`,
		expectedContent:     "Bye ",
		expectedContentType: "text/plain",
	}, {
		title:               "template variable content type",
		route:               `* -> inlineContent("<html>Hello ${request.query.name}</html>") -> <shunt>`,
		expectedContent:     "<html>Hello world</html>",
		expectedContentType: "text/html",
	}} {
		t.Run(test.title, func(t *testing.T) {
			r, err := eskip.Parse(test.route)
			if err != nil {
				t.Fatal(err)
			}

			p := proxytest.New(MakeRegistry(), r...)
			defer p.Close()

			rsp, err := http.Get(p.URL + "/?name=world")
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

			if test.invalidArgs {
				if rsp.StatusCode != http.StatusNotFound {
					t.Errorf("Expected 404 due to no route, got : %v", rsp.StatusCode)
				}
				return
			}

			if rsp.StatusCode != http.StatusOK {
				t.Error("invalid status received")
				t.Log("got:     ", rsp.StatusCode)
				t.Log("expected:", http.StatusOK)
			}

			if !strings.HasPrefix(
				rsp.Header.Get("Content-Type"),
				test.expectedContentType,
			) {
				t.Error("invalid content type received")
				t.Log("got:     ", rsp.Header.Get("Content-Type"))
				t.Log("expected:", test.expectedContentType)
			}

			if rsp.Header.Get("Content-Length") !=
				strconv.Itoa(len(test.expectedContent)) {
				t.Error("invalid content length received")
				t.Log("got:     ", rsp.Header.Get("Content-Length"))
				t.Log("expected:", len(test.expectedContent))
			}

			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Error(err)
				return
			}

			if string(b) != test.expectedContent {
				t.Error("invalid content received")
				t.Log("got:     ", string(b))
				t.Log("expected:", test.expectedContent)
			}
		})
	}
}
