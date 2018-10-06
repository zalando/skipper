package accesslog

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAccessLogControl(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		state  filters.Spec
		result bool
	}{
		{
			msg:    "false-value-enables-access-log",
			state:  NewEnableAccessLog(),
			result: true,
		},
		{
			msg:    "true-value-disables-access-log",
			state:  NewDisableAccessLog(),
			result: false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := ti.state.CreateFilter([]interface{}{})

			if err != nil {
				return
			}

			var ctx filtertest.Context
			ctx.FStateBag = make(map[string]interface{})

			f.Request(&ctx)
			bag := ctx.StateBag()
			if bag[AccessLogEnabledKey] != ti.result {
				t.Errorf("access log state is not equal to expected '%v': %v", ti.result, bag[AccessLogEnabledKey])
			}
		})
	}
}
