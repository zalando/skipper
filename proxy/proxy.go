package proxy

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

const (
	proxyBufferSize = 8192
	proxyErrorFmt   = "proxy: %s"
	unknownRouteID  = "_unknownroute_"

	// Number of loops allowed by default.
	DefaultMaxLoopbacks = 9

	// The default value set for http.Transport.MaxIdleConnsPerHost.
	DefaultIdleConnsPerHost = 64

	// The default period at which the idle connections are forcibly
	// closed.
	DefaultCloseIdleConnsPeriod = 20 * time.Second
)

// Flags control the behavior of the proxy.
type Flags uint

const (
	FlagsNone Flags = 0

	// Insecure causes the proxy to ignore the verification of
	// the TLS certificates of the backend services.
	Insecure Flags = 1 << iota

	// PreserveOriginal indicates that filters require the
	// preserved original metadata of the request and the response.
	PreserveOriginal

	// PreserveHost indicates whether the outgoing request to the
	// backend should use by default the 'Host' header of the incoming
	// request, or the host part of the backend address, in case filters
	// don't change it.
	PreserveHost

	// Debug indicates that the current proxy instance will be used as a
	// debug proxy. Debug proxies don't forward the request to the
	// route backends, but they execute all filters, and return a
	// JSON document with the changes the filters make to the request
	// and with the approximate changes they would make to the
	// response.
	Debug
)

// Options are deprecated alias for Flags.
type Options Flags

const (
	OptionsNone             = Options(FlagsNone)
	OptionsInsecure         = Options(Insecure)
	OptionsPreserveOriginal = Options(PreserveOriginal)
	OptionsPreserveHost     = Options(PreserveHost)
	OptionsDebug            = Options(Debug)
)

// Proxy initialization options.
type Params struct {
	// The proxy expects a routing instance that is used to match
	// the incoming requests to routes.
	Routing *routing.Routing

	// Control flags. See the Flags values.
	Flags Flags

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

	// MaxLoopbacks sets the maximum number of allowed loops. If 0
	// the default (9) is applied. To disable looping, set it to
	// -1. Note, that disabling looping by this option, may result
	// wrong routing depending on the current configuration.
	MaxLoopbacks int

	CircuitBreakers *circuit.Registry
}

var (
	errMaxLoopbacksReached = errors.New("max loopbacks reached")
	errCircuitBreakerOpen  = &proxyError{
		err:  errors.New("circuit breaker open"),
		code: http.StatusServiceUnavailable,
	}
)

// When set, the proxy will skip the TLS verification on outgoing requests.
func (f Flags) Insecure() bool { return f&Insecure != 0 }

// When set, the filters will recieve an unmodified clone of the original
// incoming request and response.
func (f Flags) PreserveOriginal() bool { return f&(PreserveOriginal|Debug) != 0 }

// When set, the proxy will set the, by default, the Host header value
// of the outgoing requests to the one of the incoming request.
func (f Flags) PreserveHost() bool { return f&PreserveHost != 0 }

// When set, the proxy runs in debug mode.
func (f Flags) Debug() bool { return f&Debug != 0 }

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

// Proxy instances implement Skipper proxying functionality. For
// initializing, see the WithParams the constructor and Params.
type Proxy struct {
	routing             *routing.Routing
	roundTripper        *http.Transport
	priorityRoutes      []PriorityRoute
	flags               Flags
	metrics             *metrics.Metrics
	quit                chan struct{}
	flushInterval       time.Duration
	experimentalUpgrade bool
	maxLoops            int
	breakers            *circuit.Registry
}

// proxyError is used to wrap errors during proxying and to indicate
// the required status code for the response sent from the main
// ServeHTTP method. Alternatively, it can indicate that the request
// was already handled, e.g. in case of deprecated shunting or the
// upgrade request.
type proxyError struct {
	err     error
	code    int
	handled bool
}

func (e *proxyError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}

	if e.handled {
		return "request handled in a non-standard way"
	}

	code := e.code
	if code == 0 {
		code = http.StatusInternalServerError
	}

	return fmt.Sprintf("proxy error: %d", code)
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

	body := r.Body
	if r.ContentLength == 0 {
		body = nil
	}

	rr, err := http.NewRequest(r.Method, u.String(), body)
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

// New returns an initialized Proxy.
// Deprecated, see WithParams and Params instead.
func New(r *routing.Routing, options Options, pr ...PriorityRoute) *Proxy {
	return WithParams(Params{
		Routing:              r,
		Flags:                Flags(options),
		PriorityRoutes:       pr,
		CloseIdleConnsPeriod: -time.Second})
}

// WithParams returns an initialized Proxy.
func WithParams(p Params) *Proxy {
	if p.IdleConnectionsPerHost <= 0 {
		p.IdleConnectionsPerHost = DefaultIdleConnsPerHost
	}

	if p.CloseIdleConnsPeriod == 0 {
		p.CloseIdleConnsPeriod = DefaultCloseIdleConnsPeriod
	}

	tr := &http.Transport{MaxIdleConnsPerHost: p.IdleConnectionsPerHost}
	quit := make(chan struct{})
	if p.CloseIdleConnsPeriod > 0 {
		go func() {
			for {
				select {
				case <-time.After(p.CloseIdleConnsPeriod):
					tr.CloseIdleConnections()
				case <-quit:
					return
				}
			}
		}()
	}

	if p.Flags.Insecure() {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := metrics.Default
	if p.Flags.Debug() {
		m = metrics.Void
	}

	if p.MaxLoopbacks == 0 {
		p.MaxLoopbacks = DefaultMaxLoopbacks
	} else if p.MaxLoopbacks < 0 {
		p.MaxLoopbacks = 0
	}

	return &Proxy{
		routing:             p.Routing,
		roundTripper:        tr,
		priorityRoutes:      p.PriorityRoutes,
		flags:               p.Flags,
		metrics:             m,
		quit:                quit,
		flushInterval:       p.FlushInterval,
		experimentalUpgrade: p.ExperimentalUpgrade,
		maxLoops:            p.MaxLoopbacks,
		breakers:            p.CircuitBreakers,
	}
}

func tryCatch(p func(), onErr func(err interface{})) {
	defer func() {
		if err := recover(); err != nil {
			onErr(err)
		}
	}()

	p()
}

// applies filters to a request
func (p *Proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx *context) []*routing.RouteFilter {
	filtersStart := time.Now()

	var filters = make([]*routing.RouteFilter, 0, len(f))
	for _, fi := range f {
		start := time.Now()
		tryCatch(func() {
			fi.Request(ctx)
			p.metrics.MeasureFilterRequest(fi.Name, start)
		}, func(err interface{}) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			log.Errorf("error while processing filter during request: %s: %v", fi.Name, err)
		})

		filters = append(filters, fi)
		if ctx.deprecatedShunted() || ctx.shunted() {
			break
		}
	}

	p.metrics.MeasureAllFiltersRequest(ctx.route.Id, filtersStart)
	return filters
}

// applies filters to a response in reverse order
func (p *Proxy) applyFiltersToResponse(filters []*routing.RouteFilter, ctx *context) {
	filtersStart := time.Now()

	count := len(filters)
	for i := range filters {
		fi := filters[count-1-i]
		start := time.Now()
		tryCatch(func() {
			fi.Response(ctx)
			p.metrics.MeasureFilterResponse(fi.Name, start)
		}, func(err interface{}) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			log.Errorf("error while processing filters during response: %s: %v", fi.Name, err)
		})
	}

	p.metrics.MeasureAllFiltersResponse(ctx.route.Id, filtersStart)
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
func (p *Proxy) sendError(c *context, id string, code int) {
	addBranding(c.responseWriter.Header())
	http.Error(c.responseWriter, http.StatusText(code), code)
	p.metrics.MeasureServe(
		id,
		c.metricsHost(),
		c.request.Method,
		code,
		c.startServe,
	)
}

func (p *Proxy) makeUpgradeRequest(ctx *context, route *routing.Route, req *http.Request) error {
	// have to parse url again, because path is not copied by mapRequest
	backendURL, err := url.Parse(route.Backend)
	if err != nil {
		log.Errorf("can not parse backend %s, caused by: %s", route.Backend, err)
		return &proxyError{
			err:  err,
			code: http.StatusBadGateway,
		}
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(backendURL)
	reverseProxy.FlushInterval = p.flushInterval
	upgradeProxy := upgradeProxy{
		backendAddr:     backendURL,
		reverseProxy:    reverseProxy,
		insecure:        p.flags.Insecure(),
		tlsClientConfig: p.roundTripper.TLSClientConfig,
	}

	upgradeProxy.serveHTTP(ctx.responseWriter, req)
	log.Debugf("finished upgraded protocol %s session", getUpgradeRequest(ctx.request))
	return nil
}

func (p *Proxy) makeBackendRequest(ctx *context) (*http.Response, error) {
	req, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost)
	if err != nil {
		log.Errorf("could not map backend request, caused by: %v", err)
		return nil, err
	}

	if p.experimentalUpgrade && isUpgradeRequest(req) {
		if err := p.makeUpgradeRequest(ctx, ctx.route, req); err != nil {
			return nil, err
		}

		// We are not owner of the connection anymore.
		return nil, &proxyError{handled: true}
	}

	response, err := p.roundTripper.RoundTrip(req)
	if err != nil {
		log.Errorf("error during backend roundtrip: %s: %v", ctx.route.Id, err)
		if _, ok := err.(net.Error); ok {
			err = &proxyError{
				err:  err,
				code: http.StatusServiceUnavailable,
			}
		}

		return nil, err
	}

	return response, nil
}

func (p *Proxy) checkBreaker(c *context) (func(bool), bool) {
	if p.breakers == nil {
		return nil, true
	}

	b := p.breakers.Get(c.breakerSettings())
	if b == nil {
		return nil, true
	}

	return b.Allow()
}

func (p *Proxy) do(ctx *context) error {
	if ctx.loopCounter > p.maxLoops {
		return errMaxLoopbacksReached
	}

	ctx.loopCounter++

	lookupStart := time.Now()
	route, params := p.lookupRoute(ctx.request)
	p.metrics.MeasureRouteLookup(lookupStart)

	if route == nil {
		if !p.flags.Debug() {
			p.metrics.IncRoutingFailures()
		}

		log.Debugf("could not find a route for %v", ctx.request.URL)
		return &proxyError{code: http.StatusNotFound}
	}

	ctx.applyRoute(route, params, p.flags.PreserveHost())

	processedFilters := p.applyFiltersToRequest(ctx.route.Filters, ctx)

	if ctx.deprecatedShunted() {
		log.Debug("deprecated shunting detected in route: %s", ctx.route.Id)
		return &proxyError{handled: true}
	} else if ctx.shunted() || ctx.route.Shunt || ctx.route.BackendType == eskip.ShuntBackend {
		ctx.ensureDefaultResponse()
	} else if ctx.route.BackendType == eskip.LoopBackend {
		loopCTX := ctx.clone()
		if err := p.do(loopCTX); err != nil {
			return err
		}

		ctx.setResponse(loopCTX.response, p.flags.PreserveOriginal())
	} else if p.flags.Debug() {
		debugReq, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost)
		if err != nil {
			return &proxyError{err: err}
		}

		ctx.outgoingDebugRequest = debugReq
		ctx.setResponse(&http.Response{Header: make(http.Header)}, p.flags.PreserveOriginal())
	} else {
		done, allow := p.checkBreaker(ctx)
		if !allow {
			return errCircuitBreakerOpen
		}

		backendStart := time.Now()
		rsp, err := p.makeBackendRequest(ctx)
		if err != nil {
			if done != nil {
				done(false)
			}

			p.metrics.IncErrorsBackend(ctx.route.Id)
			return err
		}

		if done != nil {
			done(rsp.StatusCode < http.StatusInternalServerError)
		}

		ctx.setResponse(rsp, p.flags.PreserveOriginal())
		p.metrics.MeasureBackend(ctx.route.Id, backendStart)
		p.metrics.MeasureBackendHost(ctx.route.Host, backendStart)
	}

	p.applyFiltersToResponse(processedFilters, ctx)
	return nil
}

func (p *Proxy) serveResponse(ctx *context) {
	if p.flags.Debug() {
		dbgResponse(ctx.responseWriter, &debugInfo{
			route:        &ctx.route.Route,
			incoming:     ctx.originalRequest,
			outgoing:     ctx.outgoingDebugRequest,
			response:     ctx.response,
			filterPanics: ctx.debugFilterPanics,
		})

		return
	}

	start := time.Now()
	addBranding(ctx.response.Header)
	copyHeader(ctx.responseWriter.Header(), ctx.response.Header)
	ctx.responseWriter.WriteHeader(ctx.response.StatusCode)
	err := copyStream(ctx.responseWriter.(flusherWriter), ctx.response.Body)
	if err != nil {
		p.metrics.IncErrorsStreaming(ctx.route.Id)
		log.Error("error while copying the response stream", err)
	} else {
		p.metrics.MeasureResponse(ctx.response.StatusCode, ctx.request.Method, ctx.route.Id, start)
	}
}

func (p *Proxy) errorResponse(ctx *context, err error) {
	perr, ok := err.(*proxyError)
	if ok && perr.handled {
		return
	}

	id := unknownRouteID
	if ctx.route != nil {
		id = ctx.route.Id
	}

	code := http.StatusInternalServerError
	if ok && perr.code != 0 {
		code = perr.code
	}

	if p.flags.Debug() {
		di := &debugInfo{
			incoming:     ctx.originalRequest,
			outgoing:     ctx.outgoingDebugRequest,
			response:     ctx.response,
			err:          err,
			filterPanics: ctx.debugFilterPanics,
		}

		if ctx.route != nil {
			di.route = &ctx.route.Route
		}

		dbgResponse(ctx.responseWriter, di)
		return
	}

	switch {
	case err == errCircuitBreakerOpen:
		ctx.responseWriter.Header().Set("X-Circuit-Open", "true")
	default:
		log.Errorf("error while proxying, route %s, status code %d: %v", id, code, err)
	}

	p.sendError(ctx, id, code)
}

// http.Handler implementation
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := newContext(w, r, p.flags.PreserveOriginal())
	ctx.startServe = time.Now()

	defer func() {
		if ctx.response != nil && ctx.response.Body != nil {
			err := ctx.response.Body.Close()
			if err != nil {
				log.Error("error during closing the response body", err)
			}
		}
	}()

	err := p.do(ctx)
	if err != nil {
		p.errorResponse(ctx, err)
		return
	}

	p.serveResponse(ctx)
	p.metrics.MeasureServe(
		ctx.route.Id,
		ctx.metricsHost(),
		r.Method,
		ctx.response.StatusCode,
		ctx.startServe,
	)
}

// Close causes the proxy to stop closing idle
// connections and, currently, has no other effect.
// It's primary purpose is to support testing.
func (p *Proxy) Close() error {
	close(p.quit)
	return nil
}
