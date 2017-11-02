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

func TestInlineContentArgs(t *testing.T) {
	for _, test := range []struct {
		title        string
		args         []interface{}
		expectedText string
		expectedMime string
		fail         bool
	}{{
		title: "no args",
		fail:  true,
	}, {
		title: "too many args",
		args:  []interface{}{"foo", "bar", "baz"},
		fail:  true,
	}, {
		title: "not string for text",
		args:  []interface{}{42, "bar"},
		fail:  true,
	}, {
		title: "not string for mime",
		args:  []interface{}{"foo", 42},
		fail:  true,
	}, {
		title:        "text only",
		args:         []interface{}{"foo"},
		expectedText: "foo",
		expectedMime: "text/plain",
	}, {
		title:        "html, detected",
		args:         []interface{}{`<!doctype html><html>foo</html>`},
		expectedText: `<!doctype html><html>foo</html>`,
		expectedMime: "text/html",
	}} {
		t.Run(test.title, func(t *testing.T) {
			f, err := (&inlineContent{}).CreateFilter(test.args)
			if test.fail && err == nil {
				t.Error("fail to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			c := f.(*inlineContent)

			if c.text != test.expectedText {
				t.Error("invalid content")
				t.Log("got:     ", c.text)
				t.Log("expected:", test.expectedText)
			}

			if !strings.HasPrefix(c.mime, test.expectedMime) {
				t.Error("invalid mime")
				t.Log("got:     ", c.mime)
				t.Log("expected:", test.expectedMime)
			}
		})
	}
}

func TestInlineContent(t *testing.T) {
	for _, test := range []struct {
		title               string
		routes              string
		expectedContent     string
		expectedContentType string
	}{{
		title:  "empty",
		routes: `* -> inlineContent("") -> <shunt>`,
	}, {
		title:               "some text, automatic",
		routes:              `* -> inlineContent("foo bar baz") -> <shunt>`,
		expectedContent:     "foo bar baz",
		expectedContentType: "text/plain",
	}, {
		title: "some text, custom",
		routes: `*
			-> inlineContent("foo bar baz", "application/foo")
			-> <shunt>`,
		expectedContent:     "foo bar baz",
		expectedContentType: "application/foo",
	}, {
		title: "some json",
		routes: `*
			-> inlineContent("{\"foo\": [\"bar\", \"baz\"]}", "application/json")
			-> <shunt>`,
		expectedContent:     "{\"foo\": [\"bar\", \"baz\"]}",
		expectedContentType: "application/json",
	}} {
		t.Run(test.title, func(t *testing.T) {
			r, err := eskip.Parse(test.routes)
			if err != nil {
				t.Error(err)
				return
			}

			p := proxytest.New(MakeRegistry(), r...)
			defer p.Close()

			rsp, err := http.Get(p.URL)
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

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
