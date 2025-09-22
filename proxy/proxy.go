package proxy

import (
	"bytes"
	stdlibcontext "context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/exp/maps"
	"golang.org/x/time/rate"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	al "github.com/zalando/skipper/filters/accesslog"
	circuitfilters "github.com/zalando/skipper/filters/circuit"
	flowidFilter "github.com/zalando/skipper/filters/flowid"
	filterslog "github.com/zalando/skipper/filters/log"
	ratelimitfilters "github.com/zalando/skipper/filters/ratelimit"
	tracingfilter "github.com/zalando/skipper/filters/tracing"
	skpio "github.com/zalando/skipper/io"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/fastcgi"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/rfc"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

const (
	proxyBufferSize         = 8192
	unknownRouteID          = "_unknownroute_"
	unknownRouteBackendType = "<unknown>"
	unknownRouteBackend     = "<unknown>"

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

	// DisableFilterSpans disables creation of spans representing request and response filters.
	// Default: false
	DisableFilterSpans bool

	// LogFilterEvents enables the behavior to mark start and completion times of filters
	// on the span representing request/response filters being processed.
	// Default: false
	LogFilterEvents bool

	// LogStreamEvents enables the logs that marks the times when response headers & payload are streamed to
	// the client
	// Default: false
	LogStreamEvents bool

	// ExcludeTags controls what tags are disabled. Any tag that is listed here will be ignored.
	ExcludeTags []string
}

type PassiveHealthCheck struct {
	// The period of time after which the endpointregistry begins to calculate endpoints statistics
	// from scratch
	Period time.Duration

	// The minimum number of total requests that should be sent to an endpoint in a single period to
	// potentially opt out the endpoint from the list of healthy endpoints
	MinRequests int64

	// The minimal ratio of failed requests in a single period to potentially opt out the endpoint
	// from the list of healthy endpoints. This ratio is equal to the minimal non-zero probability of
	// dropping endpoint out from load balancing for every specific request
	MinDropProbability float64

	// The maximum probability of unhealthy endpoint to be dropped out from load balancing for every specific request
	MaxDropProbability float64

	// MaxUnhealthyEndpointsRatio is the maximum ratio of unhealthy endpoints in the list of all endpoints PHC will check
	// in case of all endpoints are unhealthy
	MaxUnhealthyEndpointsRatio float64
}

func InitPassiveHealthChecker(o map[string]string) (bool, *PassiveHealthCheck, error) {
	if len(o) == 0 {
		return false, &PassiveHealthCheck{}, nil
	}

	result := &PassiveHealthCheck{
		MinDropProbability:         0.0,
		MaxUnhealthyEndpointsRatio: 1.0,
	}
	requiredParams := map[string]struct{}{
		"period":               {},
		"max-drop-probability": {},
		"min-requests":         {},
	}

	for key, value := range o {
		delete(requiredParams, key)
		switch key {
		/* required parameters */
		case "period":
			period, err := time.ParseDuration(value)
			if err != nil {
				return false, nil, fmt.Errorf("passive health check: invalid period value: %s", value)
			}
			if period < 0 {
				return false, nil, fmt.Errorf("passive health check: invalid period value: %s", value)
			}
			result.Period = period
		case "min-requests":
			minRequests, err := strconv.Atoi(value)
			if err != nil {
				return false, nil, fmt.Errorf("passive health check: invalid minRequests value: %s", value)
			}
			if minRequests < 0 {
				return false, nil, fmt.Errorf("passive health check: invalid minRequests value: %s", value)
			}
			result.MinRequests = int64(minRequests)
		case "max-drop-probability":
			maxDropProbability, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return false, nil, fmt.Errorf("passive health check: invalid maxDropProbability value: %s", value)
			}
			if maxDropProbability < 0 || maxDropProbability > 1 {
				return false, nil, fmt.Errorf("passive health check: invalid maxDropProbability value: %s", value)
			}
			result.MaxDropProbability = maxDropProbability

		/* optional parameters */
		case "min-drop-probability":
			minDropProbability, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return false, nil, fmt.Errorf("passive health check: invalid minDropProbability value: %s", value)
			}
			if minDropProbability < 0 || minDropProbability > 1 {
				return false, nil, fmt.Errorf("passive health check: invalid minDropProbability value: %s", value)
			}
			result.MinDropProbability = minDropProbability
		case "max-unhealthy-endpoints-ratio":
			maxUnhealthyEndpointsRatio, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return false, nil, fmt.Errorf("passive health check: invalid maxUnhealthyEndpointsRatio value: %q", value)
			}
			if maxUnhealthyEndpointsRatio < 0 || maxUnhealthyEndpointsRatio > 1 {
				return false, nil, fmt.Errorf("passive health check: invalid maxUnhealthyEndpointsRatio value: %q", value)
			}
			result.MaxUnhealthyEndpointsRatio = maxUnhealthyEndpointsRatio
		default:
			return false, nil, fmt.Errorf("passive health check: invalid parameter: key=%s,value=%s", key, value)
		}
	}

	if len(requiredParams) != 0 {
		return false, nil, fmt.Errorf("passive health check: missing required parameters %+v", maps.Keys(requiredParams))
	}
	if result.MinDropProbability >= result.MaxDropProbability {
		return false, nil, fmt.Errorf("passive health check: minDropProbability should be less than maxDropProbability")
	}
	return true, result, nil
}

// Params are Proxy initialization options.
type Params struct {
	// The proxy expects a routing instance that is used to match
	// the incoming requests to routes.
	Routing *routing.Routing

	// Control flags. See the Flags values.
	Flags Flags

	// Metrics collector.
	// If not specified proxy uses global metrics.Default.
	Metrics metrics.Metrics

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

	// DisableHTTPKeepalives forces backend to always create a new connection
	DisableHTTPKeepalives bool

	// CircuitBreakers provides a registry that skipper can use to
	// find the matching circuit breaker for backend requests. If not
	// set, no circuit breakers are used.
	CircuitBreakers *circuit.Registry

	// RateLimiters provides a registry that skipper can use to
	// find the matching ratelimiter for backend requests. If not
	// set, no ratelimits are used.
	RateLimiters *ratelimit.Registry

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

	// CustomHttpRoundTripperWrap provides ability to wrap http.RoundTripper created by skipper.
	// http.RoundTripper is used for making outgoing requests (backends)
	// It allows to add additional logic (for example tracing) by providing a wrapper function
	// which accepts original skipper http.RoundTripper as an argument and returns a wrapped roundtripper
	CustomHttpRoundTripperWrap func(http.RoundTripper) http.RoundTripper

	// Registry provides key-value API which uses "host:port" string as a key
	// and returns some metadata about endpoint. Information about the metadata
	// returned from the registry could be found in routing.Metrics interface.
	EndpointRegistry *routing.EndpointRegistry

	// EnablePassiveHealthCheck enables the healthy endpoints checker
	EnablePassiveHealthCheck bool

	// PassiveHealthCheck defines the parameters for the healthy endpoints checker.
	PassiveHealthCheck *PassiveHealthCheck
}

type (
	ratelimitError   string
	routeLookupError string
)

func (e ratelimitError) Error() string   { return string(e) }
func (e routeLookupError) Error() string { return string(e) }

const (
	errRatelimit   = ratelimitError("ratelimited")
	errRouteLookup = routeLookupError("route lookup failed")
)

var (
	errRouteLookupFailed  = &proxyError{err: errRouteLookup}
	errCircuitBreakerOpen = &proxyError{
		err:              errors.New("circuit breaker open"),
		code:             http.StatusServiceUnavailable,
		additionalHeader: http.Header{"X-Circuit-Open": []string{"true"}},
	}

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

// PriorityRoute are custom route implementations that are matched against
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
	registry                 *routing.EndpointRegistry
	fadein                   *fadeIn
	heathlyEndpoints         *healthyEndpoints
	roundTripper             http.RoundTripper
	priorityRoutes           []PriorityRoute
	flags                    Flags
	metrics                  metrics.Metrics
	quit                     chan struct{}
	flushInterval            time.Duration
	breakers                 *circuit.Registry
	limiters                 *ratelimit.Registry
	log                      logging.Logger
	tracing                  *proxyTracing
	upgradeAuditLogOut       io.Writer
	upgradeAuditLogErr       io.Writer
	auditLogHook             chan struct{}
	clientTLS                *tls.Config
	hostname                 string
	onPanicSometimes         rate.Sometimes
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

type flusher struct {
	w flushedResponseWriter
}

func (f *flusher) Write(p []byte) (n int, err error) {
	n, err = f.w.Write(p)
	if err == nil {
		f.w.Flush()
	}
	return
}

func copyStream(to flushedResponseWriter, from io.Reader) (int64, error) {
	b := make([]byte, proxyBufferSize)

	return io.CopyBuffer(&flusher{to}, from, b)
}

func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func setRequestURLFromRequest(u *url.URL, r *http.Request) {
	if u.Host == "" {
		u.Host = r.Host
	}
	if u.Scheme == "" {
		u.Scheme = schemeFromRequest(r)
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

func (p *Proxy) selectEndpoint(ctx *context) *routing.LBEndpoint {
	rt := ctx.route
	endpoints := rt.LBEndpoints
	endpoints = p.fadein.filterFadeIn(endpoints, rt)
	endpoints = p.heathlyEndpoints.filterHealthyEndpoints(ctx, endpoints, p.metrics)

	lbctx := &routing.LBContext{
		Request:     ctx.request,
		Route:       rt,
		LBEndpoints: endpoints,
		Params:      ctx.StateBag(),
	}

	e := rt.LBAlgorithm.Apply(lbctx)

	return &e
}

// creates an outgoing http request to be forwarded to the route endpoint
// based on the augmented incoming request
func (p *Proxy) mapRequest(ctx *context, requestContext stdlibcontext.Context) (*http.Request, routing.Metrics, error) {
	var endpointMetrics routing.Metrics
	r := ctx.request
	rt := ctx.route
	host := ctx.outgoingHost
	stateBag := ctx.StateBag()
	u := r.URL

	switch rt.BackendType {
	case eskip.DynamicBackend:
		setRequestURLFromRequest(u, r)
		setRequestURLForDynamicBackend(u, stateBag)
	case eskip.LBBackend:
		endpoint := p.selectEndpoint(ctx)
		endpointMetrics = endpoint.Metrics
		u.Scheme = endpoint.Scheme
		u.Host = endpoint.Host
	case eskip.NetworkBackend:
		endpointMetrics = p.registry.GetMetrics(rt.Host)
		fallthrough
	default:
		u.Scheme = rt.Scheme
		u.Host = rt.Host
	}

	body := r.Body
	if r.ContentLength == 0 {
		body = nil
	}

	rr, err := http.NewRequestWithContext(requestContext, r.Method, u.String(), body)
	if err != nil {
		return nil, nil, err
	}

	rr.ContentLength = r.ContentLength
	if p.flags.HopHeadersRemoval() {
		rr.Header = cloneHeaderExcluding(r.Header, hopHeaders)
	} else {
		rr.Header = cloneHeader(r.Header)
	}
	// Disable default net/http user agent when user agent is not specified
	if _, ok := rr.Header["User-Agent"]; !ok {
		rr.Header["User-Agent"] = []string{""}
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

	if _, ok := stateBag[filters.BackendIsProxyKey]; ok {
		rr = forwardToProxy(r, rr)
	}

	return rr, endpointMetrics, nil
}

type proxyUrlContextKey struct{}

func forwardToProxy(incoming, outgoing *http.Request) *http.Request {
	proxyURL := &url.URL{
		Scheme: outgoing.URL.Scheme,
		Host:   outgoing.URL.Host,
	}

	outgoing.URL.Host = incoming.Host
	outgoing.URL.Scheme = schemeFromRequest(incoming)

	return outgoing.WithContext(stdlibcontext.WithValue(outgoing.Context(), proxyUrlContextKey{}, proxyURL))
}

func proxyFromContext(req *http.Request) (*url.URL, error) {
	proxyURL, _ := req.Context().Value(proxyUrlContextKey{}).(*url.URL)
	if proxyURL != nil {
		return proxyURL, nil
	}
	return nil, nil
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
			err:  fmt.Errorf("err from dial context: %w", cerr),
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

	if p.CustomHttpRoundTripperWrap == nil {
		// default wrapper which does nothing
		p.CustomHttpRoundTripperWrap = func(original http.RoundTripper) http.RoundTripper {
			return original
		}
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
		DisableKeepAlives:     p.DisableHTTPKeepalives,
		Proxy:                 proxyFromContext,
	}

	quit := make(chan struct{})
	// We need this to reliably fade on DNS change, which is right
	// now not fixed with IdleConnTimeout in the http.Transport.
	// https://github.com/golang/go/issues/23427
	if p.CloseIdleConnsPeriod > 0 {
		go func() {
			ticker := time.NewTicker(p.CloseIdleConnsPeriod)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					tr.CloseIdleConnections()
				case <-quit:
					return
				}
			}
		}()
	}

	if p.ClientTLS != nil {
		tr.TLSClientConfig = p.ClientTLS
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

	m := p.Metrics
	if m == nil {
		m = metrics.Default
	}

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

	if p.EndpointRegistry == nil {
		p.EndpointRegistry = routing.NewEndpointRegistry(routing.RegistryOptions{})
	}

	hostname := os.Getenv("HOSTNAME")

	var healthyEndpointsChooser *healthyEndpoints
	if p.EnablePassiveHealthCheck {
		healthyEndpointsChooser = &healthyEndpoints{
			rnd:                        rand.New(loadbalancer.NewLockedSource()),
			maxUnhealthyEndpointsRatio: p.PassiveHealthCheck.MaxUnhealthyEndpointsRatio,
		}
	}
	return &Proxy{
		routing:  p.Routing,
		registry: p.EndpointRegistry,
		fadein: &fadeIn{
			rnd: rand.New(loadbalancer.NewLockedSource()),
		},
		heathlyEndpoints:         healthyEndpointsChooser,
		roundTripper:             p.CustomHttpRoundTripperWrap(tr),
		priorityRoutes:           p.PriorityRoutes,
		flags:                    p.Flags,
		metrics:                  m,
		quit:                     quit,
		flushInterval:            p.FlushInterval,
		experimentalUpgrade:      p.ExperimentalUpgrade,
		experimentalUpgradeAudit: p.ExperimentalUpgradeAudit,
		maxLoops:                 p.MaxLoopbacks,
		breakers:                 p.CircuitBreakers,
		limiters:                 p.RateLimiters,
		log:                      &logging.DefaultLog{},
		defaultHTTPStatus:        defaultHTTPStatus,
		tracing:                  newProxyTracing(p.OpenTracing),
		accessLogDisabled:        p.AccessLogDisabled,
		upgradeAuditLogOut:       os.Stdout,
		upgradeAuditLogErr:       os.Stderr,
		clientTLS:                tr.TLSClientConfig,
		hostname:                 hostname,
		onPanicSometimes:         rate.Sometimes{First: 3, Interval: 1 * time.Minute},
	}
}

// applies filters to a request
func (p *Proxy) applyFiltersToRequest(f []*routing.RouteFilter, ctx *context) []*routing.RouteFilter {
	if len(f) == 0 {
		return f
	}

	filtersStart := time.Now()
	filterTracing := p.tracing.startFilterTracing("request_filters", ctx)
	defer filterTracing.finish()

	var filters = make([]*routing.RouteFilter, 0, len(f))
	for _, fi := range f {
		start := time.Now()
		filterTracing.logStart(fi.Name)
		ctx.setMetricsPrefix(fi.Name)

		fi.Request(ctx)

		p.metrics.MeasureFilterRequest(fi.Name, start)
		filterTracing.logEnd(fi.Name)

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
	filterTracing := p.tracing.startFilterTracing("response_filters", ctx)
	defer filterTracing.finish()

	for i := len(filters) - 1; i >= 0; i-- {
		fi := filters[i]
		start := time.Now()
		filterTracing.logStart(fi.Name)
		ctx.setMetricsPrefix(fi.Name)

		fi.Response(ctx)

		p.metrics.MeasureFilterResponse(fi.Name, start)
		filterTracing.logEnd(fi.Name)
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

func (p *Proxy) makeUpgradeRequest(ctx *context, req *http.Request) {
	backendURL := req.URL

	reverseProxy := httputil.NewSingleHostReverseProxy(backendURL)
	reverseProxy.FlushInterval = p.flushInterval
	upgradeProxy := upgradeProxy{
		backendAddr:     backendURL,
		reverseProxy:    reverseProxy,
		insecure:        p.flags.Insecure(),
		tlsClientConfig: p.clientTLS,
		useAuditLog:     p.experimentalUpgradeAudit,
		auditLogOut:     p.upgradeAuditLogOut,
		auditLogErr:     p.upgradeAuditLogErr,
		auditLogHook:    p.auditLogHook,
	}

	upgradeProxy.serveHTTP(ctx.responseWriter, req)
	ctx.successfulUpgrade = true
	ctx.Logger().Debugf("finished upgraded protocol %s session", getUpgradeRequest(ctx.request))
}

func (p *Proxy) makeBackendRequest(ctx *context, requestContext stdlibcontext.Context) (*http.Response, *proxyError) {
	requestStopWatch, responseStopWatch := newStopWatch(), newStopWatch()
	requestStopWatch.Start()

	defer func() {
		requestStopWatch.Stop()
		responseStopWatch.Stop()
		ctx.proxyRequestElapsed = requestStopWatch.Elapsed()
		ctx.proxyResponseElapsed = responseStopWatch.Elapsed()
	}()

	payloadProtocol := getUpgradeRequest(ctx.Request())

	req, endpointMetrics, err := p.mapRequest(ctx, requestContext)
	if err != nil {
		return nil, &proxyError{err: fmt.Errorf("could not map backend request: %w", err)}
	}

	if res, ok := p.rejectBackend(ctx, req); ok {
		return res, nil
	}

	if endpointMetrics != nil {
		endpointMetrics.IncInflightRequest()
		defer endpointMetrics.DecInflightRequest()
	}

	if p.experimentalUpgrade && payloadProtocol != "" {
		// see also https://github.com/golang/go/blob/9159cd4ec6b0e9475dc9c71c830035c1c4c13483/src/net/http/httputil/reverseproxy.go#L423-L428
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", payloadProtocol)
		p.makeUpgradeRequest(ctx, req)

		// We are not owner of the connection anymore.
		return nil, &proxyError{handled: true}
	}

	roundTripper, err := p.getRoundTripper(ctx, req)
	if err != nil {
		return nil, &proxyError{err: fmt.Errorf("failed to get roundtripper: %w", err), code: http.StatusBadGateway}
	}

	bag := ctx.StateBag()
	spanName, ok := bag[tracingfilter.OpenTracingProxySpanKey].(string)
	if !ok {
		spanName = "proxy"
	}

	proxySpanOpts := []ot.StartSpanOption{ot.Tags{
		SpanKindTag: SpanKindClient,
	}}
	if parentSpan := ot.SpanFromContext(req.Context()); parentSpan != nil {
		proxySpanOpts = append(proxySpanOpts, ot.ChildOf(parentSpan.Context()))
	}
	ctx.proxySpan = p.tracing.tracer.StartSpan(spanName, proxySpanOpts...)

	u := cloneURL(req.URL)
	u.RawQuery = ""
	p.tracing.
		setTag(ctx.proxySpan, HTTPUrlTag, u.String()).
		setTag(ctx.proxySpan, SkipperRouteIDTag, ctx.route.Id).
		setTag(ctx.proxySpan, NetworkPeerAddressTag, u.Host)
	p.setCommonSpanInfo(u, req, ctx.proxySpan)

	carrier := ot.HTTPHeadersCarrier(req.Header)
	_ = p.tracing.tracer.Inject(ctx.proxySpan.Context(), ot.HTTPHeaders, carrier)

	req = req.WithContext(ot.ContextWithSpan(req.Context(), ctx.proxySpan))

	p.metrics.IncCounter("outgoing." + req.Proto)
	ctx.proxySpan.LogKV("http_roundtrip", StartEvent)
	req = injectClientTrace(req, ctx.proxySpan)

	p.metrics.MeasureBackendRequestHeader(ctx.metricsHost(), snet.SizeOfRequestHeader(req))

	requestStopWatch.Stop()

	response, err := roundTripper.RoundTrip(req)

	responseStopWatch.Start()

	if endpointMetrics != nil {
		endpointMetrics.IncRequests(routing.IncRequestsOptions{FailedRoundTrip: err != nil})
	}
	ctx.proxySpan.LogKV("http_roundtrip", EndEvent)
	if err != nil {
		if errors.Is(err, skpio.ErrBlocked) {
			p.tracing.setTag(ctx.proxySpan, BlockTag, true)
			p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(http.StatusBadRequest))
			return nil, &proxyError{err: err, code: http.StatusBadRequest}
		}
		p.tracing.setTag(ctx.proxySpan, ErrorTag, true)

		// Check if the request has been cancelled or timed out
		// The roundtrip error `err` may be different:
		// - for `Canceled` it could be either the same `context canceled` or `unexpected EOF` (net.OpError)
		// - for `DeadlineExceeded` it is net.Error(timeout=true, temporary=true) wrapping this `context deadline exceeded`
		if cerr := req.Context().Err(); cerr != nil {
			ctx.proxySpan.LogKV("event", "error", "message", ensureUTF8(cerr.Error()))
			if cerr == stdlibcontext.Canceled {
				return nil, &proxyError{err: cerr, code: 499}
			} else if cerr == stdlibcontext.DeadlineExceeded {
				return nil, &proxyError{err: cerr, code: http.StatusGatewayTimeout}
			}
		}

		errMessage := err.Error()
		ctx.proxySpan.LogKV("event", "error", "message", ensureUTF8(errMessage))

		if perr, ok := err.(*proxyError); ok {
			perr.err = fmt.Errorf("failed to do backend roundtrip to %s: %w", req.URL.Host, perr.err)
			return nil, perr
		} else if nerr, ok := err.(net.Error); ok {
			var status int
			if nerr.Timeout() {
				status = http.StatusGatewayTimeout
			} else {
				status = http.StatusServiceUnavailable
			}
			p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(status))
			//lint:ignore SA1019 Temporary is deprecated in Go 1.18, but keep it for now (https://github.com/zalando/skipper/issues/1992)
			return nil, &proxyError{err: fmt.Errorf("net.Error during backend roundtrip to %s: timeout=%v temporary='%v': %w", req.URL.Host, nerr.Timeout(), nerr.Temporary(), err), code: status}
		}

		switch errMessage {
		case // net/http/internal/chunked.go
			"header line too long",
			"chunked encoding contains too much non-data",
			"malformed chunked encoding",
			"empty hex number for chunk length",
			"invalid byte in chunk length",
			"http chunk length too large":
			return nil, &proxyError{code: http.StatusBadRequest, err: fmt.Errorf("failed to do backend roundtrip due to invalid request: %w", err)}
		}

		return nil, &proxyError{err: fmt.Errorf("unexpected error from Go stdlib net/http package during roundtrip: %w", err)}
	}
	p.tracing.setTag(ctx.proxySpan, HTTPStatusCodeTag, uint16(response.StatusCode))
	if response.Uncompressed {
		p.metrics.IncCounter("experimental.uncompressed")
	}
	return response, nil
}

func (p *Proxy) getRoundTripper(ctx *context, req *http.Request) (http.RoundTripper, error) {
	switch req.URL.Scheme {
	case "fastcgi":
		f := "index.php"
		if sf, ok := ctx.StateBag()["fastCgiFilename"]; ok {
			f = sf.(string)
		} else if len(req.URL.Path) > 1 && req.URL.Path != "/" {
			f = req.URL.Path[1:]
		}
		rt, err := fastcgi.NewRoundTripper(p.log, req.URL.Host, f)
		if err != nil {
			return nil, err
		}

		// FastCgi expects the Host to be in form host:port
		// It will then be split and added as 2 separate params to the backend process
		if _, _, err := net.SplitHostPort(req.Host); err != nil {
			req.Host = req.Host + ":" + req.URL.Port()
		}

		// RemoteAddr is needed to pass to the backend process as param
		req.RemoteAddr = ctx.request.RemoteAddr

		return rt, nil
	default:
		return p.roundTripper, nil
	}
}

func (p *Proxy) rejectBackend(ctx *context, req *http.Request) (*http.Response, bool) {
	limit, ok := ctx.StateBag()[filters.BackendRatelimit].(*ratelimitfilters.BackendRatelimit)
	if ok {
		s := req.URL.Scheme + "://" + req.URL.Host

		if !p.limiters.Get(limit.Settings).Allow(req.Context(), s) {
			return &http.Response{
				StatusCode: limit.StatusCode,
				Header:     http.Header{"Content-Length": []string{"0"}},
				Body:       io.NopCloser(&bytes.Buffer{}),
			}, true
		}
	}
	return nil, false
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
	if !ok && c.request.Body != nil {
		// consume the body to prevent goroutine leaks
		io.Copy(io.Discard, c.request.Body)
	}
	return done, ok
}

func newRatelimitError(settings ratelimit.Settings, retryAfter int) *proxyError {
	return &proxyError{
		err:              errRatelimit,
		code:             http.StatusTooManyRequests,
		additionalHeader: ratelimit.Headers(settings.MaxHits, settings.TimeWindow, retryAfter),
	}
}

// copied from debug.PrintStack
func stack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

func (p *Proxy) do(ctx *context, parentSpan ot.Span) (err error) {
	var requestElapsed, responseElapsed time.Duration

	requestStopWatch, responseStopWatch := newStopWatch(), newStopWatch()
	requestStopWatch.Start()

	defer func() {
		if r := recover(); r != nil {
			p.onPanicSometimes.Do(func() {
				ctx.Logger().Errorf("stacktrace of panic caused by: %v:\n%s", r, stack())
			})

			perr := &proxyError{
				err: fmt.Errorf("panic caused by: %v", r),
			}
			p.makeErrorResponse(ctx, perr)
			err = perr
		}
		requestStopWatch.Stop()
		responseStopWatch.Stop()
		ctx.proxyRequestElapsed = requestElapsed + requestStopWatch.Elapsed()
		ctx.proxyResponseElapsed = responseElapsed + responseStopWatch.Elapsed()
	}()

	if ctx.executionCounter > p.maxLoops {
		// TODO(sszuecs): think about setting status code to 463 or 465 (check what AWS ALB sets for redirect loop) or similar
		perr := &proxyError{
			err: fmt.Errorf("max loopbacks reached after route %s", ctx.route.Id),
		}
		p.makeErrorResponse(ctx, perr)
		return perr
	}

	// proxy global setting
	if !ctx.wasExecuted() {
		if settings, retryAfter := p.limiters.Check(ctx.request); retryAfter > 0 {
			perr := newRatelimitError(settings, retryAfter)
			p.makeErrorResponse(ctx, perr)
			return perr
		}
	}
	// every time the context is used for a request the context executionCounter is incremented
	// a context executionCounter equal to zero represents a root context.
	ctx.executionCounter++
	lookupStart := time.Now()
	route, params := p.lookupRoute(ctx)
	p.metrics.MeasureRouteLookup(lookupStart)
	if route == nil {
		p.metrics.IncRoutingFailures()
		ctx.Logger().Debugf("could not find a route for %v", ctx.request.URL)
		p.makeErrorResponse(ctx, errRouteLookupFailed)
		return errRouteLookupFailed
	}
	parentSpan.SetTag(SkipperRouteIDTag, route.Id)

	ctx.applyRoute(route, params, p.flags.PreserveHost())

	requestStopWatch.Stop()
	processedFilters := p.applyFiltersToRequest(ctx.route.Filters, ctx)
	requestStopWatch.Start()

	// not every of these branches could endup in a response to the client
	if ctx.deprecatedShunted() {
		ctx.Logger().Debugf("deprecated shunting detected in route: %s", ctx.route.Id)
		return &proxyError{handled: true}
	} else if ctx.shunted() || ctx.route.Shunt || ctx.route.BackendType == eskip.ShuntBackend {
		requestStopWatch.Stop()
		// consume the body to prevent goroutine leaks
		if ctx.request.Body != nil {
			if _, err := io.Copy(io.Discard, ctx.request.Body); err != nil {
				ctx.Logger().Debugf("error while discarding remainder request body: %v.", err)
			}
		}

		responseStopWatch.Start()
		ctx.ensureDefaultResponse()
	} else if ctx.route.BackendType == eskip.LoopBackend {

		loopCTX := ctx.clone()

		loopSpanOpts := []ot.StartSpanOption{ot.Tags{
			SpanKindTag: SpanKindServer,
		}}
		if parentSpan := ot.SpanFromContext(ctx.request.Context()); parentSpan != nil {
			loopSpanOpts = append(loopSpanOpts, ot.ChildOf(parentSpan.Context()))
		}

		// defer can't be used to finish the loopSpan, because it will be called only at the end of the outer function
		// leading to distortion of the tracing picture, because response filters processing time will be included into the loopSpan timings
		loopSpan := p.tracing.tracer.StartSpan("loopback", loopSpanOpts...)

		p.tracing.setTag(loopSpan, SkipperRouteIDTag, ctx.route.Id)
		p.setCommonSpanInfo(ctx.Request().URL, ctx.Request(), loopSpan)
		ctx.parentSpan = loopSpan

		r := loopCTX.Request()
		r = r.WithContext(ot.ContextWithSpan(r.Context(), loopSpan))
		loopCTX.request = r

		requestStopWatch.Stop()

		err := p.do(loopCTX, loopSpan)

		loopSpan.Finish()
		responseStopWatch.Start()

		if err != nil {
			// in case of error we have to copy the response in this recursion unwinding
			ctx.response = loopCTX.response
			p.applyFiltersOnError(ctx, processedFilters)
			return err
		}

		requestElapsed += loopCTX.proxyRequestElapsed
		responseElapsed += loopCTX.proxyResponseElapsed

		ctx.setResponse(loopCTX.response, p.flags.PreserveOriginal())
		ctx.proxySpan = loopCTX.proxySpan
	} else if p.flags.Debug() {
		requestStopWatch.Stop()
		debugReq, _, err := p.mapRequest(ctx, ctx.request.Context())
		responseStopWatch.Start()
		if err != nil {
			perr := &proxyError{err: err}
			p.makeErrorResponse(ctx, perr)
			p.applyFiltersOnError(ctx, processedFilters)
			return perr
		}

		ctx.outgoingDebugRequest = debugReq
		ctx.setResponse(&http.Response{Header: make(http.Header)}, p.flags.PreserveOriginal())
	} else {

		done, allow := p.checkBreaker(ctx)
		if !allow {
			tracing.LogKV("circuit_breaker", "open", ctx.request.Context())
			p.makeErrorResponse(ctx, errCircuitBreakerOpen)
			p.applyFiltersOnError(ctx, processedFilters)
			return errCircuitBreakerOpen
		}

		backendContext := ctx.request.Context()
		if timeout, ok := ctx.StateBag()[filters.BackendTimeout]; ok {
			backendContext, ctx.cancelBackendContext = stdlibcontext.WithTimeout(backendContext, timeout.(time.Duration))
		}

		backendStart := time.Now()
		if d, ok := ctx.StateBag()[filters.ReadTimeout].(time.Duration); ok {
			e := ctx.ResponseController().SetReadDeadline(backendStart.Add(d))
			if e != nil {
				ctx.Logger().Errorf("Failed to set read deadline: %v", e)
			}
		}

		requestStopWatch.Stop()
		rsp, perr := p.makeBackendRequest(ctx, backendContext)
		requestElapsed += ctx.proxyRequestElapsed
		responseElapsed += ctx.proxyResponseElapsed
		if perr != nil {
			requestStopWatch.Start()
			if done != nil {
				done(false)
			}

			p.metrics.IncErrorsBackend(ctx.route.Id)

			if retryable(ctx, perr) {
				if ctx.proxySpan != nil {
					ctx.proxySpan.Finish()
					ctx.proxySpan = nil
				}

				tracing.LogKV("retry", ctx.route.Id, ctx.Request().Context())

				perr = nil
				var perr2 *proxyError
				requestStopWatch.Stop()
				rsp, perr2 = p.makeBackendRequest(ctx, backendContext)
				requestElapsed += ctx.proxyRequestElapsed
				responseElapsed += ctx.proxyResponseElapsed
				responseStopWatch.Start()
				if perr2 != nil {
					ctx.Logger().Errorf("Failed to retry backend request: %v", perr2)
					if perr2.code >= http.StatusInternalServerError {
						p.metrics.MeasureBackend5xx(backendStart)
					}

					p.makeErrorResponse(ctx, perr2)
					p.applyFiltersOnError(ctx, processedFilters)
					return perr2
				}
			} else {
				requestStopWatch.Stop()
				responseStopWatch.Start()
				p.makeErrorResponse(ctx, perr)
				p.applyFiltersOnError(ctx, processedFilters)
				return perr
			}
		} else {
			responseStopWatch.Start()
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
	responseStopWatch.Stop()
	p.applyFiltersToResponse(processedFilters, ctx)
	responseStopWatch.Start()
	return nil
}

func retryable(ctx *context, perr *proxyError) bool {
	req := ctx.Request()
	return perr.code != 499 && perr.DialError() &&
		ctx.route.BackendType == eskip.LBBackend &&
		req != nil && (req.Body == nil || req.Body == http.NoBody)
}

func (p *Proxy) serveResponse(ctx *context) {

	responseStopWatch := newStopWatch()
	responseStopWatch.Start()
	defer func() {
		responseStopWatch.Stop()
		ctx.proxyResponseElapsed = responseStopWatch.Elapsed()
	}()

	if p.flags.Debug() {
		dbgResponse(ctx.responseWriter, &debugInfo{
			route:    &ctx.route.Route,
			incoming: ctx.originalRequest,
			outgoing: ctx.outgoingDebugRequest,
			response: ctx.response,
		})

		return
	}

	start := time.Now()
	p.tracing.logStreamEvent(ctx.proxySpan, StreamHeadersEvent, StartEvent)
	copyHeader(ctx.responseWriter.Header(), ctx.response.Header)

	if err := ctx.Request().Context().Err(); err != nil {
		// deadline exceeded or canceled in stdlib, client closed request
		// see https://github.com/zalando/skipper/pull/864
		ctx.Logger().Debugf("Client request: %v", err)
		ctx.response.StatusCode = 499
		p.tracing.setTag(ctx.proxySpan, ClientRequestStateTag, ClientRequestCanceled)
	}

	p.tracing.setTag(ctx.initialSpan, HTTPStatusCodeTag, uint16(ctx.response.StatusCode))

	ctx.responseWriter.WriteHeader(ctx.response.StatusCode)
	ctx.responseWriter.Flush()
	p.tracing.logStreamEvent(ctx.proxySpan, StreamHeadersEvent, EndEvent)

	responseStopWatch.Stop()

	n, err := copyStream(ctx.responseWriter, ctx.response.Body)

	responseStopWatch.Start()

	p.tracing.logStreamEvent(ctx.proxySpan, StreamBodyEvent, strconv.FormatInt(n, 10))
	if err != nil {
		p.metrics.IncErrorsStreaming(ctx.route.Id)
		ctx.Logger().Debugf("error while copying the response stream: %v", err)
		p.tracing.setTag(ctx.proxySpan, ErrorTag, true)
		p.tracing.setTag(ctx.proxySpan, StreamBodyEvent, StreamBodyError)
		p.tracing.logStreamEvent(ctx.proxySpan, StreamBodyEvent, fmt.Sprintf("Failed to stream response: %v", err))
	} else {
		p.metrics.MeasureResponse(ctx.response.StatusCode, ctx.request.Method, ctx.route.Id, start)
		p.metrics.MeasureResponseSize(ctx.metricsHost(), n)
	}
	p.metrics.MeasureServe(ctx.route.Id, ctx.metricsHost(), ctx.request.Method, ctx.response.StatusCode, ctx.startServe)
}

func (p *Proxy) errorResponse(ctx *context, err error) {
	responseStopWatch := newStopWatch()
	responseStopWatch.Start()
	defer func() {
		responseStopWatch.Stop()
		ctx.proxyResponseElapsed = responseStopWatch.Elapsed()
	}()

	perr, ok := err.(*proxyError)
	if ok && perr.handled {
		return
	}

	flowIdLog := ""
	flowId := ctx.Request().Header.Get(flowidFilter.HeaderName)
	if flowId != "" {
		flowIdLog = fmt.Sprintf(", flow id %s", flowId)
	}
	id := unknownRouteID
	backendType := unknownRouteBackendType
	backend := unknownRouteBackend
	if ctx.route != nil {
		id = ctx.route.Id
		backendType = ctx.route.BackendType.String()
		backend = fmt.Sprintf("%s://%s", ctx.request.URL.Scheme, ctx.request.URL.Host)
	}

	if err == errRouteLookupFailed {
		ctx.initialSpan.LogKV("event", "error", "message", errRouteLookup.Error())
	}

	p.tracing.setTag(ctx.initialSpan, ErrorTag, true)
	p.tracing.setTag(ctx.initialSpan, HTTPStatusCodeTag, ctx.response.StatusCode)

	if p.flags.Debug() {
		di := &debugInfo{
			incoming: ctx.originalRequest,
			outgoing: ctx.outgoingDebugRequest,
			response: ctx.response,
			err:      err,
		}

		if ctx.route != nil {
			di.route = &ctx.route.Route
		}

		dbgResponse(ctx.responseWriter, di)
		return
	}

	msgPrefix := "error while proxying"
	logFunc := ctx.Logger().Errorf
	if ctx.response.StatusCode == 499 {
		msgPrefix = "client canceled"
		logFunc = ctx.Logger().Infof
		if p.accessLogDisabled {
			logFunc = ctx.Logger().Debugf
		}
	}
	if id != unknownRouteID {
		req := ctx.Request()
		remoteAddr := remoteHost(req)
		uri := req.RequestURI
		if i := strings.IndexRune(uri, '?'); i >= 0 {
			uri = uri[:i]
		}
		logFunc(
			`%s after %v, route %s with backend %s %s%s, status code %d: %v, remote host: %s, request: "%s %s %s", host: %s, user agent: "%s"`,
			msgPrefix,
			time.Since(ctx.startServe),
			id,
			backendType,
			backend,
			flowIdLog,
			ctx.response.StatusCode,
			err,
			remoteAddr,
			req.Method,
			uri,
			req.Proto,
			req.Host,
			req.UserAgent(),
		)
	}

	copyHeader(ctx.responseWriter.Header(), ctx.response.Header)
	ctx.responseWriter.WriteHeader(ctx.response.StatusCode)
	ctx.responseWriter.Flush()

	responseStopWatch.Stop()
	_, _ = copyStream(ctx.responseWriter, ctx.response.Body)
	responseStopWatch.Start()

	p.metrics.MeasureServe(
		id,
		ctx.metricsHost(),
		ctx.request.Method,
		ctx.response.StatusCode,
		ctx.startServe,
	)
}

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}

	return address
}

// The remote address of the client. When the 'X-Forwarded-For'
// header is set, then it is used instead.
func remoteAddr(r *http.Request) string {
	ff := r.Header.Get("X-Forwarded-For")
	if ff != "" {
		return ff
	}

	return r.RemoteAddr
}

func remoteHost(r *http.Request) string {
	a := remoteAddr(r)
	return stripPort(a)
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

	var requestElapsed, responseElapsed time.Duration
	requestStopWatch, responseStopWatch := newStopWatch(), newStopWatch()
	requestStopWatch.Start()

	lw := logging.NewLoggingWriter(w)

	p.metrics.IncCounter("incoming." + r.Proto)
	var ctx *context

	spanOpts := []ot.StartSpanOption{ot.Tags{
		SpanKindTag: SpanKindServer,
	}}
	if wireContext, err := p.tracing.tracer.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(r.Header)); err == nil {
		spanOpts = append(spanOpts, ext.RPCServerOption(wireContext))
	}
	span := p.tracing.tracer.StartSpan(p.tracing.initialOperationName, spanOpts...)

	defer func() {
		if ctx != nil && ctx.proxySpan != nil {
			ctx.proxySpan.Finish()
		}
		span.Finish()
		requestStopWatch.Stop()
		responseStopWatch.Stop()

		p.metrics.MeasureProxy(requestElapsed+requestStopWatch.Elapsed(), responseElapsed+responseStopWatch.Elapsed())
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
			authUser, _ := ctx.stateBag[filterslog.AuthUserKey].(string)
			entry := &logging.AccessEntry{
				Request:      r,
				ResponseSize: lw.GetBytes(),
				StatusCode:   statusCode,
				RequestTime:  ctx.startServe,
				Duration:     time.Since(ctx.startServe),
				AuthUser:     authUser,
			}

			additionalData, _ := ctx.stateBag[al.AccessLogAdditionalDataKey].(map[string]interface{})

			logging.LogAccess(entry, additionalData)
		}

		// This flush is required in I/O error
		if !ctx.successfulUpgrade {
			lw.Flush()
		}
	}()

	if p.flags.patchPath() {
		r.URL.Path = rfc.PatchPath(r.URL.Path, r.URL.RawPath)
	}

	p.tracing.setTag(span, HTTPRemoteIPTag, stripPort(r.RemoteAddr))
	p.setCommonSpanInfo(r.URL, r, span)
	r = r.WithContext(ot.ContextWithSpan(r.Context(), span))
	r = r.WithContext(routing.NewContext(r.Context()))

	rCtx := r.Context()
	defer pprof.SetGoroutineLabels(rCtx)

	tCtx := pprof.WithLabels(rCtx, pprof.Labels("trace_id", tracing.GetTraceID(span)))
	pprof.SetGoroutineLabels(tCtx)
	r = r.WithContext(tCtx)

	ctx = newContext(lw, r, p)
	ctx.startServe = time.Now()
	ctx.tracer = p.tracing.tracer
	ctx.initialSpan = span
	ctx.parentSpan = span

	defer func() {
		if ctx.response != nil && ctx.response.Body != nil {
			err := ctx.response.Body.Close()
			if err != nil {
				ctx.Logger().Errorf("error during closing the response body: %v", err)
			}
		}
	}()

	requestStopWatch.Stop()

	err := p.do(ctx, span)

	requestElapsed += ctx.proxyRequestElapsed
	responseElapsed += ctx.proxyResponseElapsed

	responseStopWatch.Start()

	// writeTimeout() filter
	if d, ok := ctx.StateBag()[filters.WriteTimeout].(time.Duration); ok {
		e := ctx.ResponseController().SetWriteDeadline(time.Now().Add(d))
		if e != nil {
			ctx.Logger().Errorf("Failed to set write deadline: %v", e)
		}
	}

	responseStopWatch.Stop()
	// stream response body to client
	if err != nil {
		p.errorResponse(ctx, err)
	} else {
		p.serveResponse(ctx)
	}
	responseElapsed += ctx.proxyResponseElapsed
	responseStopWatch.Start()
	// fifoWithBody() filter
	if sbf, ok := ctx.StateBag()[filters.FifoWithBodyName]; ok {
		if fs, ok := sbf.([]func()); ok {
			for i := len(fs) - 1; i >= 0; i-- {
				fs[i]()
			}
		}
	}

	if ctx.cancelBackendContext != nil {
		ctx.cancelBackendContext()
	}
}

// Close causes the proxy to stop closing idle
// connections and, currently, has no other effect.
// It's primary purpose is to support testing.
func (p *Proxy) Close() error {
	close(p.quit)
	p.registry.Close()
	return nil
}

func (p *Proxy) setCommonSpanInfo(u *url.URL, r *http.Request, s ot.Span) {
	p.tracing.
		setTag(s, ComponentTag, "skipper").
		setTag(s, HTTPMethodTag, r.Method).
		setTag(s, HostnameTag, p.hostname).
		setTag(s, HTTPPathTag, u.Path).
		setTag(s, HTTPHostTag, r.Host)
	if val := r.Header.Get("X-Flow-Id"); val != "" {
		p.tracing.setTag(s, FlowIDTag, val)
	}
}

// TODO(sszuecs): copy from net.Client, we should refactor this to use net.Client
func injectClientTrace(req *http.Request, span ot.Span) *http.Request {
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			span.LogKV("DNS", "start")
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			span.LogKV("DNS", "end")
		},
		ConnectStart: func(string, string) {
			span.LogKV("connect", "start")
		},
		ConnectDone: func(string, string, error) {
			span.LogKV("connect", "end")
		},
		TLSHandshakeStart: func() {
			span.LogKV("TLS", "start")
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			span.LogKV("TLS", "end")
		},
		GetConn: func(string) {
			span.LogKV("get_conn", "start")
		},
		GotConn: func(httptrace.GotConnInfo) {
			span.LogKV("get_conn", "end")
		},
		WroteHeaders: func() {
			span.LogKV("wrote_headers", "done")
		},
		WroteRequest: func(wri httptrace.WroteRequestInfo) {
			if wri.Err != nil {
				span.LogKV("wrote_request", ensureUTF8(wri.Err.Error()))
			} else {
				span.LogKV("wrote_request", "done")
			}
		},
		GotFirstResponseByte: func() {
			span.LogKV("got_first_byte", "done")
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

func ensureUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return fmt.Sprintf("invalid utf-8: %q", s)
}

func (p *Proxy) makeErrorResponse(ctx *context, perr *proxyError) {
	ctx.response = &http.Response{
		Header: http.Header{},
	}

	if len(perr.additionalHeader) > 0 {
		copyHeader(ctx.response.Header, perr.additionalHeader)
	}
	addBranding(ctx.response.Header)
	ctx.response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	ctx.response.Header.Set("X-Content-Type-Options", "nosniff")

	code := http.StatusInternalServerError
	switch {
	case perr == errRouteLookupFailed:
		code = p.defaultHTTPStatus
	case perr.code == -1:
		// -1 == dial connection refused
		code = http.StatusBadGateway
	case perr.code != 0:
		code = perr.code
	}

	text := http.StatusText(code) + "\n"
	ctx.response.Header.Set("Content-Length", strconv.Itoa(len(text)))
	ctx.response.StatusCode = code

	ctx.response.Body = io.NopCloser(bytes.NewBufferString(text))
}

// errorHandlerFilter is an opt-in for filters to get called
// Response(ctx) in case of errors.
type errorHandlerFilter interface {
	// HandleErrorResponse returns true in case a filter wants to get called
	HandleErrorResponse() bool
}

func (p *Proxy) applyFiltersOnError(ctx *context, filters []*routing.RouteFilter) {
	filtersStart := time.Now()
	filterTracing := p.tracing.startFilterTracing("response_filters", ctx)
	defer filterTracing.finish()

	for i := len(filters) - 1; i >= 0; i-- {
		fi := filters[i]

		if ehf, ok := fi.Filter.(errorHandlerFilter); !ok || !ehf.HandleErrorResponse() {
			continue
		}
		ctx.Logger().Debugf("filter %s handles error", fi.Name)

		start := time.Now()
		filterTracing.logStart(fi.Name)
		ctx.setMetricsPrefix(fi.Name)

		fi.Response(ctx)

		p.metrics.MeasureFilterResponse(fi.Name, start)
		filterTracing.logEnd(fi.Name)
	}

	p.metrics.MeasureAllFiltersResponse(ctx.route.Id, filtersStart)
}
