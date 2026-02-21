package cookie

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestCookieArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []any
		err  bool
	}{{
		"no args",
		nil,
		true,
	}, {
		"too many args",
		[]any{"name", "value", "something"},
		true,
	}, {
		"invalid name",
		[]any{float64(1), "value"},
		true,
	}, {
		"invalid value",
		[]any{"name", `\`},
		true,
	}, {
		"ok",
		[]any{"name", "value"},
		false,
	}} {
		func() {
			p, err := New().Create(ti.args)
			if ti.err && err == nil {
				t.Error(ti.msg, "failed to fail")
			} else if !ti.err && err != nil {
				t.Error(ti.msg, err)
			}

			if err != nil {
				return
			}

			if p == nil {
				t.Error(ti.msg, "failed to create filter")
			}
		}()
	}
}

func TestCookieMatch(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		args    []any
		cookies string
		match   bool
	}{{
		"not found",
		[]any{"tcial", "^enabled$"},
		"some=value",
		false,
	}, {
		"don't match",
		[]any{"tcial", "^enabled, but not working$"},
		"some=value;tcial=enabled",
		false,
	}, {
		"match",
		[]any{"tcial", "^enabled$"},
		"some=value;tcial=enabled",
		true,
	}} {
		func() {
			p, err := New().Create(ti.args)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			r, err := http.ReadRequest(bufio.NewReader(strings.NewReader(fmt.Sprintf(
				"GET / HTTP/1.0\r\nCookie: %s\r\n\r\n", ti.cookies))))
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			if m := p.Match(r); m != ti.match {
				t.Error(ti.msg, "failed to match", m, ti.match)
			}
		}()
	}
}
