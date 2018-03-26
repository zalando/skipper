package proxy

import (
	stdlibcontext "context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	circuitfilters "github.com/zalando/skipper/filters/circuit"
	ratelimitfilters "github.com/zalando/skipper/filters/ratelimit"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/ratelimit"
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

	// HopeHeadersRemoval indicates whether the Hop Headers should be removed
	// in compliance with RFC 2616
	HopHeadersRemoval
)

// Options are deprecated alias for Flags.
type Options Flags

const (
	OptionsNone              = Options(FlagsNone)
	OptionsInsecure          = Options(Insecure)
	OptionsPreserveOriginal  = Options(PreserveOriginal)
	OptionsPreserveHost      = Options(PreserveHost)
	OptionsDebug             = Options(Debug)
	OptionsHopHeadersRemoval = Options(HopHeadersRemoval)
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

	// Enable the experimental upgrade protocol feature
	ExperimentalUpgrade bool

	// MaxLoopbacks sets the maximum number of allowed loops. If 0
	// the default (9) is applied. To disable looping, set it to
	// -1. Note, that disabling looping by this option, may result
	// wrong routing depending on the current configuration.
	MaxLoopbacks int

	// CircuitBreakers provides a registry that skipper can use to
	// find the matching circuit breaker for backend requests. If not
	// set, no circuit breakers are used.
	CircuitBreakers *circuit.Registry

	// DefaultHTTPStatus is the HTTP status used when no routes are found
	// for a request.
	DefaultHTTPStatus int

	// RateLimiters provides a registry that skipper can use to
	// find the matching ratelimiter for backend requests. If not
	// set, no ratelimits are used.
	RateLimiters *ratelimit.Registry

	// OpenTracer holds the tracer enabled for this proxy instance
	OpenTracer ot.Tracer

	// Loadbalancer to report unhealthy or dead backends to
	LoadBalancer *loadbalancer.LB

	// Timeout sets the TCP client connection timeout for proxy http connections to the backend
	Timeout time.Duration

	// KeepAlive sets the TCP keepalive for proxy http connections to the backend
	KeepAlive time.Duration

	// DualStack sets if the proxy TCP connections to the backend should be dual stack
	DualStack bool

	// TLSHandshakeTimeout sets the TLS handshake timeout for proxy connections to the backend
	TLSHandshakeTimeout time.Duration

	// MaxIdleConns limits the number of idle connections to all backends, 0 means no limit
	MaxIdleConns int
}

var (
	errMaxLoopbacksReached = errors.New("max loopbacks reached")
	errRouteLookupFailed   = &proxyError{err: errors.New("route lookup failed")}
	errCircuitBreakerOpen  = &proxyError{
		err:              errors.New("circuit breaker open"),
		code:             http.StatusServiceUnavailable,
		additionalHeader: http.Header{"X-Circuit-Open": []string{"true"}},
	}
	errRatelimitError = errors.New("ratelimited")
	hopHeaders        = map[string]bool{
		"Connection":          true,
		"Proxy-Connection":    true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                true,
		"Trailer":           true,
		"Transfer-Encoding": true,
		"Upgrade":           true,
	}
)

// When set, the proxy will skip the TLS verification on outgoing requests.
func (f Flags) Insecure() bool { return f&Insecure != 0 }

// When set, the filters will receive an unmodified clone of the original
// incoming request and response.
func (f Flags) PreserveOriginal() bool { return f&(PreserveOriginal|Debug) != 0 }

// When set, the proxy will set the, by default, the Host header value
// of the outgoing requests to the one of the incoming request.
func (f Flags) PreserveHost() bool { return f&PreserveHost != 0 }

// When set, the proxy runs in debug mode.
func (f Flags) Debug() bool { return f&Debug != 0 }

// When set, the proxy will remove the Hop Headers
func (f Flags) HopHeadersRemoval() bool { return f&HopHeadersRemoval != 0 }

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
	metrics             metrics.Metrics
	quit                chan struct{}
	flushInterval       time.Duration
	experimentalUpgrade bool
	maxLoops            int
	breakers            *circuit.Registry
	limiters            *ratelimit.Registry
	log                 logging.Logger
	defaultHTTPStatus   int
	openTracer          ot.Tracer
	lb                  *loadbalancer.LB
}

// proxyError is used to wrap errors during proxying and to indicate
// the required status code for the response sent from the main
// ServeHTTP method. Alternatively, it can indicate that the request
// was already handled, e.g. in case of deprecated shunting or the
// upgrade request.
type proxyError struct {
	err              error
	code             int
	handled          bool
	dialingFailed    bool
	additionalHeader http.Header
}

func (e proxyError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("proxy error with code %d, dialing failed %v, unwrap: %v", e.code, e.DialError(), e.err.Error())
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

// DialError returns true if the error was caused while dialing TCP or
// TLS connections, before HTTP data was sent. It is safe to retry
// a call, if this returns true.
func (e *proxyError) DialError() bool {
	return e.dialingFailed
}

func (e *proxyError) NetError() net.Error {
	if perr, ok := e.err.(net.Error); ok {
		return perr
	}
	return nil
}

func copyHeader(to, from http.Header) {
	for k, v := range from {
		to[http.CanonicalHeaderKey(k)] = v
	}
}

func copyHeaderExcluding(to, from http.Header, excludeHeaders map[string]bool) {
	for k, v := range from {
		// The http package converts header names to their canonical version.
		// Meaning that the lookup below will be done using the canonical version of the header.
		if _, ok := excludeHeaders[k]; !ok {
			to[http.CanonicalHeaderKey(k)] = v
		}
	}
}

func cloneHeader(h http.Header) http.Header {
	hh := make(http.Header)
	copyHeader(hh, h)
	return hh
}

func cloneHeaderExcluding(h http.Header, excludeList map[string]bool) http.Header {
	hh := make(http.Header)
	copyHeaderExcluding(hh, h, excludeList)
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
func mapRequest(r *http.Request, rt *routing.Route, host string, removeHopHeaders bool) (*http.Request, error) {
	u := r.URL
	u.Scheme = rt.Scheme
	u.Host = rt.Host

	body := r.Body
	if r.ContentLength == 0 {
		body = nil
	}

	rr, err := http.NewRequest(r.Method, u.String(), body)
	rr.ContentLength = r.ContentLength
	if err != nil {
		return nil, err
	}

	if removeHopHeaders {
		rr.Header = cloneHeaderExcluding(r.Header, hopHeaders)
	} else {
		rr.Header = cloneHeader(r.Header)
	}
	rr.Host = host

	// If there is basic auth configured in the URL we add them as headers
	if u.User != nil {
		up := u.User.String()
		upBase64 := base64.StdEncoding.EncodeToString([]byte(up))
		rr.Header.Add("Authorization", fmt.Sprintf("Basic %s", upBase64))
	}

	rr = rr.WithContext(r.Context())

	return rr, nil
}

type skipperDialer struct {
	net.Dialer
	f func(ctx stdlibcontext.Context, network, addr string) (net.Conn, error)
}

func newSkipperDialer(d net.Dialer) *skipperDialer {
	return &skipperDialer{
		Dialer: d,
		f:      d.DialContext,
	}
}

// DialContext wraps net.Dialer's DialContext and returns an error,
// that can be checked if it was a Transport (TCP/TLS handshake) error
// or timeout, or a timeout from http, which is not in general
// retrieable.
func (dc *skipperDialer) DialContext(ctx stdlibcontext.Context, network, addr string) (net.Conn, error) {
	span := ot.SpanFromContext(ctx)
	if span != nil {
		span.LogKV("dial_context", "start")
	}
	con, err := dc.f(ctx, network, addr)
	if span != nil {
		span.LogKV("dial_context", "done")
	}
	if err != nil {
		return nil, &proxyError{
			err:           err,
			code:          -1,   // omit 0 handling in proxy.Error()
			dialingFailed: true, // indicate error happened before http
		}
	}
	return con, nil
}

// New returns an initialized Proxy.
// Deprecated, see WithParams and Params instead.
func New(r *routing.Routing, options Options, pr ...PriorityRoute) *Proxy {
	return WithParams(Params{
		Routing:              r,
		Flags:                Flags(options),
		PriorityRoutes:       pr,
		CloseIdleConnsPeriod: -time.Second,
	})
}

// WithParams returns an initialized Proxy.
func WithParams(p Params) *Proxy {
	if p.IdleConnectionsPerHost <= 0 {
		p.IdleConnectionsPerHost = DefaultIdleConnsPerHost
	}

	if p.CloseIdleConnsPeriod == 0 {
		p.CloseIdleConnsPeriod = DefaultCloseIdleConnsPeriod
	}

	if p.OpenTracer == nil {
		p.OpenTracer = &ot.NoopTracer{}
	}

	tr := &http.Transport{
		DialContext: newSkipperDialer(net.Dialer{
			Timeout:   p.Timeout,
			KeepAlive: p.KeepAlive,
			DualStack: p.DualStack,
		}).DialContext,
		TLSHandshakeTimeout: p.TLSHandshakeTimeout,
		//ResponseHeaderTimeout: 60 * time.Second,
		//ExpectContinueTimeout: 30 * time.Second,
		MaxIdleConns:        p.MaxIdleConns,
		MaxIdleConnsPerHost: p.IdleConnectionsPerHost,
		IdleConnTimeout:     p.CloseIdleConnsPeriod,
	}

	quit := make(chan struct{})
	// We need this to reliably fade on DNS change, which is right
	// now not fixed with IdleConnTimeout in the http.Transport.
	// https://github.com/golang/go/issues/23427
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

	defaultHTTPStatus := http.StatusNotFound

	if p.DefaultHTTPStatus >= http.StatusContinue && p.DefaultHTTPStatus <= http.StatusNetworkAuthenticationRequired {
		defaultHTTPStatus = p.DefaultHTTPStatus
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
		limiters:            p.RateLimiters,
		log:                 &logging.DefaultLog{},
		defaultHTTPStatus:   defaultHTTPStatus,
		openTracer:          p.OpenTracer,
		lb:                  p.LoadBalancer,
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
			ctx.setMetricsPrefix(fi.Name)
			fi.Request(ctx)
			p.metrics.MeasureFilterRequest(fi.Name, start)
		}, func(err interface{}) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			p.log.Errorf("error while processing filter during request: %s: %v", fi.Name, err)
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
			ctx.setMetricsPrefix(fi.Name)
			fi.Response(ctx)
			p.metrics.MeasureFilterResponse(fi.Name, start)
		}, func(err interface{}) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			p.log.Errorf("error while processing filters during response: %s: %v", fi.Name, err)
		})
	}

	p.metrics.MeasureAllFiltersResponse(ctx.route.Id, filtersStart)
}

// addBranding overwrites any existing `X-Powered-By` or `Server` header from headerMap
func addBranding(headerMap http.Header) {
	if headerMap.Get("Server") == "" {
		headerMap.Set("Server", "Skipper")
	}
}

func (p *Proxy) lookupRoute(ctx *context) (rt *routing.Route, params map[string]string) {
	for _, prt := range p.priorityRoutes {
		rt, params = prt.Match(ctx.request)
		if rt != nil {
			return rt, params
		}
	}

	return ctx.routeLookup.Do(ctx.request)
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
		p.log.Errorf("can not parse backend %s, caused by: %s", route.Backend, err)
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
	p.log.Debugf("finished upgraded protocol %s session", getUpgradeRequest(ctx.request))
	return nil
}

func (p *Proxy) makeBackendRequest(ctx *context) (*http.Response, *proxyError) {
	req, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost, p.flags.HopHeadersRemoval())
	if err != nil {
		p.log.Errorf("could not map backend request, caused by: %v", err)
		return nil, &proxyError{err: err}
	}

	if p.experimentalUpgrade && isUpgradeRequest(req) {
		if err := p.makeUpgradeRequest(ctx, ctx.route, req); err != nil {
			return nil, &proxyError{err: err}
		}

		// We are not owner of the connection anymore.
		return nil, &proxyError{handled: true}
	}

	ingress := ot.SpanFromContext(req.Context())
	var proxySpan ot.Span
	if ingress == nil {
		proxySpan = p.openTracer.StartSpan("proxy")
	} else {
		proxySpan = p.openTracer.StartSpan("proxy", ot.ChildOf(ingress.Context()))
	}
	ext.SpanKind.Set(proxySpan, ext.SpanKindRPCClientEnum)
	u := cloneURL(req.URL)
	u.RawQuery = ""
	ext.HTTPUrl.Set(proxySpan, u.String())
	ext.HTTPMethod.Set(proxySpan, req.Method)
	proxySpan.SetTag("skipper.route", ctx.route.String())
	defer proxySpan.Finish()

	carrier := ot.HTTPHeadersCarrier(req.Header)
	p.openTracer.Inject(proxySpan.Context(), ot.HTTPHeaders, carrier)

	req = req.WithContext(ot.ContextWithSpan(req.Context(), proxySpan))

	response, err := p.roundTripper.RoundTrip(req)
	if err != nil {
		ext.Error.Set(proxySpan, true)
		proxySpan.LogKV(`error`, err.Error())
		if perr, ok := err.(*proxyError); ok {
			p.log.Errorf("Failed to do backend roundtrip to %s: %v", ctx.route.Backend, perr)
			//p.lb.AddHealthcheck(ctx.route.Backend)
			return nil, perr

		} else if nerr, ok := err.(net.Error); ok {
			p.log.Errorf("net.Error during backend roundtrip to %s: timeout=%v temporary=%v: %v", ctx.route.Backend, nerr.Timeout(), nerr.Temporary(), err)
			//p.lb.AddHealthcheck(ctx.route.Backend)
			if nerr.Timeout() {
				ext.HTTPStatusCode.Set(proxySpan, uint16(http.StatusGatewayTimeout))
				return nil, &proxyError{
					err:  err,
					code: http.StatusGatewayTimeout,
				}
			} else if !nerr.Temporary() {
				ext.HTTPStatusCode.Set(proxySpan, uint16(http.StatusServiceUnavailable))
				return nil, &proxyError{
					err:  err,
					code: http.StatusServiceUnavailable,
				}
			} else {
				ext.HTTPStatusCode.Set(proxySpan, uint16(http.StatusInternalServerError))
				return nil, &proxyError{
					err:  err,
					code: http.StatusInternalServerError,
				}
			}
		}
		p.log.Errorf("error during backend roundtrip: %s: %v", ctx.route.Id, err)
		return nil, &proxyError{err: err}
	}
	ext.HTTPStatusCode.Set(proxySpan, uint16(response.StatusCode))
	return response, nil
}

// checkRatelimit is used in case of a route ratelimit
// configuration. It returns the used ratelimit.Settings and true if
// the request passed in the context should be allowed.
func (p *Proxy) checkRatelimit(ctx *context) (ratelimit.Settings, bool) {
	if p.limiters == nil {
		return ratelimit.Settings{}, true
	}

	settings, ok := ctx.stateBag[ratelimitfilters.RouteSettingsKey].(ratelimit.Settings)
	if !ok {
		return ratelimit.Settings{}, true
	}
	settings.Host = ctx.outgoingHost

	rl := p.limiters.Get(settings)
	if rl == nil {
		return settings, true
	}

	if settings.Lookuper == nil {
		p.log.Error("lookuper is nil")
		return settings, true
	}
	s := settings.Lookuper.Lookup(ctx.Request())

	if s == "" {
		return settings, true
	}
	return settings, rl.Allow(s)
}

func (p *Proxy) checkBreaker(c *context) (func(bool), bool) {
	if p.breakers == nil {
		return nil, true
	}

	settings, _ := c.stateBag[circuitfilters.RouteSettingsKey].(circuit.BreakerSettings)
	settings.Host = c.outgoingHost

	b := p.breakers.Get(settings)
	if b == nil {
		return nil, true
	}

	done, ok := b.Allow()
	return done, ok
}

func ratelimitError(settings ratelimit.Settings) error {
	return &proxyError{
		err:  errRatelimitError,
		code: http.StatusTooManyRequests,
		additionalHeader: http.Header{
			ratelimit.Header: []string{strconv.Itoa(settings.MaxHits * int(time.Hour/settings.TimeWindow))},
		},
	}
}

func (p *Proxy) do(ctx *context) error {
	if ctx.loopCounter > p.maxLoops {
		return errMaxLoopbacksReached
	}

	ctx.loopCounter++

	// proxy global setting
	if settings, ok := p.limiters.Check(ctx.request); !ok {
		p.log.Debugf("proxy.go limiter settings: %s", settings)
		rerr := ratelimitError(settings)
		return rerr
	}

	lookupStart := time.Now()
	route, params := p.lookupRoute(ctx)
	p.metrics.MeasureRouteLookup(lookupStart)

	if route == nil {
		if !p.flags.Debug() {
			p.metrics.IncRoutingFailures()
		}

		p.log.Debugf("could not find a route for %v", ctx.request.URL)
		return errRouteLookupFailed
	}

	ctx.applyRoute(route, params, p.flags.PreserveHost())

	processedFilters := p.applyFiltersToRequest(ctx.route.Filters, ctx)
	// per route rate limit
	if settings, allow := p.checkRatelimit(ctx); !allow {
		rerr := ratelimitError(settings)
		return rerr
	}

	if ctx.deprecatedShunted() {
		p.log.Debug("deprecated shunting detected in route: %s", ctx.route.Id)
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
		debugReq, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost, p.flags.HopHeadersRemoval())
		if err != nil {
			return &proxyError{err: err}
		}

		ctx.outgoingDebugRequest = debugReq
		ctx.setResponse(&http.Response{Header: make(http.Header)}, p.flags.PreserveOriginal())
	} else {
		done, allow := p.checkBreaker(ctx)
		if !allow {
			if span := ot.SpanFromContext(ctx.Request().Context()); span != nil {
				span.LogKV(`circuit_breaker`, `open`)
			}
			return errCircuitBreakerOpen
		}

		backendStart := time.Now()
		rsp, perr := p.makeBackendRequest(ctx)
		if perr != nil {
			if done != nil {
				done(false)
			}

			p.metrics.IncErrorsBackend(ctx.route.Id)

			if perr.DialError() && ctx.route.IsLoadBalanced {
				// here we do a transparent retry, because we know it's safe to do
				origRoute := ctx.route.Me
				if ctx.route.Next != nil && origRoute != ctx.route.Next {
					ctx.route = ctx.route.Next
				} else if ctx.route.Head != nil && origRoute != ctx.route.Head {
					ctx.route = ctx.route.Head
				}

				perr = nil
				var perr2 *proxyError
				rsp, perr2 = p.makeBackendRequest(ctx)
				if perr2 != nil {
					p.log.Errorf("Failed to do backend request to %s, retry failed to %s: %v", origRoute.Backend, ctx.route.Backend, perr2)
					if perr2.code >= http.StatusInternalServerError {
						p.metrics.MeasureBackend5xx(backendStart)
					}
					return perr2.err
				}
				p.log.Infof("Successfully retry to %v, orig %v, code: %d", ctx.route.Backend, origRoute.Backend, rsp.StatusCode)
			} else {
				p.log.Errorf("Failed to do backend request to %s: %v", ctx.route.Backend, perr)
				return perr
			}
		}

		if rsp.StatusCode >= http.StatusInternalServerError {
			p.metrics.MeasureBackend5xx(backendStart)
		}

		if done != nil {
			done(rsp.StatusCode < http.StatusInternalServerError)
		}

		ctx.setResponse(rsp, p.flags.PreserveOriginal())
		p.metrics.MeasureBackend(ctx.route.Id, backendStart)
		p.metrics.MeasureBackendHost(ctx.route.Host, backendStart)
	}

	addBranding(ctx.response.Header)
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
	copyHeader(ctx.responseWriter.Header(), ctx.response.Header)
	ctx.responseWriter.WriteHeader(ctx.response.StatusCode)
	err := copyStream(ctx.responseWriter.(flusherWriter), ctx.response.Body)
	if err != nil {
		p.metrics.IncErrorsStreaming(ctx.route.Id)
		p.log.Error("error while copying the response stream", err)
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
		if perr.code == -1 { // -1 == dial connection refused
			code = http.StatusBadGateway
		} else {
			code = perr.code
		}
	}

	if span := ot.SpanFromContext(ctx.Request().Context()); span != nil {
		ext.HTTPStatusCode.Set(span, uint16(code))
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

	if ok && len(perr.additionalHeader) > 0 {
		copyHeader(ctx.responseWriter.Header(), perr.additionalHeader)

	}
	switch {
	case err == errRouteLookupFailed:
		code = p.defaultHTTPStatus
	case ok && perr.err == errRatelimitError:
		code = perr.code
	default:
		p.log.Errorf("error while proxying, route %s, status code %d: %v", id, code, err)
	}

	p.sendError(ctx, id, code)
}

// http.Handler implementation
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var span ot.Span
	wireContext, err := p.openTracer.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(r.Header))
	if err == nil {
		span = p.openTracer.StartSpan("ingress", ext.RPCServerOption(wireContext))
	} else {
		span = p.openTracer.StartSpan("ingress")
	}
	ext.HTTPUrl.Set(span, r.URL.Path)
	ext.HTTPMethod.Set(span, r.Method)
	defer span.Finish()
	r = r.WithContext(ot.ContextWithSpan(r.Context(), span))

	ctx := newContext(w, r, p.flags.PreserveOriginal(), p.metrics, p.routing.Get())
	ctx.startServe = time.Now()
	ctx.tracer = p.openTracer

	defer func() {
		if ctx.response != nil && ctx.response.Body != nil {
			err := ctx.response.Body.Close()
			if err != nil {
				p.log.Error("error during closing the response body", err)
			}
		}
	}()

	err = p.do(ctx)
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
