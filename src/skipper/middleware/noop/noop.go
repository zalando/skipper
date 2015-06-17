package noop

import (
	"net/http"
	"skipper/skipper"
)

const name = "_noop"

type Type struct{ id string }

func (mw *Type) Name() string                                   { return name }
func (mw *Type) SetId(id string)                                { mw.id = id }
func (f *Type) Id() string                                      { return f.id }
func (f *Type) ProcessRequest(r *http.Request) *http.Request    { return r }
func (f *Type) ProcessResponse(r *http.Response) *http.Response { return r }

func (mw *Type) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	return &Type{id}, nil
}
