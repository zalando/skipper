package query_missing

import (
	"net/http"
	"testing"
)

func TestQueryMissingArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []interface{}
		err  bool
	}{{
		"too few args",
		[]interface{}{},
		true,
	}, {
		"too many args",
		[]interface{}{"key", "value", "something"},
		true,
	}, {
		"exists case",
		[]interface{}{"query"},
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
				t.Error(ti.msg, "failed to create predicate")
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
		"should return true for non existing param",
		[]interface{}{"key"},
		"someOtherKey",
		[]string{},
		true,
	}, {
		"should return true for empty params",
		[]interface{}{"key"},
		"key",
		[]string{"", ""},
		true,
	}, {
		"should return false if parameter exist",
		[]interface{}{"key"},
		"key",
		[]string{"value1"},
		false,
	}, {
		"should return false if parameter has at least one value",
		[]interface{}{"key"},
		"key",
		[]string{"", "value", ""},
		false,
	}} {
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
