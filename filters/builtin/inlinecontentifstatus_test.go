package builtin

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestInlineContentIfStatusArgs(t *testing.T) {
	for _, test := range []struct {
		title          string
		args           []any
		expectedStatus int
		expectedText   string
		expectedMime   string
		fail           bool
	}{{
		title: "no args",
		fail:  true,
	}, {
		title: "too little args",
		args:  []any{400},
		fail:  true,
	}, {
		title: "too many args",
		args:  []any{400, "bar", "baz", "qux"},
		fail:  true,
	}, {
		title: "not string for text",
		args:  []any{503, 42},
		fail:  true,
	}, {
		title: "too small status code",
		args:  []any{42, "bar"},
		fail:  true,
	}, {
		title: "too large status code",
		args:  []any{666, "bar"},
		fail:  true,
	}, {
		title: "not string for mime",
		args:  []any{400, "foo", 42},
		fail:  true,
	}, {
		title:          "status and text only",
		args:           []any{200, "foo"},
		expectedStatus: 200,
		expectedText:   "foo",
		expectedMime:   "text/plain",
	}, {
		title:          "status and text content, html type specified",
		args:           []any{403.0, `Works!`, "text/html"},
		expectedStatus: 403,
		expectedText:   `Works!`,
		expectedMime:   "text/html",
	}, {
		title:          "status and html type detected",
		args:           []any{500, `<!doctype html><html>foo</html>`},
		expectedStatus: 500,
		expectedText:   `<!doctype html><html>foo</html>`,
		expectedMime:   "text/html",
	}, {
		title:          "status and html type detected",
		args:           []any{500, `<!doctype html><html>foo</html>`},
		expectedStatus: 500,
		expectedText:   `<!doctype html><html>foo</html>`,
		expectedMime:   "text/html",
	}} {
		t.Run(test.title, func(t *testing.T) {
			f, err := (&inlineContentIfStatus{}).CreateFilter(test.args)
			if test.fail && err == nil {
				t.Error("fail to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			c := f.(*inlineContentIfStatus)

			if c.text != test.expectedText {
				t.Error("invalid status")
				t.Log("got:     ", c.statusCode)
				t.Log("expected:", test.expectedStatus)
			}

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

func TestInlineContentIfStatus(t *testing.T) {
	for _, test := range []struct {
		title               string
		routes              string
		expectedStatus      int
		expectedContent     string
		expectedContentType string
	}{{
		title: "some text, automatic",
		routes: `*
			-> inlineContentIfStatus(404, "<h1>Not Found</h1>")
			-> <shunt>`,
		expectedStatus:      404,
		expectedContent:     "<h1>Not Found</h1>",
		expectedContentType: "text/html",
	}, {
		title: "some text, custom",
		routes: `*
			-> inlineContentIfStatus(500, "Internal Error", "application/foo")
			-> status(500)
			-> <shunt>`,
		expectedStatus:      500,
		expectedContent:     "Internal Error",
		expectedContentType: "application/foo",
	}, {
		title: "some json",
		routes: `*
			-> inlineContentIfStatus(200, "{\"foo\": [\"bar\", \"baz\"]}", "application/json")
			-> status(200)
			-> <shunt>`,
		expectedStatus:      200,
		expectedContent:     `{"foo": ["bar", "baz"]}`,
		expectedContentType: "application/json",
	}, {
		title: "should reset content encoding",
		routes: `*
			-> inlineContentIfStatus(200, "text")
			-> status(200)
			-> compress()
			-> inlineContent("compressed text")
			-> <shunt>`,
		expectedStatus:      200,
		expectedContent:     `text`,
		expectedContentType: "text/plain",
	}} {
		t.Run(test.title, func(t *testing.T) {
			r := eskip.MustParse(test.routes)

			p := proxytest.New(MakeRegistry(), r...)
			defer p.Close()

			rsp, err := http.Get(p.URL)
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != test.expectedStatus {
				t.Error("invalid status received")
				t.Log("got:     ", rsp.StatusCode)
				t.Log("expected:", test.expectedStatus)
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

			if rsp.Header.Get("Content-Encoding") != "" {
				t.Error("content encoding not reset")
			}

			b, err := io.ReadAll(rsp.Body)
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
