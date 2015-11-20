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
)

func (o Options) Insecure() bool {
	return o&OptionsInsecure != 0
}

func (o Options) PreserveOriginal() bool {
	return o&OptionsPreserveOriginal != 0
}

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
	routing          *routing.Routing
	roundTripper     http.RoundTripper
	priorityRoutes   []PriorityRoute
	preserveOriginal bool
}

type filterContext struct {
	w                http.ResponseWriter
	req              *http.Request
	res              *http.Response
	served           bool
	pathParams       map[string]string
	stateBag         map[string]interface{}
	originalRequest  *http.Request
	originalResponse *http.Response
	backendUrl       string
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
func mapRequest(r *http.Request, rt *routing.Route) (*http.Request, error) {
	u := r.URL
	u.Scheme = rt.Scheme
	u.Host = rt.Host

	rr, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}

	rr.Header = cloneHeader(r.Header)
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

	return &proxy{r, tr, pr, options.PreserveOriginal()}
}

// calls a function with recovering from panics and logging them
func callSafe(p func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Error("filter", err)
		}
	}()

	p()
}

func newFilterContext(
	w http.ResponseWriter,
	r *http.Request,
	params map[string]string,
	preserveOriginal bool,
	route *routing.Route) *filterContext {

	c := &filterContext{
		w:          w,
		req:        r,
		pathParams: params,
		stateBag:   make(map[string]interface{}),
		backendUrl: route.Backend}
	if preserveOriginal {
		c.originalRequest = cloneRequestMetadata(r)
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

func (c *filterContext) OriginalRequest() *http.Request {
	return c.originalRequest
}

func (c *filterContext) OriginalResponse() *http.Response {
	return c.originalResponse
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
func (p *proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx filters.FilterContext) {
	for _, fi := range f {
		metrics.MeasureFilterRequest(fi.Name, func() {
			callSafe(func() { fi.Request(ctx) })
		})
	}
}

// executes an http roundtrip to a route backend
func (p *proxy) roundtrip(r *http.Request, rt *routing.Route) (*http.Response, error) {
	rr, err := mapRequest(r, rt)
	if err != nil {
		return nil, err
	}

	return p.roundTripper.RoundTrip(rr)
}

// applies all filters to a response
func (p *proxy) applyFiltersToResponse(f []*routing.RouteFilter, ctx filters.FilterContext) {
	count := len(f)
	for i, _ := range f {
		fi := f[count-1-i]
		metrics.MeasureFilterResponse(fi.Name, func() {
			callSafe(func() { fi.Response(ctx) })
		})
	}
}

func addBranding(rs *http.Response) {
	rs.Header.Set("X-Powered-By", "Skipper")
	rs.Header.Set("Server", "Skipper")
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

// http.Handler implementation
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rt, params := p.lookupRoute(r)
	if rt == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	metrics.MeasureRouteLookup(start)

	f := rt.Filters
	c := newFilterContext(w, r, params, p.preserveOriginal, rt)
	metrics.MeasureAllFiltersRequest(rt.Id, func() {
		p.applyFiltersToRequest(f, c)
	})

	start = time.Now()
	var (
		rs  *http.Response
		err error
	)
	if rt.Shunt {
		rs = shunt(r)
	} else {
		rs, err = p.roundtrip(r, rt)
		if err != nil {
			http.Error(w,
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
	addBranding(rs)
	metrics.MeasureBackend(rt.Id, start)

	metrics.MeasureAllFiltersResponse(rt.Id, func() {
		c.res = rs
		if p.preserveOriginal {
			c.originalResponse = cloneResponseMetadata(rs)
		}

		p.applyFiltersToResponse(f, c)
	})

	if !c.Served() {
		metrics.MeasureResponse(rs.StatusCode, r.Method, rt.Id, func() {
			copyHeader(w.Header(), rs.Header)
			w.WriteHeader(rs.StatusCode)
			err := copyStream(w.(flusherWriter), rs.Body)
			if err != nil {
				log.Error(err)
			}
		})
	}
}
