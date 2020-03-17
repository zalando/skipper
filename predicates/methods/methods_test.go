package methods

import (
	"bufio"
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/routing"
)

func TestMethodsArgs(t *testing.T) {
	for _, tt := range []struct {
		msg  string
		spec routing.PredicateSpec
	}{{
		"singular",
		NewSingular(),
	}, {
		"plural",
		NewPlural(),
	}} {
		for _, ti := range []struct {
			msg  string
			args []interface{}
			err  bool
		}{{
			"no args",
			nil,
			true,
		}, {
			"empty args",
			[]interface{}{},
			true,
		}, {
			"invalid type",
			[]interface{}{float64(1)},
			true,
		}, {
			"invalid method",
			[]interface{}{"name", "name2"},
			true,
		}, {
			"ok",
			[]interface{}{http.MethodGet, http.MethodPost},
			false,
		}, {
			"ok case insensitive",
			[]interface{}{"GeT", "post", "oPtiOnS"},
			false,
		}} {
			func() {
				p, err := tt.spec.Create(ti.args)
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
}

func TestMethodsMatch(t *testing.T) {
	for _, tt := range []struct {
		msg  string
		spec routing.PredicateSpec
	}{{
		"singular",
		NewSingular(),
	}, {
		"plural",
		NewPlural(),
	}} {
		msg := "match multiple case insensitive"
		args := []interface{}{"gEt", "post", "DELETE", "ConnEct"}
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

		p, err := tt.spec.Create(args)
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
}
