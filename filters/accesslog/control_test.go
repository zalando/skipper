package accesslog

import (
	"testing"

	"github.com/google/go-cmp/cmp"

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
			result:  AccessLogFilter{Enable: true, Prefixes: make([]int, 0)},
			isError: false,
		},
		{
			msg:     "enable-access-log-selective",
			state:   NewEnableAccessLog(),
			args:    []interface{}{2, 4, 300},
			result:  AccessLogFilter{Enable: true, Prefixes: []int{2, 4, 300}},
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
			result:  AccessLogFilter{Enable: false, Prefixes: make([]int, 0)},
			isError: false,
		},
		{
			msg:     "disables-access-log-selective",
			state:   NewDisableAccessLog(),
			args:    []interface{}{1, 201, 30},
			result:  AccessLogFilter{Enable: false, Prefixes: []int{1, 201, 30}},
			isError: false,
		},
		{
			msg:     "disables-access-log-convert-float",
			state:   NewDisableAccessLog(),
			args:    []interface{}{1.0, 201},
			result:  AccessLogFilter{Enable: false, Prefixes: []int{1, 201}},
			isError: false,
		},
		{
			msg:     "mask-access-log-query",
			state:   NewMaskAccessLogQuery(),
			args:    []interface{}{"key_1"},
			result:  AccessLogFilter{Enable: true, MaskedQueryParams: map[string]bool{"key_1": true}},
			isError: false,
		},
		{
			msg:     "mask-access-log-query-convert-int",
			state:   NewMaskAccessLogQuery(),
			args:    []interface{}{1},
			result:  AccessLogFilter{},
			isError: true,
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
