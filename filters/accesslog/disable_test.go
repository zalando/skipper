package accesslog

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAccessLogDisabled(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		state  []interface{}
		result bool
		err    error
	}{
		{
			msg:    "false-value-enables-access-log",
			state:  []interface{}{"false"},
			result: false,
			err:    nil,
		},
		{
			msg:    "true-value-disables-access-log",
			state:  []interface{}{"true"},
			result: true,
			err:    nil,
		},
		{
			msg:    "unknown-argument-leads-to-error",
			state:  []interface{}{"unknownValue"},
			result: false,
			err:    filters.ErrInvalidFilterParameters,
		},
		{
			msg:    "no-arguments-lead-to-error",
			state:  []interface{}{},
			result: false,
			err:    filters.ErrInvalidFilterParameters,
		},
		{
			msg:    "multiple-arguments-lead-to-error",
			state:  []interface{}{"true", "second"},
			result: false,
			err:    filters.ErrInvalidFilterParameters,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := NewAccessLogDisabled().CreateFilter(ti.state)

			if err != ti.err {
				t.Errorf("error is not equal to expected '%v': %v", ti.err, err)
				return
			}

			if err != nil {
				return
			}

			var ctx filtertest.Context
			ctx.FStateBag = make(map[string]interface{})

			f.Request(&ctx)
			bag := ctx.StateBag()
			if bag[AccessLogDisabledKey] != ti.result {
				t.Errorf("access log state is not equal to expected '%v': %v", ti.result, bag[AccessLogDisabledKey])
			}
		})
	}
}
