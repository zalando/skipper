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

// Returns a new dropQuery filter Spec, whose instances drop a corresponding
// query parameter.
//
// As an EXPERIMENTAL feature: the dropQuery filter provides the possiblity
// to apply template operations. The current solution supports templates
// with placeholders of the format: ${param1}, and the placeholders will
// be replaced with the values of the same name from the wildcards in the
// Path() predicate.
// The templating feature will stay in Skipper, but the syntax of the
// templating may change.
//
// See also: https://github.com/zalando/skipper/issues/182
//
// Name: "dropQuery".
func NewDropQuery() filters.Spec { return &modQuery{behavior: drop} }

// Returns a new setQuery filter Spec, whose instances replace
// the query parameters.
//
// As an EXPERIMENTAL feature: the setPath filter provides the possiblity
// to apply template operations. The current solution supports templates
// with placeholders of the format: ${param1}, and the placeholders will
// be replaced with the values of the same name from the wildcards in the
// Path() predicate.
//
// See: https://godoc.org/github.com/zalando/skipper/routing#hdr-Wildcards
//
// The templating feature will stay in Skipper, but the syntax of the
// templating may change.
//
// See also: https://github.com/zalando/skipper/issues/182
//
// Instances expect two parameters: the name and the value to be set, either
// strings or templates are valid.
//
// Name: "setQuery".
func NewSetQuery() filters.Spec { return &modQuery{behavior: set} }

// "setQuery" or "dropQuery"
func (spec *modQuery) Name() string {
	switch spec.behavior {
	case drop:
		return DropQueryName
	case set:
		return SetQueryName
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
	if len(config) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	name, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	value, ok := config[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &modQuery{behavior: set, name: eskip.NewTemplate(name), value: eskip.NewTemplate(value)}, nil
}

// Creates instances of the modQuery filter.
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
		params.Del(f.name.Apply(ctx.PathParam))
	case set:
		params.Set(f.name.Apply(ctx.PathParam), f.value.Apply(ctx.PathParam))
	default:
		panic("unspecified behavior")
	}

	req.URL.RawQuery = params.Encode()
}

// Noop.
func (_ *modQuery) Response(_ filters.FilterContext) {}
