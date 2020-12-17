package builtin

import (
	"time"

	"github.com/zalando/skipper/filters"
)

type timeout struct {
	timeout time.Duration
}

func NewBackendTimeout() filters.Spec {
	return &timeout{}
}

func (*timeout) Name() string { return BackendTimeoutName }

func (*timeout) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var tf timeout
	switch v := args[0].(type) {
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
		tf.timeout = d
	case time.Duration:
		tf.timeout = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
	return &tf, nil
}

func (t *timeout) Request(ctx filters.FilterContext) {
	// allows overwrite
	ctx.StateBag()[filters.BackendTimeout] = t.timeout
}

func (t *timeout) Response(filters.FilterContext) {}
