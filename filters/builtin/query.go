package builtin

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type modQueryBehavior int

const (
	set modQueryBehavior = 1 + iota
	drop
)

type modQuery struct {
	behavior modQueryBehavior
	name     *eskip.Template
	value    *eskip.Template
}

// NewDropQuery returns a new dropQuery filter Spec, whose instances drop a corresponding
// query parameter.
//
// # Instances expect the name string or template parameter, see eskip.Template.ApplyContext
//
// Name: "dropQuery".
func NewDropQuery() filters.Spec { return &modQuery{behavior: drop} }

// NewSetQuery returns a new setQuery filter Spec, whose instances replace
// the query parameters.
//
// Instances expect two parameters: the name and the value to be set, either
// strings or templates are valid, see eskip.Template.ApplyContext
//
// Name: "setQuery".
func NewSetQuery() filters.Spec { return &modQuery{behavior: set} }

// "setQuery" or "dropQuery"
func (spec *modQuery) Name() string {
	switch spec.behavior {
	case drop:
		return filters.DropQueryName
	case set:
		return filters.SetQueryName
	default:
		panic("unspecified behavior")
	}
}

func createDropQuery(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	tpl, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &modQuery{behavior: drop, name: eskip.NewTemplate(tpl)}, nil
}

func createSetQuery(config []interface{}) (filters.Filter, error) {
	l := len(config)
	if l < 1 || l > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	name, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	if l == 1 {
		return &modQuery{behavior: set, name: eskip.NewTemplate(name)}, nil
	}

	value, ok := config[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &modQuery{behavior: set, name: eskip.NewTemplate(name), value: eskip.NewTemplate(value)}, nil
}

// Creates instances of the modQuery filter.
//
//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *modQuery) CreateFilter(config []interface{}) (filters.Filter, error) {
	switch spec.behavior {
	case drop:
		return createDropQuery(config)
	case set:
		return createSetQuery(config)
	default:
		panic("unspecified behavior")
	}
}

// Modifies the query of a request.
func (f *modQuery) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	params := req.URL.Query()

	switch f.behavior {
	case drop:
		name, _ := f.name.ApplyContext(ctx)
		params.Del(name)
	case set:
		if f.value == nil {
			req.URL.RawQuery, _ = f.name.ApplyContext(ctx)
			return
		} else {
			name, _ := f.name.ApplyContext(ctx)
			value, _ := f.value.ApplyContext(ctx)
			params.Set(name, value)
		}
	default:
		panic("unspecified behavior")
	}

	req.URL.RawQuery = params.Encode()
}

// Noop.
func (*modQuery) Response(filters.FilterContext) {}
