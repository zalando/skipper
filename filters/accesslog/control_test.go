package accesslog

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAccessLogControl(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		state   filters.Spec
		args    []any
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
			args:    []any{2, 4, 300},
			result:  AccessLogFilter{Enable: true, Prefixes: []int{2, 4, 300}},
			isError: false,
		},
		{
			msg:     "enable-access-log-error-string",
			state:   NewEnableAccessLog(),
			args:    []any{1, "a"},
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
			args:    []any{1, 201, 30},
			result:  AccessLogFilter{Enable: false, Prefixes: []int{1, 201, 30}},
			isError: false,
		},
		{
			msg:     "disables-access-log-convert-float",
			state:   NewDisableAccessLog(),
			args:    []any{1.0, 201},
			result:  AccessLogFilter{Enable: false, Prefixes: []int{1, 201}},
			isError: false,
		},
		{
			msg:     "mask-access-log-query",
			state:   NewMaskAccessLogQuery(),
			args:    []any{"key_1"},
			result:  AccessLogFilter{Enable: true, MaskedQueryParams: map[string]struct{}{"key_1": {}}},
			isError: false,
		},
		{
			msg:     "mask-access-log-query-convert-int",
			state:   NewMaskAccessLogQuery(),
			args:    []any{1},
			result:  AccessLogFilter{},
			isError: true,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := ti.state.CreateFilter(ti.args)

			if ti.isError {
				require.Error(t, err, "Expected error creating filter")
				return
			}

			var ctx filtertest.Context
			ctx.FStateBag = make(map[string]any)

			f.Request(&ctx)
			bag := ctx.StateBag()
			filter := bag[AccessLogEnabledKey]

			assert.Equal(t, filter, &ti.result, "access log state is not equal to expected")
		})
	}
}

func TestAccessLogMaskedParametersMerging(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		state  filters.Spec
		args   [][]any
		result map[string]struct{}
	}{
		{
			msg:   "should merge masked query params from multiple filters",
			state: NewMaskAccessLogQuery(),
			args: [][]any{
				{"key_1"},
				{"key_2"},
			},
			result: map[string]struct{}{"key_1": {}, "key_2": {}},
		},
		{
			msg:   "should overwrite already masked params",
			state: NewMaskAccessLogQuery(),
			args: [][]any{
				{"key_1"},
				{"key_1"},
				{"key_1"},
			},
			result: map[string]struct{}{"key_1": {}},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {

			filters := make([]filters.Filter, len(ti.args))
			for i, a := range ti.args {
				f, err := ti.state.CreateFilter(a)
				require.NoError(t, err, "Expected no error creating filter")
				filters[i] = f
			}

			var ctx filtertest.Context
			ctx.FStateBag = make(map[string]any)

			for _, f := range filters {
				f.Request(&ctx)
			}

			bag := ctx.StateBag()
			params := bag[AccessLogAdditionalDataKey].(map[string]any)[KeyMaskedQueryParams].(map[string]struct{})
			assert.Equal(t, params, ti.result, "access log state is not equal to expected")
		})
	}
}
