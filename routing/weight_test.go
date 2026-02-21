package routing

import (
	"testing"
)

func TestWeightArgs(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		args   []any
		weight int
		err    bool
	}{{
		"no args",
		nil,
		0,
		true,
	}, {
		"too many args",
		[]any{"name", "value"},
		0,
		true,
	}, {
		"invalid value",
		[]any{"string"},
		0,
		true,
	}, {
		"ok float to int",
		[]any{500.99},
		500,
		false,
	}, {
		"ok",
		[]any{500},
		500,
		false,
	}} {
		func() {
			p, err := parseWeightPredicateArgs(ti.args)
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
