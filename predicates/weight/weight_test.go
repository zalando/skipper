package weight

import (
	"testing"
)

func TestWeightArgs(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		args   []interface{}
		weight int
		err    bool
	}{{
		"no args",
		nil,
		0,
		true,
	}, {
		"too many args",
		[]interface{}{"name", "value"},
		0,
		true,
	}, {
		"invalid value",
		[]interface{}{"string"},
		0,
		true,
	}, {
		"ok float to int",
		[]interface{}{500.99},
		500,
		false,
	}, {
		"ok",
		[]interface{}{500},
		500,
		false,
	}} {
		func() {
			p, err := ParseWeightPredicateArgs(ti.args)
			if ti.err && err == nil {
				t.Error(ti.msg, "failed to fail")
			} else if !ti.err && err != nil {
				t.Error(ti.msg, err)
			}

			if p != ti.weight {
				t.Error(ti.msg, "wrong weight")
			}
		}()
	}
}
