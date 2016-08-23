package priority

import (
	"testing"

	"github.com/zalando/skipper/routing"
)

func TestCreate(t *testing.T) {
	spec := New()
	for _, ti := range []struct {
		msg       string
		args      []interface{}
		predicate routing.Predicate
		err       bool
	}{{
		msg: "no args",
		err: true,
	}, {
		msg:  "too many args",
		args: []interface{}{2.72, 3.14},
		err:  true,
	}, {
		msg:  "not a number",
		args: []interface{}{"1"},
		err:  true,
	}, {
		msg:       "ok",
		args:      []interface{}{3.14},
		predicate: routing.Priority(3.14),
	}} {
		func() {
			predicate, err := spec.Create(ti.args)
			if err == nil && ti.err {
				t.Error(ti.msg, "failed to fail")
				return
			} else if err != nil && !ti.err {
				t.Error(ti.msg, err)
				return
			} else if err != nil {
				return
			}

			if predicate != ti.predicate {
				t.Error("failed to create the right predicate")
			}
		}()
	}
}
