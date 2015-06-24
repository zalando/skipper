// the filter instances created by this middleware have no effect on either the request or the response.
// it can be used as a base composite type for other middleware, that implement only either the request
// or the response processing.
package noop

import (
	"skipper/skipper"
)

const name = "_noop"

// type implements both skipper.Middleware and skipper.Filter.
type Type struct{ id string }

// the name of the middleware: _noop
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
func (mw *Type) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	return &Type{id}, nil
}
