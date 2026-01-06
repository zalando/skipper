package query

import (
	"net/http"
	"testing"
)

func TestQueryArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []interface{}
		typ  matchType
		err  bool
	}{{
		"too few args",
		[]interface{}{},
		0,
		true,
	}, {
		"too many args",
		[]interface{}{"key", "value", "something"},
		0,
		true,
	}, {
		"exists case",
		[]interface{}{"query"},
		exists,
		false,
	}, {
		"match case simple",
		[]interface{}{"key", "value"},
		matches,
		false,
	}, {
		"match case regexp",
		[]interface{}{"key", "value"},
		matches,
		false,
	}, {
		"invalid regexp",
		[]interface{}{"key", "value", `\`},
		0,
		true,
	}, {
		"invalid type key",
		[]interface{}{5, "value"},
		0,
		true,
	}, {
		"invalid type value",
		[]interface{}{"key", 5},
		0,
		true,
	}, {
		"invalid regexp string",
		[]interface{}{"key", `\`},
		0,
		true,
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
				t.Error(ti.msg, "failed to create predicate")
			}

			switch v := p.(type) {
			case *predicate:
				if v.typ != ti.typ {
					t.Error(ti.msg, err)
					return
				}
			}

		}()
	}
}

func TestMatchArgs(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		args   []interface{}
		key    string
		values []string
		match  bool
	}{{
		"find existing params",
		[]interface{}{"key"},
		"key",
		[]string{"value"},
		true,
	}, {
		"does not find nonexistent params",
		[]interface{}{"keyNot"},
		"key",
		[]string{"value"},
		false,
	}, {
		"find existing params with multiple values",
		[]interface{}{"key"},
		"key",
		[]string{"value1", "value2"},
		true,
	}, {
		"match query params",
		[]interface{}{"key", "^regexp$"},
		"key",
		[]string{"regexp"},
		true,
	}, {
		"match query params with multiple values",
		[]interface{}{"key", "^regexp$"},
		"key",
		[]string{"value", "regexp"},
		true,
	}, {
		"does not match nonexistent params",
		[]interface{}{"key", "^regexp$"},
		"key",
		[]string{"value", "value2"},
		false,
	},
	} {
		func() {
			p, err := New().Create(ti.args)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}
			if p == nil {
				t.Error(ti.msg, "failed to create predicate")
			}

			req, _ := http.NewRequest("GET", "http://example.com", nil)

			q := req.URL.Query()
			for _, v := range ti.values {
				q.Add(ti.key, v)
			}
			req.URL.RawQuery = q.Encode()

			if m := p.Match(req); m != ti.match {
				t.Error(ti.msg, "failed to match", m, ti.match)
			}

		}()
	}
}
