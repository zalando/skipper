package routesrv

import (
	"net/http"

	ot "github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

// filterContext is a minimal filters.FilterContext implementation for use as
// HTTP middleware. It supports the Request() path only: filters can read the
// request, set state-bag values, and short-circuit via Serve(). Methods that
// are meaningful only inside the proxy's full routing engine (Split, Loopback,
// backend access, etc.) are not implemented and will panic if called.
type filterContext struct {
	w        http.ResponseWriter
	r        *http.Request
	served   bool
	stateBag map[string]interface{}
}

func (fc *filterContext) ResponseWriter() http.ResponseWriter   { return fc.w }
func (fc *filterContext) Request() *http.Request                { return fc.r }
func (fc *filterContext) Response() *http.Response              { return nil }
func (fc *filterContext) OriginalRequest() *http.Request        { return fc.r }
func (fc *filterContext) OriginalResponse() *http.Response      { return nil }
func (fc *filterContext) Served() bool                          { return fc.served }
func (fc *filterContext) MarkServed()                           { fc.served = true }
func (fc *filterContext) StateBag() map[string]interface{}      { return fc.stateBag }
func (fc *filterContext) BackendUrl() string                    { return "" }
func (fc *filterContext) OutgoingHost() string                  { return "" }
func (fc *filterContext) SetOutgoingHost(string)                {}
func (fc *filterContext) PathParam(string) string               { return "" }
func (fc *filterContext) RouteId() string                       { return "" }
func (fc *filterContext) Metrics() filters.Metrics              { return nil }
func (fc *filterContext) Tracer() ot.Tracer                     { return nil }
func (fc *filterContext) ParentSpan() ot.Span                   { return nil }
func (fc *filterContext) Split() (filters.FilterContext, error) { panic("not implemented") }
func (fc *filterContext) Loopback()                             { panic("not implemented") }
func (fc *filterContext) LoopbackWithResponse()                 { panic("not implemented") }

func (fc *filterContext) Logger() filters.FilterContextLogger {
	return log.StandardLogger()
}

func (fc *filterContext) Serve(rsp *http.Response) {
	fc.served = true
	if rsp == nil {
		fc.w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for k, vs := range rsp.Header {
		for _, v := range vs {
			fc.w.Header().Add(k, v)
		}
	}
	status := rsp.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	fc.w.WriteHeader(status)
}

// withFilters returns an http.Handler that runs the given filter chain before
// delegating to h. If any filter calls ctx.Serve(), the chain stops and h is
// not called. An empty filter slice returns h unchanged.
func withFilters(h http.Handler, chain []filters.Filter) http.Handler {
	if len(chain) == 0 {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := &filterContext{
			w:        w,
			r:        r,
			stateBag: make(map[string]any),
		}
		for _, f := range chain {
			f.Request(ctx)
			if ctx.served {
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
