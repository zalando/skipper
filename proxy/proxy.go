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
	slog "github.com/zalando/skipper/log"
	"github.com/zalando/skipper/routing"
	"io"
	"net/http"
	"net/url"
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

// applies all filters to a request
func applyFiltersToRequest(f []*routing.RouteFilter, ctx filters.FilterContext) {
	for _, fi := range f {
		// <measure>
		// missing filter name :(
		callSafe(func() { fi.Request(ctx) })
		// </measure>
	}
}

// applies all filters to a response
func applyFiltersToResponse(f []*routing.RouteFilter, ctx filters.FilterContext) {
	count := len(f)
	for i, _ := range f {
		fi := f[count-1-i]
		// <measure>
		callSafe(func() { fi.Response(ctx) })
		// </measure>
	}
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

func (c *filterContext) OriginalRequest() *http.Request {
	return c.originalRequest
}

func (c *filterContext) OriginalResponse() *http.Response {
	return c.originalResponse
}

// creates an empty shunt response with the status code initially 404
func shunt(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       &bodyBuffer{&bytes.Buffer{}},
		Request:    r}
}

// executes an http roundtrip to a route backend
func (p *proxy) roundtrip(r *http.Request, rt *routing.Route) (*http.Response, error) {
	rr, err := mapRequest(r, rt)
	if err != nil {
		return nil, err
	}

	return p.roundTripper.RoundTrip(rr)
}

func addBranding(rs *http.Response) {
	rs.Header.Set("X-Powered-By", "Skipper")
	rs.Header.Set("Server", "Skipper")
}

func (p *proxy) matchAndRoute(r *http.Request) (rt *routing.Route, params map[string]string) {
	for _, prt := range p.priorityRoutes {
		rt, params = prt.Match(r)
		if rt != nil {
			break
		}
	}

	if rt == nil {
		return p.routing.Route(r)
	}

	return nil, nil
}

// http.Handler implementation
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		// todo: just a bet that we shouldn't send here 50x
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Error(err)
	}

	// <measure>
	rt, params := p.matchAndRoute(r)
	if rt == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		println("logging access")
		// TODO: use always the original request, clone it, always
		slog.Access(&slog.AccessEntry{Request: r})
		return
	}
	// </measure>

	// <measure>
	f := rt.Filters
	c := &filterContext{
		w:          w,
		req:        r,
		pathParams: params,
		stateBag:   make(map[string]interface{})}
	if p.preserveOriginal {
		c.originalRequest = cloneRequestMetadata(r)
	}
	applyFiltersToRequest(f, c)
	// </measure>

	// <measure>
	var (
		rs  *http.Response
		err error
	)
	if rt.Shunt {
		rs = shunt(r)
	} else {
		rs, err = p.roundtrip(r, rt)
		if err != nil {
			hterr(err)
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
	// </measure>

	// <measure>
	c.res = rs
	if p.preserveOriginal {
		c.originalResponse = cloneResponseMetadata(rs)
	}
	applyFiltersToResponse(f, c)
	// </measure>

	// <measure>
	if !c.Served() {
		copyHeader(w.Header(), rs.Header)
		w.WriteHeader(rs.StatusCode)
		copyStream(w.(flusherWriter), rs.Body)
	}
	// </measure>
}
