package testfilter

import "github.com/zalando/skipper/filters"

type T struct {
    FilterName string
    Args []interface{}
}

func (spec *T) Name() string { return spec.FilterName }
func (f *T) Request(ctx filters.FilterContext) {}
func (f *T) Response(ctx filters.FilterContext) {}

func (spec *T) CreateFilter(config []interface{}) (filters.Filter, error) {
    return &T{spec.FilterName, config}, nil
}
