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

func (*timeout) Name() string { return filters.BackendTimeoutName }

func (*timeout) CreateFilter(args []interface{}) (filters.Filter, error) {
	a := filters.Args(args)
	return &timeout{a.Duration()}, a.Err()
}

func (t *timeout) Request(ctx filters.FilterContext) {
	// allows overwrite
	ctx.StateBag()[filters.BackendTimeout] = t.timeout
}

func (t *timeout) Response(filters.FilterContext) {}
