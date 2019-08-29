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
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/aryszka/jobqueue"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	al "github.com/zalando/skipper/filters/accesslog"
	circuitfilters "github.com/zalando/skipper/filters/circuit"
	ratelimitfilters "github.com/zalando/skipper/filters/ratelimit"
	tracingfilter "github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/rfc"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/scheduler"
	"github.com/zalando/skipper/tracing"
)

const (
	proxyBufferSize     = 8192
	unknownRouteID      = "_unknownroute_"
	unknownRouteBackend = "<unknown>"

	// Number of loops allowed by default.
	DefaultMaxLoopbacks = 9

	// The default value set for http.Transport.MaxIdleConnsPerHost.
	DefaultIdleConnsPerHost = 64

	// The default period at which the idle connections are forcibly
	// closed.
	DefaultCloseIdleConnsPeriod = 20 * time.Second

	// DefaultResponseHeaderTimeout, the default response header timeout
	DefaultResponseHeaderTimeout = 60 * time.Second

	// DefaultExpectContinueTimeout, the default timeout to expect
	// a response for a 100 Continue request
	DefaultExpectContinueTimeout = 30 * time.Second
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

	// HopHeadersRemoval indicates whether the Hop Headers should be removed
	// in compliance with RFC 2616
	HopHeadersRemoval

	// PatchPath instructs the proxy to patch the parsed request path
	// if the reserved characters according to RFC 2616 and RFC 3986
	// were unescaped by the parser.
	PatchPath
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

type OpenTracingParams struct {
	// Tracer holds the tracer enabled for this proxy instance
	Tracer ot.Tracer

	// InitialSpan can override the default initial, pre-routing, span name.
	// Default: "ingress".
	InitialSpan string

	// LogFilterEvents enables the behavior to mark start and completion times of filters
	// on the span representing request filters being processed.
	// Default: false
	LogFilterEvents bool

	// LogStreamEvents enables the logs that marks the times when response headers & payload are streamed to
	// the client
	// Default: false
	LogStreamEvents bool

	// ExcludeTags controls what tags are disabled. Any tag that is listed here will be ignored.
	ExcludeTags []string
}

// Proxy initialization options.
type Params struct {
	// The proxy expects a routing instance that is used to match
	// the incoming requests to routes.
	Routing *routing.Routing

	// Control flags. See the Flags values.
	Flags Flags

	// And optional list of priority routes to be used for matching
	// before the general lookup tree.
	PriorityRoutes []PriorityRoute

	// Enable the experimental upgrade protocol feature
	ExperimentalUpgrade bool

	// ExperimentalUpgradeAudit enables audit log of both the request line
	// and the response messages during web socket upgrades.
	ExperimentalUpgradeAudit bool

	// When set, no access log is printed.
	AccessLogDisabled bool

	// DualStack sets if the proxy TCP connections to the backend should be dual stack
	DualStack bool

	// DefaultHTTPStatus is the HTTP status used when no routes are found
	// for a request.
	DefaultHTTPStatus int

	// MaxLoopbacks sets the maximum number of allowed loops. If 0
	// the default (9) is applied. To disable looping, set it to
	// -1. Note, that disabling looping by this option, may result
	// wrong routing depending on the current configuration.
	MaxLoopbacks int

	// Same as net/http.Transport.MaxIdleConnsPerHost, but the default
	// is 64. This value supports scenarios with relatively few remote
	// hosts. When the routing table contains different hosts in the
	// range of hundreds, it is recommended to set this options to a
	// lower value.
	IdleConnectionsPerHost int

	// MaxIdleConns limits the number of idle connections to all backends, 0 means no limit
	MaxIdleConns int

	// CircuitBreakers provides a registry that skipper can use to
	// find the matching circuit breaker for backend requests. If not
	// set, no circuit breakers are used.
	CircuitBreakers *circuit.Registry

	// RateLimiters provides a registry that skipper can use to
	// find the matching ratelimiter for backend requests. If not
	// set, no ratelimits are used.
	RateLimiters *ratelimit.Registry

	// Loadbalancer to report unhealthy or dead backends to
	LoadBalancer *loadbalancer.LB

	// Defines the time period of how often the idle connections are
	// forcibly closed. The default is 12 seconds. When set to less than
	// 0, the proxy doesn't force closing the idle connections.
	CloseIdleConnsPeriod time.Duration

	// The Flush interval for copying upgraded connections
	FlushInterval time.Duration

	// Timeout sets the TCP client connection timeout for proxy http connections to the backend
	Timeout time.Duration

	// ResponseHeaderTimeout sets the HTTP response timeout for
	// proxy http connections to the backend.
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout sets the HTTP timeout to expect a
	// response for status Code 100 for proxy http connections to
	// the backend.
	ExpectContinueTimeout time.Duration

	// KeepAlive sets the TCP keepalive for proxy http connections to the backend
	KeepAlive time.Duration

	// TLSHandshakeTimeout sets the TLS handshake timeout for proxy connections to the backend
	TLSHandshakeTimeout time.Duration

	// Client TLS to connect to Backends
	ClientTLS *tls.Config

	// OpenTracing contains parameters related to OpenTracing instrumentation. For default values
	// check OpenTracingParams
	OpenTracing *OpenTracingParams

	// GlobalLIFO can be used to control all the requests with a common LIFO queue.
	GlobalLIFO *scheduler.Queue
}

type (
	maxLoopbackError string
	ratelimitError   string
	routeLookupError string
)

func (e maxLoopbackError) Error() string { return string(e) }
func (e ratelimitError) Error() string   { return string(e) }
func (e routeLookupError) Error() string { return string(e) }

const (
	errMaxLoopbacksReached = maxLoopbackError("max loopbacks reached")
	errRatelimit           = ratelimitError("ratelimited")
	errRouteLookup         = routeLookupError("route lookup failed")
)

var (
	errRouteLookupFailed  = &proxyError{err: errRouteLookup}
	errCircuitBreakerOpen = &proxyError{
		err:              errors.New("circuit breaker open"),
		code:             http.StatusServiceUnavailable,
		additionalHeader: http.Header{"X-Circuit-Open": []string{"true"}},
	}

	errQueueFull    = &proxyError{err: jobqueue.ErrStackFull, code: http.StatusServiceUnavailable}
	errQueueTimeout = &proxyError{err: jobqueue.ErrTimeout, code: http.StatusBadGateway}

	hostname          = ""
	disabledAccessLog = al.AccessLogFilter{Enable: false, Prefixes: nil}
	enabledAccessLog  = al.AccessLogFilter{Enable: true, Prefixes: nil}
	hopHeaders        = map[string]bool{
		"Te":                  true,
		"Connection":          true,
		"Proxy-Connection":    true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Trailer":             true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
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

func (f Flags) patchPath() bool { return f&PatchPath != 0 }

// Priority routes are custom route implementations that are matched against
// each request before the routes in the general lookup tree.
type PriorityRoute interface {
	// If the request is matched, returns a route, otherwise nil.
	// Additionally it may return a parameter map used by the filters
	// in the route.
	Match(*http.Request) (*routing.Route, map[string]string)
}

// Proxy instances implement Skipper proxying functionality. For
// initializing, see the WithParams the constructor and Params.
type Proxy struct {
	experimentalUpgrade      bool
	experimentalUpgradeAudit bool
	accessLogDisabled        bool
	maxLoops                 int
	defaultHTTPStatus        int
	routing                  *routing.Routing
	roundTripper             *http.Transport
	priorityRoutes           []PriorityRoute
	flags                    Flags
	metrics                  metrics.Metrics
	quit                     chan struct{}
	flushInterval            time.Duration
	breakers                 *circuit.Registry
	lifo                     *scheduler.Queue
	limiters                 *ratelimit.Registry
	log                      logging.Logger
	tracing                  *proxyTracing
	lb                       *loadbalancer.LB
	upgradeAuditLogOut       io.Writer
	upgradeAuditLogErr       io.Writer
	auditLogHook             chan struct{}
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
		return fmt.Sprintf("dialing failed %v: %v", e.DialError(), e.err.Error())
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
func copyStream(to flushedResponseWriter, from io.Reader, tracing *proxyTracing, span ot.Span) error {
	b := make([]byte, proxyBufferSize)

	for {
		l, rerr := from.Read(b)

		tracing.logStreamEvent(span, "streamBody.byte", fmt.Sprintf("%d", l))

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

func setRequestURLFromRequest(u *url.URL, r *http.Request) {
	if u.Host == "" {
		u.Host = r.Host
	}
	if u.Scheme == "" {
		if r.TLS != nil {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}
	}
}

func setRequestURLForDynamicBackend(u *url.URL, stateBag map[string]interface{}) {
	dbu, ok := stateBag[filters.DynamicBackendURLKey].(string)
	if ok && dbu != "" {
		bu, err := url.ParseRequestURI(dbu)
		if err == nil {
			u.Host = bu.Host
			u.Scheme = bu.Scheme
		}
	} else {
		host, ok := stateBag[filters.DynamicBackendHostKey].(string)
		if ok && host != "" {
			u.Host = host
		}

		scheme, ok := stateBag[filters.DynamicBackendSchemeKey].(string)
		if ok && scheme != "" {
			u.Scheme = scheme
		}
	}
}

func setRequestURLForLoadBalancedBackend(u *url.URL, rt *routing.Route, lbctx *routing.LBContext) {
	e := rt.LBAlgorithm.Apply(lbctx)
	u.Scheme = e.Scheme
	u.Host = e.Host
}

// creates an outgoing http request to be forwarded to the route endpoint
// based on the augmented incoming request
func mapRequest(r *http.Request, rt *routing.Route, host string, removeHopHeaders bool, stateBag map[string]interface{}) (*http.Request, error) {
	u := r.URL
	switch rt.BackendType {
	case eskip.DynamicBackend:
		setRequestURLFromRequest(u, r)
		setRequestURLForDynamicBackend(u, stateBag)
	case eskip.LBBackend:
		setRequestURLForLoadBalancedBackend(u, rt, routing.NewLBContext(r, rt))
	default:
		u.Scheme = rt.Scheme
		u.Host = rt.Host
	}

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

	ctxspan := ot.SpanFromContext(r.Context())
	if ctxspan != nil {
		rr = rr.WithContext(ot.ContextWithSpan(rr.Context(), ctxspan))
	}

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
// not possible to retry.
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
	} else if cerr := ctx.Err(); cerr != nil {
		// unclear when this is being triggered
		return nil, &proxyError{
			err:  fmt.Errorf("err from dial context: %v", cerr),
			code: http.StatusGatewayTimeout,
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

	if p.ResponseHeaderTimeout == 0 {
		p.ResponseHeaderTimeout = DefaultResponseHeaderTimeout
	}

	if p.ExpectContinueTimeout == 0 {
		p.ExpectContinueTimeout = DefaultExpectContinueTimeout
	}

	tr := &http.Transport{
		DialContext: newSkipperDialer(net.Dialer{
			Timeout:   p.Timeout,
			KeepAlive: p.KeepAlive,
			DualStack: p.DualStack,
		}).DialContext,
		TLSHandshakeTimeout:   p.TLSHandshakeTimeout,
		ResponseHeaderTimeout: p.ResponseHeaderTimeout,
		ExpectContinueTimeout: p.ExpectContinueTimeout,
		MaxIdleConns:          p.MaxIdleConns,
		MaxIdleConnsPerHost:   p.IdleConnectionsPerHost,
		IdleConnTimeout:       p.CloseIdleConnsPeriod,
	}

	if p.ClientTLS != nil {
		tr.TLSClientConfig = p.ClientTLS
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
		if tr.TLSClientConfig == nil {
			/* #nosec */
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			/* #nosec */
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
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

	hostname = os.Getenv("HOSTNAME")

	return &Proxy{
		routing:                  p.Routing,
		roundTripper:             tr,
		priorityRoutes:           p.PriorityRoutes,
		flags:                    p.Flags,
		metrics:                  m,
		quit:                     quit,
		flushInterval:            p.FlushInterval,
		experimentalUpgrade:      p.ExperimentalUpgrade,
		experimentalUpgradeAudit: p.ExperimentalUpgradeAudit,
		maxLoops:                 p.MaxLoopbacks,
		breakers:                 p.CircuitBreakers,
		lifo:                     p.GlobalLIFO,
		lb:                       p.LoadBalancer,
		limiters:                 p.RateLimiters,
		log:                      &logging.DefaultLog{},
		defaultHTTPStatus:        defaultHTTPStatus,
		tracing:                  newProxyTracing(p.OpenTracing),
		accessLogDisabled:        p.AccessLogDisabled,
		upgradeAuditLogOut:       os.Stdout,
		upgradeAuditLogErr:       os.Stderr,
	}
}

var caughtPanic = false

// tryCatch executes function `p` and `onErr` if `p` panics
// onErr will receive a stack trace string of the first panic
// further panics are ignored for efficiency reasons
func tryCatch(p func(), onErr func(err interface{}, stack string)) {
	defer func() {
		if err := recover(); err != nil {
			s := ""
			if !caughtPanic {
				buf := make([]byte, 1024)
				l := runtime.Stack(buf, false)
				s = string(buf[:l])
				caughtPanic = true
			}
			onErr(err, s)
		}
	}()

	p()
}

// applies filters to a request
func (p *Proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx *context) []*routing.RouteFilter {
	if len(f) == 0 {
		return f
	}

	filtersStart := time.Now()
	filtersSpan := tracing.CreateSpan("request_filters", ctx.request.Context(), p.tracing.tracer)
	defer filtersSpan.Finish()
	ctx.parentSpan = filtersSpan

	var filters = make([]*routing.RouteFilter, 0, len(f))
	for _, fi := range f {
		start := time.Now()
		p.tracing.logFilterStart(filtersSpan, fi.Name)
		tryCatch(func() {
			ctx.setMetricsPrefix(fi.Name)
			fi.Request(ctx)
			p.metrics.MeasureFilterRequest(fi.Name, start)
		}, func(err interface{}, stack string) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			p.log.Errorf("error while processing filter during request: %s: %v (%s)", fi.Name, err, stack)
		})
		p.tracing.logFilterEnd(filtersSpan, fi.Name)

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
	filtersSpan := tracing.CreateSpan("response_filters", ctx.request.Context(), p.tracing.tracer)
	defer filtersSpan.Finish()

	last := len(filters) - 1
	for i := range filters {
		fi := filters[last-i]
		start := time.Now()
		p.tracing.logFilterStart(filtersSpan, fi.Name)
		tryCatch(func() {
			ctx.setMetricsPrefix(fi.Name)
			fi.Response(ctx)
			p.metrics.MeasureFilterResponse(fi.Name, start)
		}, func(err interface{}, stack string) {
			if p.flags.Debug() {
				// these errors are collected for the debug mode to be able
				// to report in the response which filters failed.
				ctx.debugFilterPanics = append(ctx.debugFilterPanics, err)
				return
			}

			p.log.Errorf("error while processing filters during response: %s: %v (%s)", fi.Name, err, stack)
		})
		p.tracing.logFilterEnd(filtersSpan, fi.Name)
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

func (p *Proxy) makeUpgradeRequest(ctx *context, req *http.Request) error {
	backendURL := req.URL

	reverseProxy := httputil.NewSingleHostReverseProxy(backendURL)
	reverseProxy.FlushInterval = p.flushInterval
	upgradeProxy := upgradeProxy{
		backendAddr:     backendURL,
		reverseProxy:    reverseProxy,
		insecure:        p.flags.Insecure(),
		tlsClientConfig: p.roundTripper.TLSClientConfig,
		useAuditLog:     p.experimentalUpgradeAudit,
		auditLogOut:     p.upgradeAuditLogOut,
		auditLogErr:     p.upgradeAuditLogErr,
		auditLogHook:    p.auditLogHook,
	}

	upgradeProxy.serveHTTP(ctx.responseWriter, req)
	p.log.Debugf("finished upgraded protocol %s session", getUpgradeRequest(ctx.request))
	return nil
}

func (p *Proxy) makeBackendRequest(ctx *context) (*http.Response, *proxyError) {
	req, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost, p.flags.HopHeadersRemoval(), ctx.StateBag())
	if err != nil {
		p.log.Errorf("could not map backend request, caused by: %v", err)
		return nil, &proxyError{err: err}
	}

	if p.experimentalUpgrade && isUpgradeRequest(req) {
		if err = p.makeUpgradeRequest(ctx, req); err != nil {
			return nil, &proxyError{err: err}
		}

		// We are not owner of the connection anymore.
		return nil, &proxyError{handled: true}
	}

	bag := ctx.StateBag()
	spanName, ok := bag[tracingfilter.OpenTracingProxySpanKey].(string)
	if !ok {
		spanName = "proxy"
	}
	ctx.proxySpan = tracing.CreateSpan(spanName, req.Context(), p.tracing.tracer)

	p.tracing.
		setTag(ctx.proxySpan, SpanKindTag, SpanKindClient).
		setTag(ctx.proxySpan, SkipperRouteIDTag, ctx.route.Id).
		setTag(ctx.proxySpan, SkipperRouteTag, ctx.route.String())

	u := cloneURL(req.URL)
	u.RawQuery = ""
	p.setCommonSpanInfo(u, req, ctx.proxySpan)

	carrier := ot.HTTPHeadersCarrier(req.Header)
	_ = p.tracing.tracer.Inject(ctx.proxySpan.Context(), ot.HTTPHeaders, carrier)

	req = req.WithContext(ot.ContextWithSpan(req.Context(), ctx.proxySpan))

	p.metrics.IncCounter("outgoing." + req.Proto)
	ctx.proxySpan.LogKV("http_roundtrip", StartEvent)
	response, err := p.roundTripper.RoundTrip(req)
	ctx.proxySpan.LogKV("http_roundtrip", EndEvent)
	if err != nil {
		p.tracing.setTag(ctx.proxySpan, ErrorTag, true)
		ctx.proxySpan.LogKV(
			"event", "error",
			"message", err.Error())

		if perr, ok := err.(*proxyError); ok {
			p.log.Errorf("Failed to do backend roundtrip to %s: %v", ctx.route.Backend, perr)
			//p.lb.AddHealthcheck(ctx.route.Backend)
			return nil, perr

		} else if nerr, ok := err.(net.Error); ok {
			p.log.Errorf("net.Error during backend roundtrip to %s: timeout=%v temporary=%v: %v", ctx.route.Backend, nerr.Timeout(), nerr.Temporary(), err)
			//p.lb.AddHealthcheck(ctx.route.Backend)
			if nerr.Timeout() {
				p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(http.StatusGatewayTimeout))
				return nil, &proxyError{
					err:  err,
					code: http.StatusGatewayTimeout,
				}
			} else if !nerr.Temporary() {
				p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(http.StatusServiceUnavailable))
				return nil, &proxyError{
					err:  err,
					code: http.StatusServiceUnavailable,
				}
			} else if !nerr.Timeout() && nerr.Temporary() {
				p.log.Errorf("Backend error see https://github.com/zalando/skipper/issues/768: %v", err)
				p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(http.StatusServiceUnavailable))
				return nil, &proxyError{
					err:  err,
					code: http.StatusServiceUnavailable,
				}
			} else {
				p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(http.StatusInternalServerError))
				return nil, &proxyError{
					err:  err,
					code: http.StatusInternalServerError,
				}
			}
		}

		if cerr := req.Context().Err(); cerr != nil {
			// deadline exceeded or canceled in stdlib, proxy client closed request
			// see https://github.com/zalando/skipper/issues/687#issuecomment-405557503
			return nil, &proxyError{err: cerr, code: 499}
		}

		p.log.Errorf("Unexpected error from Go stdlib net/http package during roundtrip: %v", err)
		return nil, &proxyError{err: err}
	}
	p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(response.StatusCode))
	return response, nil
}

// checkRatelimit is used in case of a route ratelimit
// configuration. It returns the used ratelimit.Settings and 0 if
// the request passed in the context should be allowed.
// otherwise it returns the used ratelimit.Settings and the retry-after period.
func (p *Proxy) checkRatelimit(ctx *context) (ratelimit.Settings, int) {
	if p.limiters == nil {
		return ratelimit.Settings{}, 0
	}

	settings, ok := ctx.stateBag[ratelimitfilters.RouteSettingsKey].([]ratelimit.Settings)
	if !ok || len(settings) < 1 {
		return ratelimit.Settings{}, 0
	}

	for _, setting := range settings {
		rl := p.limiters.Get(setting)
		if rl == nil {
			p.log.Errorf("RateLimiter is nil for setting: %s", setting)
			continue
		}

		if setting.Lookuper == nil {
			p.log.Errorf("Lookuper is nil for setting: %s", setting)
			continue
		}

		s := setting.Lookuper.Lookup(ctx.Request())
		if s == "" {
			p.log.Errorf("Lookuper found no data in request for setting: %s and request: %v", setting, ctx.Request())
			continue
		}

		if !rl.Allow(s) {
			return setting, rl.RetryAfter(s)
		}
	}

	return ratelimit.Settings{}, 0
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

func newRatelimitError(settings ratelimit.Settings, retryAfter int) error {
	return &proxyError{
		err:  errRatelimit,
		code: http.StatusTooManyRequests,
		additionalHeader: http.Header{
			ratelimit.Header:           []string{strconv.Itoa(settings.MaxHits * int(time.Hour/settings.TimeWindow))},
			ratelimit.RetryAfterHeader: []string{strconv.Itoa(retryAfter)},
		},
	}
}

func (p *Proxy) do(ctx *context) error {
	if ctx.loopCounter > p.maxLoops {
		return errMaxLoopbacksReached
	}

	ctx.loopCounter++

	// proxy global setting
	if settings, retryAfter := p.limiters.Check(ctx.request); retryAfter > 0 {
		rerr := newRatelimitError(settings, retryAfter)
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

	if ctx.deprecatedShunted() {
		p.log.Debugf("deprecated shunting detected in route: %s", ctx.route.Id)
		return &proxyError{handled: true}
	} else if ctx.shunted() || ctx.route.Shunt || ctx.route.BackendType == eskip.ShuntBackend {
		ctx.ensureDefaultResponse()
	} else if ctx.route.BackendType == eskip.LoopBackend {
		loopCTX := ctx.clone()
		if err := p.do(loopCTX); err != nil {
			return err
		}

		ctx.setResponse(loopCTX.response, p.flags.PreserveOriginal())
		ctx.proxySpan = loopCTX.proxySpan
	} else if p.flags.Debug() {
		debugReq, err := mapRequest(ctx.request, ctx.route, ctx.outgoingHost, p.flags.HopHeadersRemoval(), ctx.StateBag())
		if err != nil {
			return &proxyError{err: err}
		}

		ctx.outgoingDebugRequest = debugReq
		ctx.setResponse(&http.Response{Header: make(http.Header)}, p.flags.PreserveOriginal())
	} else {
		// per route rate limit
		if settings, retryAfter := p.checkRatelimit(ctx); retryAfter > 0 {
			rerr := newRatelimitError(settings, retryAfter)
			return rerr
		}

		done, allow := p.checkBreaker(ctx)
		if !allow {
			tracing.LogKV("circuit_breaker", "open", ctx.request.Context())
			return errCircuitBreakerOpen
		}

		backendStart := time.Now()
		rsp, perr := p.makeBackendRequest(ctx)
		if perr != nil {
			if done != nil {
				done(false)
			}

			p.metrics.IncErrorsBackend(ctx.route.Id)

			if retryable(ctx.Request()) && perr.DialError() && ctx.route.BackendType == eskip.LBBackend {
				if ctx.proxySpan != nil {
					ctx.proxySpan.Finish()
					ctx.proxySpan = nil
				}

				tracing.LogKV("retry", ctx.route.Id, ctx.Request().Context())

				perr = nil
				var perr2 *proxyError
				rsp, perr2 = p.makeBackendRequest(ctx)
				if perr2 != nil {
					p.log.Errorf("Failed to do retry backend request: %v", perr2)
					if perr2.code >= http.StatusInternalServerError {
						p.metrics.MeasureBackend5xx(backendStart)
					}
					return perr2
				}
			} else {
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

func retryable(req *http.Request) bool {
	return req != nil && (req.Body == nil || req.Body == http.NoBody)
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
	p.tracing.logStreamEvent(ctx.proxySpan, StreamHeadersEvent, StartEvent)
	copyHeader(ctx.responseWriter.Header(), ctx.response.Header)
	p.tracing.logStreamEvent(ctx.proxySpan, StreamHeadersEvent, EndEvent)

	if err := ctx.Request().Context().Err(); err != nil {
		// deadline exceeded or canceled in stdlib, client closed request
		// see https://github.com/zalando/skipper/pull/864
		p.log.Infof("Client request: %v", err)
		ctx.response.StatusCode = 499
		p.tracing.setTag(ctx.proxySpan, ClientRequestStateTag, ClientRequestCanceled)
	}

	ctx.responseWriter.WriteHeader(ctx.response.StatusCode)
	ctx.responseWriter.Flush()
	err := copyStream(ctx.responseWriter, ctx.response.Body, p.tracing, ctx.proxySpan)
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
	backend := unknownRouteBackend
	if ctx.route != nil {
		id = ctx.route.Id
		backend = ctx.route.Backend
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
		p.tracing.setTag(span, HTTPStatusCodeTag, uint16(code))
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
	case ok && perr.err == errRatelimit:
		code = perr.code
	default:
		p.log.Errorf("error while proxying, route %s with backend %s, status code %d: %v", id, backend, code, err)
	}

	p.sendError(ctx, id, code)
}

func shouldLog(statusCode int, filter *al.AccessLogFilter) bool {
	if len(filter.Prefixes) == 0 {
		return filter.Enable
	}
	match := false
	for _, prefix := range filter.Prefixes {
		switch {
		case prefix < 10:
			match = (statusCode >= prefix*100 && statusCode < (prefix+1)*100)
		case prefix < 100:
			match = (statusCode >= prefix*10 && statusCode < (prefix+1)*10)
		default:
			match = statusCode == prefix
		}
		if match {
			break
		}
	}
	return match == filter.Enable
}

// http.Handler implementation
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lw := logging.NewLoggingWriter(w)

	p.metrics.IncCounter("incoming." + r.Proto)
	var ctx *context

	var span ot.Span
	wireContext, err := p.tracing.tracer.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(r.Header))
	if err == nil {
		span = p.tracing.tracer.StartSpan(p.tracing.initialOperationName, ext.RPCServerOption(wireContext))
	} else {
		span = p.tracing.tracer.StartSpan(p.tracing.initialOperationName)
		err = nil
	}
	defer func() {
		if ctx != nil && ctx.proxySpan != nil {
			ctx.proxySpan.Finish()
		}
		span.Finish()
	}()

	defer func() {
		accessLogEnabled, ok := ctx.stateBag[al.AccessLogEnabledKey].(*al.AccessLogFilter)

		if !ok {
			if p.accessLogDisabled {
				accessLogEnabled = &disabledAccessLog
			} else {
				accessLogEnabled = &enabledAccessLog
			}
		}
		statusCode := lw.GetCode()

		if shouldLog(statusCode, accessLogEnabled) {
			entry := &logging.AccessEntry{
				Request:      r,
				ResponseSize: lw.GetBytes(),
				StatusCode:   statusCode,
				RequestTime:  ctx.startServe,
				Duration:     time.Since(ctx.startServe),
			}

			logging.LogAccess(entry)
		}
	}()

	if p.flags.patchPath() {
		r.URL.Path = rfc.PatchPath(r.URL.Path, r.URL.RawPath)
	}

	p.tracing.setTag(span, SpanKindTag, SpanKindServer)
	p.setCommonSpanInfo(r.URL, r, span)
	r = r.WithContext(ot.ContextWithSpan(r.Context(), span))

	ctx = newContext(lw, r, p.flags.PreserveOriginal(), p.metrics, p.routing.Get())
	ctx.startServe = time.Now()
	ctx.tracer = p.tracing.tracer

	defer func() {
		if ctx.response != nil && ctx.response.Body != nil {
			err := ctx.response.Body.Close()
			if err != nil {
				p.log.Error("error during closing the response body", err)
			}
		}
	}()

	var lifoDone func()
	if p.lifo != nil {
		lifoDone, err = p.lifo.Wait()
		defer lifoDone()
	}

	switch {
	case err == jobqueue.ErrStackFull:
		err = errQueueFull
	case err == jobqueue.ErrTimeout:
		err = errQueueTimeout
	case err != nil:
		err = &proxyError{err: err, code: http.StatusInternalServerError}
	}

	if err == nil {
		err = p.do(ctx)
		pendingLIFO, _ := ctx.StateBag()[scheduler.LIFOKey].([]func())
		for _, done := range pendingLIFO {
			done()
		}
	}

	if err != nil {
		p.tracing.setTag(span, ErrorTag, true)
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

func (p *Proxy) setCommonSpanInfo(u *url.URL, r *http.Request, s ot.Span) {
	p.tracing.
		setTag(s, ComponentTag, "skipper").
		setTag(s, HTTPUrlTag, u.String()).
		setTag(s, HTTPMethodTag, r.Method).
		setTag(s, HostnameTag, hostname).
		setTag(s, HTTPRemoteAddrTag, r.RemoteAddr).
		setTag(s, HTTPPathTag, u.Path).
		setTag(s, HTTPHostTag, r.Host)
	if val := r.Header.Get("X-Flow-Id"); val != "" {
		p.tracing.setTag(s, FlowIDTag, val)
	}
}
