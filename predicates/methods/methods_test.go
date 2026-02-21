package methods

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

func TestMethodsArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []any
		err  bool
	}{{
		"no args",
		nil,
		true,
	}, {
		"empty args",
		[]any{},
		true,
	}, {
		"invalid type",
		[]any{float64(1)},
		true,
	}, {
		"invalid method",
		[]any{"name", "name2"},
		true,
	}, {
		"ok",
		[]any{http.MethodGet, http.MethodPost},
		false,
	},
		{
			"ok case-insensitive",
			[]any{"GeT", "post", "oPtiOnS"},
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

func TestMethodsMatch(t *testing.T) {
	msg := "match multiple case-insensitive"
	args := []any{"gEt", "post", "DELETE", "ConnEct"}
	match := map[string]bool{
		"GeT":     true,
		"POST":    true,
		"head":    false,
		"pUt":     false,
		"PaTCH":   false,
		"DELETE":  true,
		"coNNect": true,
		"options": false,
		"trace":   false,
	}

	p, err := New().Create(args)
	if err != nil {
		t.Error(msg, err)
		return
	}

	for method, match := range match {
		r, err := http.ReadRequest(bufio.NewReader(strings.NewReader(method + " / HTTP/1.0\r\n\r\n")))
		if err != nil {
			t.Error(msg, err)
			return
		}

		if m := p.Match(r); m != match {
			t.Error(msg, "failed to match", m, match, method)
		}
	}
}
