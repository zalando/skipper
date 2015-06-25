// the filter instances created by this filter spec have no effect on either the request or the response.
// it can be used as a base composite type for other filter specs, that implement only either the request
// or the response processing.
package noop

import (
	"skipper/skipper"
)

const name = "_noop"

// type implements both skipper.FilterSpec and skipper.Filter.
type Type struct{ id string }

// the name of the filter spec: _noop
func (mw *Type) Name() string { return name }

// method to set the id of the created filters
func (mw *Type) SetId(id string) { mw.id = id }

// the id of the created filters
func (f *Type) Id() string { return f.id }

// noop request processing
func (f *Type) Request(ctx skipper.FilterContext) {}

// noop response processing
func (f *Type) Response(ctx skipper.FilterContext) {}

// returns a noop filter
func (mw *Type) MakeFilter(id string, config skipper.FilterConfig) (skipper.Filter, error) {
	return &Type{id}, nil
}
