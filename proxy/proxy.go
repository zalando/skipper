// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"bytes"
	"crypto/tls"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	proxyBufferSize = 8192
	proxyErrorFmt   = "proxy: %s"

	// TODO: this should be fine tuned, yet, with benchmarks.
	// In case it doesn't make a big difference, then a lower value
	// can be safer, but the default 2 turned out to be too low during
	// previous benchmarks.
	//
	// Also: it should be a parameter.
	idleConnsPerHost = 64
)

type Options uint

const (
	OptionsNone Options = 0

	// Flag indicating to ignore the verification of the TLS
	// certificates of the backend services.
	OptionsInsecure Options = 1 << iota

	// Flag indicating whether filters require the preserved original
	// metadata of the request and the response.
	OptionsPreserveOriginal

	// Flag indicating whether the outgoing request to the backend
	// should use by default the 'Host' header of the incoming request,
	// or the host part of the backend address, in case filters don't
	// change it.
	OptionsProxyPreserveHost

	OptionsDebug
)

func (o Options) Insecure() bool          { return o&OptionsInsecure != 0 }
func (o Options) PreserveOriginal() bool  { return o&(OptionsPreserveOriginal|OptionsDebug) != 0 }
func (o Options) ProxyPreserveHost() bool { return o&OptionsProxyPreserveHost != 0 }
func (o Options) Debug() bool             { return o&OptionsDebug != 0 }

// Priority routes are custom route implementations that are matched against
// each request before the routes in the general lookup tree.
type PriorityRoute interface {

	// If the request is matched, returns a route, otherwise nil.
	// Additionally it may return a parameter map used by the filters
	// in the route.
	Match(*http.Request) (*routing.Route, map[string]string)
}

type flusherWriter interface {
	http.Flusher
	io.Writer
}

// a byte buffer implementing the Closer interface
type bodyBuffer struct {
	*bytes.Buffer
}

type proxy struct {
	routing        *routing.Routing
	roundTripper   http.RoundTripper
	priorityRoutes []PriorityRoute
	options        Options
	metrics        *metrics.Metrics
}

type filterContext struct {
	w                  http.ResponseWriter
	req                *http.Request
	res                *http.Response
	served             bool
	servedWithResponse bool // to support the deprecated way independently
	pathParams         map[string]string
	stateBag           map[string]interface{}
	originalRequest    *http.Request
	originalResponse   *http.Response
	backendUrl         string
	outgoingHost       string
}

func (sb bodyBuffer) Close() error {
	return nil
}

func copyHeader(to, from http.Header) {
	for k, v := range from {
		to[http.CanonicalHeaderKey(k)] = v
	}
}

func cloneHeader(h http.Header) http.Header {
	hh := make(http.Header)
	copyHeader(hh, h)
	return hh
}

// copies a stream with flushing on every successful read operation
// (similar to io.Copy but with flushing)
func copyStream(to flusherWriter, from io.Reader) error {
	b := make([]byte, proxyBufferSize)

	for {
		l, rerr := from.Read(b)
		if rerr != nil && rerr != io.EOF {
			return rerr
		}

		if l > 0 {
			_, werr := to.Write(b[:l])
			if werr != nil {
				return werr
			}

			to.Flush()
		}

		if rerr == io.EOF {
			return nil
		}
	}
}

// creates an outgoing http request to be forwarded to the route endpoint
// based on the augmented incoming request
func mapRequest(r *http.Request, rt *routing.Route, host string) (*http.Request, error) {
	u := r.URL
	u.Scheme = rt.Scheme
	u.Host = rt.Host

	rr, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}

	rr.Header = cloneHeader(r.Header)
	rr.Host = host

	return rr, nil
}

// Creates a proxy. It expects a routing instance that is used to match
// the incoming requests to routes. If the 'insecure' parameter is true, the
// proxy skips the TLS verification for the requests made to the
// route backends. It accepts an optional list of priority routes to
// be used for matching before the general lookup tree.
func New(r *routing.Routing, options Options, pr ...PriorityRoute) http.Handler {
	tr := &http.Transport{}
	if options.Insecure() {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := metrics.Default
	if options.Debug() {
		m = metrics.Void
	}

	return &proxy{r, tr, pr, options, m}
}

// calls a function with recovering from panics and logging them
func callSafe(p func(), onErr func(err interface{})) {
	defer func() {
		if err := recover(); err != nil {
			onErr(err)
		}
	}()

	p()
}

func (p *proxy) newFilterContext(
	w http.ResponseWriter,
	r *http.Request,
	params map[string]string,
	route *routing.Route) *filterContext {

	c := &filterContext{
		w:          w,
		req:        r,
		pathParams: params,
		stateBag:   make(map[string]interface{}),
		backendUrl: route.Backend}

	if p.options.PreserveOriginal() {
		c.originalRequest = cloneRequestMetadata(r)
	}

	if p.options.ProxyPreserveHost() {
		c.outgoingHost = r.Host
	} else {
		c.outgoingHost = route.Host
	}

	return c
}

func cloneUrl(u *url.URL) *url.URL {
	uc := *u
	return &uc
}

func cloneRequestMetadata(r *http.Request) *http.Request {
	return &http.Request{
		Method:           r.Method,
		URL:              cloneUrl(r.URL),
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Body:             &bodyBuffer{&bytes.Buffer{}},
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
		TLS:              r.TLS}
}

func cloneResponseMetadata(r *http.Response) *http.Response {
	return &http.Response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Body:             &bodyBuffer{&bytes.Buffer{}},
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Request:          r.Request,
		TLS:              r.TLS}
}

func (c *filterContext) ResponseWriter() http.ResponseWriter { return c.w }
func (c *filterContext) Request() *http.Request              { return c.req }
func (c *filterContext) Response() *http.Response            { return c.res }
func (c *filterContext) MarkServed()                         { c.served = true }
func (c *filterContext) Served() bool                        { return c.served }
func (c *filterContext) PathParam(key string) string         { return c.pathParams[key] }
func (c *filterContext) StateBag() map[string]interface{}    { return c.stateBag }
func (c *filterContext) BackendUrl() string                  { return c.backendUrl }
func (c *filterContext) OriginalRequest() *http.Request      { return c.originalRequest }
func (c *filterContext) OriginalResponse() *http.Response    { return c.originalResponse }
func (c *filterContext) OutgoingHost() string                { return c.outgoingHost }
func (c *filterContext) SetOutgoingHost(h string)            { c.outgoingHost = h }

func (c *filterContext) Serve(res *http.Response) {
	res.Request = c.Request()

	if res.Header == nil {
		res.Header = make(http.Header)
	}

	if res.Body == nil {
		res.Body = &bodyBuffer{&bytes.Buffer{}}
	}

	c.servedWithResponse = true
	c.res = res
}

// creates an empty shunt response with the initial status code of 404
func shunt(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       &bodyBuffer{&bytes.Buffer{}},
		Request:    r}
}

// applies all filters to a request
func (p *proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx *filterContext, onErr func(err interface{})) []*routing.RouteFilter {
	var start time.Time
	var filters = make([]*routing.RouteFilter, 0, len(f))
	for _, fi := range f {
		start = time.Now()
		callSafe(func() { fi.Request(ctx) }, onErr)
		p.metrics.MeasureFilterRequest(fi.Name, start)
		filters = append(filters, fi)
		if ctx.served || ctx.servedWithResponse {
			break
		}
	}
	return filters
}

// executes an http roundtrip to a route backend
func (p *proxy) roundtrip(r *http.Request, rt *routing.Route, host string) (*http.Response, error) {
	rr, err := mapRequest(r, rt, host)
	if err != nil {
		return nil, err
	}

	return p.roundTripper.RoundTrip(rr)
}

// applies filters to a response in reverse order
func (p *proxy) applyFiltersToResponse(filters []*routing.RouteFilter, ctx filters.FilterContext, onErr func(err interface{})) {
	count := len(filters)
	var start time.Time
	for i, _ := range filters {
		fi := filters[count-1-i]
		start = time.Now()
		callSafe(func() { fi.Response(ctx) }, onErr)
		p.metrics.MeasureFilterResponse(fi.Name, start)
	}
}

// addBranding overwrites any existing `X-Powered-By` or `Server` header from headerMap
func addBranding(headerMap http.Header) {
	headerMap.Set("X-Powered-By", "Skipper")
	headerMap.Set("Server", "Skipper")
}

func (p *proxy) lookupRoute(r *http.Request) (rt *routing.Route, params map[string]string) {
	for _, prt := range p.priorityRoutes {
		rt, params = prt.Match(r)
		if rt != nil {
			return rt, params
		}
	}

	return p.routing.Route(r)
}

// send a premature error response
func sendError(w http.ResponseWriter, error string, code int) {
	http.Error(w, error, code)
	addBranding(w.Header())
}

// http.Handler implementation
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rt, params := p.lookupRoute(r)
	if rt == nil {
		if p.options.Debug() {
			dbgResponse(w, &debugInfo{
				incoming: r,
				response: &http.Response{StatusCode: http.StatusNotFound}})
			return
		}

		p.metrics.IncRoutingFailures()
		sendError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Debugf("Could not find a route for %v", r.URL)
		return
	}
	p.metrics.MeasureRouteLookup(start)

	start = time.Now()
	routeFilters := rt.Filters
	c := p.newFilterContext(w, r, params, rt)

	var (
		onErr        func(err interface{})
		filterPanics []interface{}
	)
	if p.options.Debug() {
		onErr = func(err interface{}) {
			filterPanics = append(filterPanics, err)
		}
	} else {
		onErr = func(err interface{}) {
			log.Error("filter", err)
		}
	}

	processedFilters := p.applyFiltersToRequest(routeFilters, c, onErr)
	p.metrics.MeasureAllFiltersRequest(rt.Id, start)

	var debugReq *http.Request
	if !c.served && !c.servedWithResponse {
		var (
			rs  *http.Response
			err error
		)

		start = time.Now()
		if rt.Shunt {
			rs = shunt(r)
		} else if p.options.Debug() {
			debugReq, err = mapRequest(r, rt, c.outgoingHost)
			if err != nil {
				dbgResponse(w, &debugInfo{
					route:        &rt.Route,
					incoming:     c.OriginalRequest(),
					response:     &http.Response{StatusCode: http.StatusInternalServerError},
					err:          err,
					filterPanics: filterPanics})
				return
			}

			rs = &http.Response{Header: make(http.Header)}
		} else {
			rs, err = p.roundtrip(r, rt, c.outgoingHost)
			if err != nil {
				p.metrics.IncErrorsBackend(rt.Id)
				sendError(w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
				log.Error(err)
				return
			}

			defer func() {
				err = rs.Body.Close()
				if err != nil {
					log.Error(err)
				}
			}()
		}

		p.metrics.MeasureBackend(rt.Id, start)
		c.res = rs
	}

	start = time.Now()
	if !c.served && p.options.PreserveOriginal() {
		c.originalResponse = cloneResponseMetadata(c.Response())
	}
	p.applyFiltersToResponse(processedFilters, c, onErr)
	p.metrics.MeasureAllFiltersResponse(rt.Id, start)

	if !c.served {
		if p.options.Debug() {
			dbgResponse(w, &debugInfo{
				route:        &rt.Route,
				incoming:     c.OriginalRequest(),
				outgoing:     debugReq,
				response:     c.Response(),
				filterPanics: filterPanics})
			return
		}

		response := c.Response()
		start = time.Now()
		addBranding(response.Header)
		copyHeader(w.Header(), response.Header)
		w.WriteHeader(response.StatusCode)
		err := copyStream(w.(flusherWriter), response.Body)
		if err != nil {
			p.metrics.IncErrorsStreaming(rt.Id)
			log.Error(err)
		} else {
			p.metrics.MeasureResponse(response.StatusCode, r.Method, rt.Id, start)
		}
	}
}
