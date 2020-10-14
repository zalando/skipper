/*
Package filtertest implements mock versions of the Filter, Spec and
FilterContext interfaces used during tests.
*/
package filtertest

import (
	"net/http"
	"sync/atomic"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

// Noop filter, used to verify the filter name and the args in the route.
// Implements both the Filter and the Spec interfaces.
type Filter struct {
	FilterName string
	Args       []interface{}
	closed     int64
}

// Simple FilterContext implementation.
type Context struct {
	FResponseWriter     http.ResponseWriter
	FRequest            *http.Request
	FResponse           *http.Response
	FServed             bool
	FServedWithResponse bool
	FParams             map[string]string
	FStateBag           map[string]interface{}
	FBackendUrl         string
	FOutgoingHost       string
	FMetrics            filters.Metrics
	FTracer             opentracing.Tracer
}

func (f *Filter) Name() string { return f.FilterName }
func (f *Filter) CreateFilter(config []interface{}) (filters.Filter, error) {
	return &Filter{f.FilterName, config, 0}, nil
}

func (f *Filter) Request(ctx filters.FilterContext)  {}
func (f *Filter) Response(ctx filters.FilterContext) {}
func (f *Filter) Close() error                       { atomic.AddInt64(&f.closed, 1); return nil }
func (f *Filter) Closed() bool                       { return atomic.LoadInt64(&f.closed) > 0 }

func (fc *Context) ResponseWriter() http.ResponseWriter { return fc.FResponseWriter }
func (fc *Context) Request() *http.Request              { return fc.FRequest }
func (fc *Context) Response() *http.Response            { return fc.FResponse }
func (fc *Context) MarkServed()                         { fc.FServed = true }
func (fc *Context) Served() bool                        { return fc.FServed }
func (fc *Context) PathParam(key string) string         { return fc.FParams[key] }
func (fc *Context) StateBag() map[string]interface{}    { return fc.FStateBag }
func (fc *Context) OriginalRequest() *http.Request      { return nil }
func (fc *Context) OriginalResponse() *http.Response    { return nil }
func (fc *Context) BackendUrl() string                  { return fc.FBackendUrl }
func (fc *Context) OutgoingHost() string                { return fc.FOutgoingHost }
func (fc *Context) SetOutgoingHost(h string)            { fc.FOutgoingHost = h }
func (fc *Context) Metrics() filters.Metrics            { return fc.FMetrics }
func (fc *Context) Tracer() opentracing.Tracer {
	if fc.FTracer != nil {
		return fc.FTracer
	}
	return &opentracing.NoopTracer{}
}
func (fc *Context) ParentSpan() opentracing.Span {
	return opentracing.StartSpan("test_span")
}

func (fc *Context) Serve(resp *http.Response) {
	fc.FServedWithResponse = true
	fc.FResponse = resp
	fc.FServed = true
}

func (fc *Context) Loopback() {}

func (fc *Context) Split() (filters.FilterContext, error) {
	return fc, nil
}
