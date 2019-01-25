package accesslog

import (
	"github.com/google/go-cmp/cmp"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAccessLogDisabled(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		state  []interface{}
		result AccessLogFilter
		err    error
	}{
		{
			msg:    "false-value-enables-access-log",
			state:  []interface{}{"false"},
			result: AccessLogFilter{true, nil},
			err:    nil,
		},
		{
			msg:    "true-value-disables-access-log",
			state:  []interface{}{"true"},
			result: AccessLogFilter{false, nil},
			err:    nil,
		},
		{
			msg:    "unknown-argument-leads-to-error",
			state:  []interface{}{"unknownValue"},
			result: AccessLogFilter{true, nil},
			err:    filters.ErrInvalidFilterParameters,
		},
		{
			msg:    "no-arguments-lead-to-error",
			state:  []interface{}{},
			result: AccessLogFilter{true, nil},
			err:    filters.ErrInvalidFilterParameters,
		},
		{
			msg:    "multiple-arguments-lead-to-error",
			state:  []interface{}{"true", "second"},
			result: AccessLogFilter{true, nil},
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
			if diff := cmp.Diff(bag[AccessLogEnabledKey], &ti.result); diff != "" {
				t.Errorf("access log state is not equal to expected '%v': %v", ti.result, bag[AccessLogEnabledKey])
			}
		})
	}
}
