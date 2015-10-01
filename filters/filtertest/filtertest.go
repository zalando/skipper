package filtertest

import (
	"github.com/zalando/skipper/filters"
	"net/http"
)

type Filter struct {
	FilterName string
	Args       []interface{}
}

type Context struct {
	FResponseWriter http.ResponseWriter
	FRequest        *http.Request
	FResponse       *http.Response
	FServed         bool
	FStateBag       map[string]interface{}
}

func (spec *Filter) Name() string                    { return spec.FilterName }
func (f *Filter) Request(ctx filters.FilterContext)  {}
func (f *Filter) Response(ctx filters.FilterContext) {}

func (fc *Context) ResponseWriter() http.ResponseWriter { return fc.FResponseWriter }
func (fc *Context) Request() *http.Request              { return fc.FRequest }
func (fc *Context) Response() *http.Response            { return fc.FResponse }
func (fc *Context) MarkServed()                         { fc.FServed = true }
func (fc *Context) Served() bool                        { return fc.FServed }
func (fc *Context) StateBag() map[string]interface{}    { return fc.FStateBag }

func (spec *Filter) CreateFilter(config []interface{}) (filters.Filter, error) {
	return &Filter{spec.FilterName, config}, nil
}
