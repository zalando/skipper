package ratelimit

import (
	"github.com/zalando/skipper/filters"
)

type failureModeSpec struct{}
type failureMode struct {
	failClosed bool
}

func NewFailureMode() filters.Spec {
	return &failureModeSpec{}
}

func (*failureModeSpec) Name() string {
	return filters.RatelimitFailClosedName
}

func (*failureModeSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if a, ok := args[0].(string); ok && a == "true" || a == "false" {
		return &failureMode{
			failClosed: a == "true",
		}, nil
	}
	return nil, filters.ErrInvalidFilterParameters
}

func (fm *failureMode) Request(ctx filters.FilterContext) {
	ctx.StateBag()[FailClosedKey] = fm.failClosed
}

func (fm *failureMode) Response(ctx filters.FilterContext) {}
