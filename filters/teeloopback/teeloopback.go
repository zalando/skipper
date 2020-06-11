package teeloopback

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/tee"
)

const FilterName = "teeLoopback"

type teeLoopbackSpec struct{}
type teeLoopbackFilter struct {
	teeKey string
}

func (t *teeLoopbackSpec) Name() string {
	return FilterName
}

func (t *teeLoopbackSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	teeKey, ok := args[0].(string)
	if !ok || teeKey == "" {
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
	origRequest := ctx.Request()
	// prevent the loopback to be executed twice
	v := origRequest.Header.Get(tee.HeaderKeyName)
	if v == f.teeKey {
		return
	}
	cc, _ := ctx.Split()
	r := cc.Request()
	r.Header.Set(tee.HeaderKeyName, f.teeKey)
	go cc.Loopback()

}

func (f *teeLoopbackFilter) Response(_ filters.FilterContext) {}
