package tee

import (
	"github.com/zalando/skipper/filters"
	teepredicate "github.com/zalando/skipper/predicates/tee"
)

// FilterName is the filter name
// Deprecated, use filters.TeeLoopbackName instead
const FilterName = filters.TeeLoopbackName

type teeLoopbackSpec struct{}
type teeLoopbackFilter struct {
	teeKey string
}

func (t *teeLoopbackSpec) Name() string {
	return filters.TeeLoopbackName
}

func (t *teeLoopbackSpec) CreateFilter(args []any) (filters.Filter, error) {

	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	teeKey, _ := args[0].(string)
	if teeKey == "" {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &teeLoopbackFilter{
		teeKey,
	}, nil
}

func NewTeeLoopback() filters.Spec {
	return &teeLoopbackSpec{}
}

func (f *teeLoopbackFilter) Request(ctx filters.FilterContext) {
	cc, err := ctx.Split()
	if err != nil {
		ctx.Logger().Errorf("teeloopback: failed to split the context request: %v", err)
		return
	}
	cc.Request().Header.Set(teepredicate.HeaderKey, f.teeKey)
	go cc.Loopback()

}

func (f *teeLoopbackFilter) Response(_ filters.FilterContext) {}
