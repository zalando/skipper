package accesslog

import (
	"github.com/google/go-cmp/cmp"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAccessLogControl(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		state   filters.Spec
		args    []interface{}
		result  AccessLogFilter
		isError bool
	}{
		{
			msg:     "enables-access-log",
			state:   NewEnableAccessLog(),
			args:    nil,
			result:  AccessLogFilter{true, make([]int, 0)},
			isError: false,
		},
		{
			msg:     "enable-access-log-selective",
			state:   NewEnableAccessLog(),
			args:    []interface{}{2, 4, 300},
			result:  AccessLogFilter{true, []int{2, 4, 300}},
			isError: false,
		},
		{
			msg:     "enable-access-log-error-string",
			state:   NewEnableAccessLog(),
			args:    []interface{}{1, "a"},
			result:  AccessLogFilter{},
			isError: true,
		},
		{
			msg:     "disables-access-log",
			state:   NewDisableAccessLog(),
			args:    nil,
			result:  AccessLogFilter{false, make([]int, 0)},
			isError: false,
		},
		{
			msg:     "disables-access-log-selective",
			state:   NewDisableAccessLog(),
			args:    []interface{}{1, 201, 30},
			result:  AccessLogFilter{false, []int{1, 201, 30}},
			isError: false,
		},
		{
			msg:     "disables-access-log-convert-float",
			state:   NewDisableAccessLog(),
			args:    []interface{}{1.0, 201},
			result:  AccessLogFilter{false, []int{1, 201}},
			isError: false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := ti.state.CreateFilter(ti.args)

			if ti.isError {
				if err == nil {
					t.Errorf("Unexpected error creating filter %v", err)
					return
				} else {
					return
				}
			}

			var ctx filtertest.Context
			ctx.FStateBag = make(map[string]interface{})

			f.Request(&ctx)
			bag := ctx.StateBag()
			filter := bag[AccessLogEnabledKey]
			if diff := cmp.Diff(filter, &ti.result); diff != "" {
				t.Errorf("access log state is not equal to expected '%v' got %v", ti.result, bag[AccessLogEnabledKey])
			}
		})
	}
}
