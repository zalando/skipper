package noop

import (
	"skipper/skipper"
)

const name = "_noop"

type Type struct{ id string }

func (mw *Type) Name() string                      { return name }
func (mw *Type) SetId(id string)                   { mw.id = id }
func (f *Type) Id() string                         { return f.id }
func (f *Type) Request(ctx skipper.FilterContext)  {}
func (f *Type) Response(ctx skipper.FilterContext) {}

func (mw *Type) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	return &Type{id}, nil
}
