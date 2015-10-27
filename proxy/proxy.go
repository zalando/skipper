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
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
	"io"
	"log"
	"net/http"
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
type shuntBody struct {
	*bytes.Buffer
}

type proxy struct {
	routing        *routing.Routing
	roundTripper   http.RoundTripper
	priorityRoutes []PriorityRoute
}

type filterContext struct {
	w          http.ResponseWriter
	req        *http.Request
	res        *http.Response
	served     bool
	pathParams map[string]string
	stateBag   map[string]interface{}
}

func (sb shuntBody) Close() error {
	return nil
}

// creates a formatted error
func proxyError(m string) error {
	return fmt.Errorf(proxyErrorFmt, m)
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
func New(r *routing.Routing, insecure bool, pr ...PriorityRoute) http.Handler {
	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &proxy{r, tr, pr}
}

// calls a function with recovering from panics and logging them
func callSafe(p func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("filter", err)
		}
	}()

	p()
}

// applies all filters to a request
func applyFiltersToRequest(f []filters.Filter, ctx filters.FilterContext) {
	for _, fi := range f {
		callSafe(func() { fi.Request(ctx) })
	}
}

// applies all filters to a response
func applyFiltersToResponse(f []filters.Filter, ctx filters.FilterContext) {
	for i, _ := range f {
		fi := f[len(f)-1-i]
		callSafe(func() { fi.Response(ctx) })
	}
}

func (c *filterContext) ResponseWriter() http.ResponseWriter { return c.w }
func (c *filterContext) Request() *http.Request              { return c.req }
func (c *filterContext) Response() *http.Response            { return c.res }
func (c *filterContext) MarkServed()                         { c.served = true }
func (c *filterContext) Served() bool                        { return c.served }
func (c *filterContext) PathParam(key string) string         { return c.pathParams[key] }
func (c *filterContext) StateBag() map[string]interface{}    { return c.stateBag }

// creates an empty shunt response with the status code initially 404
func shunt(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       &shuntBody{&bytes.Buffer{}},
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

// http.Handler implementation
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		// todo: just a bet that we shouldn't send here 50x
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Println(err)
	}

	var (
		rt     *routing.Route
		params map[string]string
	)

	for _, prt := range p.priorityRoutes {
		rt, params = prt.Match(r)
		if rt != nil {
			break
		}
	}

	if rt == nil {
		rt, params = p.routing.Route(r)
		if rt == nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
	}

	f := rt.Filters
	c := &filterContext{w, r, nil, false, params, make(map[string]interface{})}
	applyFiltersToRequest(f, c)

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
				log.Println(err)
			}
		}()
	}

	addBranding(rs)

	c.res = rs
	applyFiltersToResponse(f, c)

	if !c.Served() {
		copyHeader(w.Header(), rs.Header)
		w.WriteHeader(rs.StatusCode)
		copyStream(w.(flusherWriter), rs.Body)
	}
}

func addBranding(rs *http.Response) {
	rs.Header.Set("X-Powered-By", "Skipper")
	rs.Header.Set("Server", "Skipper")
}
