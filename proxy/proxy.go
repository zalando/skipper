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
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

const (
	proxyBufferSize = 8192
	proxyErrorFmt   = "proxy: %s"

	// The default value set for http.Transport.MaxIdleConnsPerHost.
	DefaultIdleConnsPerHost = 64

	// The default period at which the idle connections are forcibly
	// closed.
	DefaultCloseIdleConnsPeriod = 12 * time.Second
)

// ProxyFlags control the behavior of the proxy.
type ProxyFlags uint

const (
	ProxyFlagsNone ProxyFlags = 0

	// ProxyInsecure causes the proxy to ignore the verification of
	// the TLS certificates of the backend services.
	ProxyInsecure ProxyFlags = 1 << iota

	// ProxyPreserveOriginal indicates that filters require the
	// preserved original metadata of the request and the response.
	ProxyPreserveOriginal

	// ProxyPreserveHost indicates whether the outgoing request to the
	// backend should use by default the 'Host' header of the incoming
	// request, or the host part of the backend address, in case filters
	// don't change it.
	ProxyPreserveHost

	// ProxyDebug indicates that the current proxy instance will be used as a
	// debug proxy. Debug proxies don't forward the request to the
	// route backends, but they execute all filters, and return a
	// JSON document with the changes the filters make to the request
	// and with the approximate changes they would make to the
	// response.
	ProxyDebug
)

// Options are deprecated alias for ProxyFlags.
type Options ProxyFlags

const (
	OptionsNone              = Options(ProxyFlagsNone)
	OptionsInsecure          = Options(ProxyInsecure)
	OptionsPreserveOriginal  = Options(ProxyPreserveOriginal)
	OptionsProxyPreserveHost = Options(ProxyPreserveHost)
	OptionsDebug             = Options(ProxyDebug)
)

// Proxy initialization options.
type ProxyOptions struct {
	// The proxy expects a routing instance that is used to match
	// the incoming requests to routes.
	Routing *routing.Routing

	// Control flags. See the ProxyFlags values.
	Flags ProxyFlags

	// Same as net/http.Transport.MaxIdleConnsPerHost, but the default
	// is 64. This value supports scenarios with relatively few remote
	// hosts. When the routing table contains different hosts in the
	// range of hundreds, it is recommended to set this options to a
	// lower value.
	IdleConnectionsPerHost int

	// Defines the time period of how often the idle connections are
	// forcibly closed. The default is 12 seconds. When set to less than
	// 0, the proxy doesn't force closing the idle connections.
	CloseIdleConnsPeriod time.Duration

	// And optional list of priority routes to be used for matching
	// before the general lookup tree.
	PriorityRoutes []PriorityRoute

	// The Flush interval for copying upgraded connections
	FlushInterval time.Duration

	// Enable the expiremental upgrade protocol feature
	ExperimentalUpgrade bool
}

// When set, the proxy will skip the TLS verification on outgoing requests.
func (f ProxyFlags) Insecure() bool { return f&ProxyInsecure != 0 }

// When set, the filters will recieve an unmodified clone of the original
// incoming request and response.
func (f ProxyFlags) PreserveOriginal() bool { return f&(ProxyPreserveOriginal|ProxyDebug) != 0 }

// When set, the proxy will set the, by default, the Host header value
// of the outgoing requests to the one of the incoming request.
func (f ProxyFlags) ProxyPreserveHost() bool { return f&ProxyPreserveHost != 0 }

// When set, the proxy runs in debug mode.
func (f ProxyFlags) Debug() bool { return f&ProxyDebug != 0 }

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

// Proxy instances implement Skipper proxying functionality. For
// initializing, see NewProxy and ProyxOptions.
type Proxy struct {
	routing             *routing.Routing
	roundTripper        http.RoundTripper
	priorityRoutes      []PriorityRoute
	flags               ProxyFlags
	metrics             *metrics.Metrics
	quit                chan struct{}
	flushInterval       time.Duration
	experimentalUpgrade bool
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

	backendURL, err := url.Parse(rt.Backend)
	if err != nil {
		log.Fatalf("backendURL %s could not be parsed, caused by: %v", rt.Backend, err)
	}
	u.User = backendURL.User

	rr, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}

	rr.Header = cloneHeader(r.Header)
	rr.Host = host

	// If there is basic auth configured int the URL we add them as headers
	if u.User != nil {
		up := u.User.String()
		upBase64 := base64.StdEncoding.EncodeToString([]byte(up))
		rr.Header.Add("Authorization", fmt.Sprintf("Basic %s", upBase64))
	}

	return rr, nil
}

// Deprecated, see NewProxy and ProxyOptions instead.
func New(r *routing.Routing, options Options, pr ...PriorityRoute) *Proxy {
	return NewProxy(ProxyOptions{
		Routing:              r,
		Flags:                ProxyFlags(options),
		PriorityRoutes:       pr,
		CloseIdleConnsPeriod: -time.Second})
}

// Creates a proxy.
func NewProxy(o ProxyOptions) *Proxy {
	if o.IdleConnectionsPerHost <= 0 {
		o.IdleConnectionsPerHost = DefaultIdleConnsPerHost
	}

	if o.CloseIdleConnsPeriod == 0 {
		o.CloseIdleConnsPeriod = DefaultCloseIdleConnsPeriod
	}

	tr := &http.Transport{MaxIdleConnsPerHost: o.IdleConnectionsPerHost}
	quit := make(chan struct{})
	if o.CloseIdleConnsPeriod > 0 {
		go func() {
			for {
				select {
				case <-time.After(o.CloseIdleConnsPeriod):
					tr.CloseIdleConnections()
				case <-quit:
					return
				}
			}
		}()
	}

	if o.Flags.Insecure() {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := metrics.Default
	if o.Flags.Debug() {
		m = metrics.Void
	}

	return &Proxy{
		routing:             o.Routing,
		roundTripper:        tr,
		priorityRoutes:      o.PriorityRoutes,
		flags:               o.Flags,
		metrics:             m,
		quit:                quit,
		flushInterval:       o.FlushInterval,
		experimentalUpgrade: o.ExperimentalUpgrade}
}

// calls a function with recovering from panics and logging them
func tryCatch(p func(), onErr func(err interface{})) {
	defer func() {
		if err := recover(); err != nil {
			onErr(err)
		}
	}()

	p()
}

func (p *Proxy) newFilterContext(
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

	if p.flags.PreserveOriginal() {
		c.originalRequest = cloneRequestMetadata(r)
	}

	if p.flags.ProxyPreserveHost() {
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
func (c *filterContext) Served() bool                        { return c.served || c.servedWithResponse }
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
func (p *Proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx *filterContext, onErr func(err interface{})) []*routing.RouteFilter {
	var start time.Time
	var filters = make([]*routing.RouteFilter, 0, len(f))
	for _, fi := range f {
		start = time.Now()
		tryCatch(func() { fi.Request(ctx) }, onErr)
		p.metrics.MeasureFilterRequest(fi.Name, start)
		filters = append(filters, fi)
		if ctx.served || ctx.servedWithResponse {
			break
		}
	}
	return filters
}

// applies filters to a response in reverse order
func (p *Proxy) applyFiltersToResponse(filters []*routing.RouteFilter, ctx filters.FilterContext, onErr func(err interface{})) {
	count := len(filters)
	var start time.Time
	for i, _ := range filters {
		fi := filters[count-1-i]
		start = time.Now()
		tryCatch(func() { fi.Response(ctx) }, onErr)
		p.metrics.MeasureFilterResponse(fi.Name, start)
	}
}

// addBranding overwrites any existing `X-Powered-By` or `Server` header from headerMap
func addBranding(headerMap http.Header) {
	headerMap.Set("X-Powered-By", "Skipper")
	headerMap.Set("Server", "Skipper")
}

func (p *Proxy) lookupRoute(r *http.Request) (rt *routing.Route, params map[string]string) {
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
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rt, params := p.lookupRoute(r)
	if rt == nil {
		if p.flags.Debug() {
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
	if p.flags.Debug() {
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
		} else if p.flags.Debug() {
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

			rr, err := mapRequest(r, rt, c.outgoingHost)
			if err != nil {
				log.Errorf("Could not mapRequest, caused by: %v", err)
				return
			}

			if p.experimentalUpgrade && isUpgradeRequest(rr) {
				// have to parse url again, because path is not be copied by mapRequest
				backendURL, err := url.Parse(rt.Backend)
				if err != nil {
					log.Errorf("Can not parse backend %s, caused by: %s", rt.Backend, err)
					sendError(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
					return
				}

				reverseProxy := httputil.NewSingleHostReverseProxy(backendURL)
				reverseProxy.FlushInterval = p.flushInterval
				upgradeProxy := upgradeProxy{
					backendAddr:  backendURL,
					reverseProxy: reverseProxy,
					insecure:     p.flags.Insecure(),
				}
				upgradeProxy.serveHTTP(w, rr)
				log.Debugf("Finished upgraded protocol %s session", getUpgradeRequest(r))
				// We are not owner of the connection anymore.
				return
			}

			rs, err = p.roundTripper.RoundTrip(rr)

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
	if !c.served && p.flags.PreserveOriginal() {
		c.originalResponse = cloneResponseMetadata(c.Response())
	}
	p.applyFiltersToResponse(processedFilters, c, onErr)
	p.metrics.MeasureAllFiltersResponse(rt.Id, start)

	if !c.served {
		if p.flags.Debug() {
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

// Close causes the proxy to stop closing idle
// connections and, currently, has no other effect.
// It's primary purpose is to support testing.
func (p *Proxy) Close() error {
	close(p.quit)
	return nil
}
