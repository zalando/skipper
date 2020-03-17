package methods

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

func TestMethodArgs(t *testing.T) {
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
		"invalid multiple args",
		[]interface{}{http.MethodGet, http.MethodPost},
		true,
	}, {
		"ok",
		[]interface{}{http.MethodGet},
		false,
	}, {
		"ok case insensitive",
		[]interface{}{"GeT"},
		false,
	}} {
		func() {
			p, err := NewSingular().Create(ti.args)
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

func TestMethodsArgs(t *testing.T) {
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
			p, err := NewPlural().Create(ti.args)
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

func TestMethodMatch(t *testing.T) {
	msg := "match multiple case insensitive"
	args := []interface{}{"gEt"}
	match := map[string]bool{
		"GeT":     true,
		"POST":    false,
		"head":    false,
		"pUt":     false,
		"PaTCH":   false,
		"DELETE":  false,
		"coNNect": false,
		"options": false,
		"trace":   false,
	}

	p, err := NewSingular().Create(args)
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

func TestMethodsMatch(t *testing.T) {
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

	p, err := NewPlural().Create(args)
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
