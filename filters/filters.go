package filters

import (
	"errors"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/opentracing/opentracing-go"
)

const (
	// DynamicBackendHostKey is the key used in the state bag to pass host to the proxy.
	DynamicBackendHostKey = "backend:dynamic:host"

	// DynamicBackendSchemeKey is the key used in the state bag to pass scheme to the proxy.
	DynamicBackendSchemeKey = "backend:dynamic:scheme"

	// DynamicBackendURLKey is the key used in the state bag to pass url to the proxy.
	DynamicBackendURLKey = "backend:dynamic:url"

	// BackendIsProxyKey is the key used in the state bag to notify proxy that the backend is also a proxy.
	BackendIsProxyKey = "backend:isproxy"

	// BackendTimeout is the key used in the state bag to configure backend timeout in proxy
	BackendTimeout = "backend:timeout"

	// ReadTimeout is the key used in the state bag to configure read request body timeout in proxy
	ReadTimeout = "read:timeout"

	// WriteTimeout is the key used in the state bag to configure write response body timeout in proxy
	WriteTimeout = "write:timeout"

	// BackendRatelimit is the key used in the state bag to configure backend ratelimit in proxy
	BackendRatelimit = "backend:ratelimit"
)

// FilterContext object providing state and information that is unique to a request.
type FilterContext interface {
	// The response writer object belonging to the incoming request. Used by
	// filters that handle the requests themselves.
	// Deprecated: use Response() or Serve()
	ResponseWriter() http.ResponseWriter

	// The incoming request object. It is forwarded to the route endpoint
	// with its properties changed by the filters.
	Request() *http.Request

	// The response object. It is returned to the client with its
	// properties changed by the filters.
	Response() *http.Response

	// The copy (deep) of the original incoming request or nil if the
	// implementation does not provide it.
	//
	// The object received from this method contains an empty body, and all
	// payload related properties have zero value.
	OriginalRequest() *http.Request

	// The copy (deep) of the original incoming response or nil if the
	// implementation does not provide it.
	//
	// The object received from this method contains an empty body, and all
	// payload related properties have zero value.
	OriginalResponse() *http.Response

	// This method is deprecated. A FilterContext implementation should flag this state
	// internally
	Served() bool

	// This method is deprecated. You should call Serve providing the desired response
	MarkServed()

	// Serve a request with the provided response. It can be used by filters that handle the requests
	// themselves. FilterContext implementations should flag this state and prevent the filter chain
	// from continuing
	Serve(*http.Response)

	// Provides the wildcard parameter values from the request path by their
	// name as the key.
	PathParam(string) string

	// Provides a read-write state bag, unique to a request and shared by all
	// the filters in the route.
	StateBag() map[string]interface{}

	// Gives filters access to the backend url specified in the route or an empty
	// value in case it's a shunt, loopback. In case of dynamic backend is empty.
	BackendUrl() string

	// Returns the host that will be set for the outgoing proxy request as the
	// 'Host' header.
	OutgoingHost() string

	// Allows explicitly setting the Host header to be sent to the backend, overriding the
	// strategy used by the implementation, which can be either the Host header from the
	// incoming request or the host fragment of the backend url.
	//
	// Filters that need to modify the outgoing 'Host' header, need to use
	// this method instead of setting the Request().Headers["Host"] value.
	// (The requestHeader filter automatically detects if the header name
	// is 'Host' and calls this method.)
	SetOutgoingHost(string)

	// Allow filters to collect metrics other than the default metrics (Filter Request, Filter Response methods)
	Metrics() Metrics

	// Allow filters to add Tags, Baggage to the trace or set the ComponentName.
	//
	// Deprecated: OpenTracing is deprecated, see https://github.com/zalando/skipper/issues/2104.
	// Use opentracing.SpanFromContext(ctx.Request().Context()).Tracer() to get the Tracer.
	Tracer() opentracing.Tracer

	// Allow filters to create their own spans
	//
	// Deprecated: OpenTracing is deprecated, see https://github.com/zalando/skipper/issues/2104.
	// Filter spans should be children of the request span,
	// use opentracing.SpanFromContext(ctx.Request().Context()) to get it.
	ParentSpan() opentracing.Span

	// Returns a clone of the FilterContext including a brand new request object.
	// The stream body of the new request is shared with the original.
	// Whenever the request body of the original request is read, the body of the
	// new request body is written.
	// The StateBag and filterMetrics object are not preserved in the new context.
	// Therefore, you can't access state bag values set in the previous context.
	Split() (FilterContext, error)

	// Performs a new route lookup and executes the matched route if any
	Loopback()

	// Performs a new route lookup and executes the matched route if any, keeping the response
	LoopbackWithResponse()

	Logger() FilterContextLogger
}

// FilterContextLogger is the logger which logs messages with additional context information.
type FilterContextLogger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// Metrics provides possibility to use custom metrics from filter implementations. The custom metrics will
// be exposed by the common metrics endpoint exposed by the proxy, where they can be accessed by the custom
// key prefixed by the filter name and the string 'custom'. E.g: <filtername>.custom.<customkey>.
type Metrics interface {
	// MeasureSince adds values to a timer with a custom key.
	MeasureSince(key string, start time.Time)

	// IncCounter increments a custom counter identified by its key.
	IncCounter(key string)

	// IncCounterBy increments a custom counter identified by its key by a certain value.
	IncCounterBy(key string, value int64)

	// IncFloatCounterBy increments a custom counter identified by its key by a certain
	// float (decimal) value. IMPORTANT: Not all Metrics implementation support float
	// counters. In that case, a call to IncFloatCounterBy is dropped.
	IncFloatCounterBy(key string, value float64)
}

// Filter is created by the Spec components, optionally using filter
// specific settings. When implementing filters, it needs to be taken
// into consideration, that filter instances are route specific and not
// request specific, so any state stored with a filter is shared between
// all requests for the same route and can cause concurrency issues.
type Filter interface {
	// The Request method is called while processing the incoming request.
	Request(FilterContext)

	// The Response method is called while processing the response to be
	// returned.
	Response(FilterContext)
}

// FilterCloser are Filters that need to cleanup resources after
// filter termination. For example Filters, that create a goroutine
// for some reason need to cleanup their goroutine or they would leak
// goroutines.
type FilterCloser interface {
	Filter
	io.Closer
}

// Spec objects are specifications for filters. When initializing the routes,
// the Filter instances are created using the Spec objects found in the
// registry.
type Spec interface {
	// Name gives the name of the Spec. It is used to identify filters in a route definition.
	Name() string

	// CreateFilter creates a Filter instance. Called with the parameters in the route
	// definition while initializing a route.
	CreateFilter(config []interface{}) (Filter, error)
}

// Registry used to lookup Spec objects while initializing routes.
type Registry map[string]Spec

// ErrInvalidFilterParameters is used in case of invalid filter parameters.
var ErrInvalidFilterParameters = errors.New("invalid filter parameters")

// Register a filter specification.
func (r Registry) Register(s Spec) {
	name := s.Name()
	if _, ok := r[name]; ok {
		log.Infof("Replacing %s filter specification", name)
	}
	r[name] = s
}

// All Skipper filter names
const (
	BackendIsProxyName                         = "backendIsProxy"
	CommentName                                = "comment"
	AnnotateName                               = "annotate"
	ModRequestHeaderName                       = "modRequestHeader"
	SetRequestHeaderName                       = "setRequestHeader"
	AppendRequestHeaderName                    = "appendRequestHeader"
	DropRequestHeaderName                      = "dropRequestHeader"
	ModResponseHeaderName                      = "modResponseHeader"
	SetResponseHeaderName                      = "setResponseHeader"
	AppendResponseHeaderName                   = "appendResponseHeader"
	DropResponseHeaderName                     = "dropResponseHeader"
	SetContextRequestHeaderName                = "setContextRequestHeader"
	AppendContextRequestHeaderName             = "appendContextRequestHeader"
	SetContextResponseHeaderName               = "setContextResponseHeader"
	AppendContextResponseHeaderName            = "appendContextResponseHeader"
	CopyRequestHeaderName                      = "copyRequestHeader"
	CopyResponseHeaderName                     = "copyResponseHeader"
	EncodeRequestHeaderName                    = "encodeRequestHeader"
	EncodeResponseHeaderName                   = "encodeResponseHeader"
	DropRequestHeaderValueRegexpName           = "dropRequestHeaderRegexp"
	DropResponseHeaderValueRegexpName          = "dropResponseHeaderRegexp"
	ModPathName                                = "modPath"
	SetPathName                                = "setPath"
	RedirectToName                             = "redirectTo"
	RedirectToLowerName                        = "redirectToLower"
	StaticName                                 = "static"
	StripQueryName                             = "stripQuery"
	PreserveHostName                           = "preserveHost"
	StatusName                                 = "status"
	CompressName                               = "compress"
	DecompressName                             = "decompress"
	SetQueryName                               = "setQuery"
	DropQueryName                              = "dropQuery"
	InlineContentName                          = "inlineContent"
	InlineContentIfStatusName                  = "inlineContentIfStatus"
	FlowIdName                                 = "flowId"
	XforwardName                               = "xforward"
	XforwardFirstName                          = "xforwardFirst"
	RandomContentName                          = "randomContent"
	RepeatContentName                          = "repeatContent"
	RepeatContentHexName                       = "repeatContentHex"
	WrapContentName                            = "wrapContent"
	WrapContentHexName                         = "wrapContentHex"
	BackendTimeoutName                         = "backendTimeout"
	ReadTimeoutName                            = "readTimeout"
	WriteTimeoutName                           = "writeTimeout"
	BlockName                                  = "blockContent"
	BlockHexName                               = "blockContentHex"
	LatencyName                                = "latency"
	BandwidthName                              = "bandwidth"
	ChunksName                                 = "chunks"
	BackendLatencyName                         = "backendLatency"
	BackendBandwidthName                       = "backendBandwidth"
	BackendChunksName                          = "backendChunks"
	TarpitName                                 = "tarpit"
	AbsorbName                                 = "absorb"
	AbsorbSilentName                           = "absorbSilent"
	UniformRequestLatencyName                  = "uniformRequestLatency"
	UniformResponseLatencyName                 = "uniformResponseLatency"
	NormalRequestLatencyName                   = "normalRequestLatency"
	NormalResponseLatencyName                  = "normalResponseLatency"
	HistogramRequestLatencyName                = "histogramRequestLatency"
	HistogramResponseLatencyName               = "histogramResponseLatency"
	LogBodyName                                = "logBody"
	LogHeaderName                              = "logHeader"
	TeeName                                    = "tee"
	TeenfName                                  = "teenf"
	TeeLoopbackName                            = "teeLoopback"
	SedName                                    = "sed"
	SedDelimName                               = "sedDelim"
	SedRequestName                             = "sedRequest"
	SedRequestDelimName                        = "sedRequestDelim"
	BasicAuthName                              = "basicAuth"
	WebhookName                                = "webhook"
	OAuthTokeninfoAnyScopeName                 = "oauthTokeninfoAnyScope"
	OAuthTokeninfoAllScopeName                 = "oauthTokeninfoAllScope"
	OAuthTokeninfoAnyKVName                    = "oauthTokeninfoAnyKV"
	OAuthTokeninfoAllKVName                    = "oauthTokeninfoAllKV"
	OAuthTokeninfoValidateName                 = "oauthTokeninfoValidate"
	OAuthTokenintrospectionAnyClaimsName       = "oauthTokenintrospectionAnyClaims"
	OAuthTokenintrospectionAllClaimsName       = "oauthTokenintrospectionAllClaims"
	OAuthTokenintrospectionAnyKVName           = "oauthTokenintrospectionAnyKV"
	OAuthTokenintrospectionAllKVName           = "oauthTokenintrospectionAllKV"
	SecureOAuthTokenintrospectionAnyClaimsName = "secureOauthTokenintrospectionAnyClaims"
	SecureOAuthTokenintrospectionAllClaimsName = "secureOauthTokenintrospectionAllClaims"
	SecureOAuthTokenintrospectionAnyKVName     = "secureOauthTokenintrospectionAnyKV"
	SecureOAuthTokenintrospectionAllKVName     = "secureOauthTokenintrospectionAllKV"
	ForwardTokenName                           = "forwardToken"
	ForwardTokenFieldName                      = "forwardTokenField"
	OAuthGrantName                             = "oauthGrant"
	GrantCallbackName                          = "grantCallback"
	GrantLogoutName                            = "grantLogout"
	GrantClaimsQueryName                       = "grantClaimsQuery"
	JwtValidationName                          = "jwtValidation"
	JwtMetricsName                             = "jwtMetrics"
	OAuthOidcUserInfoName                      = "oauthOidcUserInfo"
	OAuthOidcAnyClaimsName                     = "oauthOidcAnyClaims"
	OAuthOidcAllClaimsName                     = "oauthOidcAllClaims"
	OidcClaimsQueryName                        = "oidcClaimsQuery"
	DropRequestCookieName                      = "dropRequestCookie"
	DropResponseCookieName                     = "dropResponseCookie"
	RequestCookieName                          = "requestCookie"
	ResponseCookieName                         = "responseCookie"
	JsCookieName                               = "jsCookie"
	ConsecutiveBreakerName                     = "consecutiveBreaker"
	RateBreakerName                            = "rateBreaker"
	DisableBreakerName                         = "disableBreaker"
	AdmissionControlName                       = "admissionControl"
	ClientRatelimitName                        = "clientRatelimit"
	RatelimitName                              = "ratelimit"
	ClusterClientRatelimitName                 = "clusterClientRatelimit"
	ClusterRatelimitName                       = "clusterRatelimit"
	ClusterLeakyBucketRatelimitName            = "clusterLeakyBucketRatelimit"
	BackendRateLimitName                       = "backendRatelimit"
	RatelimitFailClosedName                    = "ratelimitFailClosed"
	LuaName                                    = "lua"
	CorsOriginName                             = "corsOrigin"
	HeaderToQueryName                          = "headerToQuery"
	QueryToHeaderName                          = "queryToHeader"
	DisableAccessLogName                       = "disableAccessLog"
	MaskAccessLogQueryName                     = "maskAccessLogQuery"
	EnableAccessLogName                        = "enableAccessLog"
	AuditLogName                               = "auditLog"
	UnverifiedAuditLogName                     = "unverifiedAuditLog"
	SetDynamicBackendHostFromHeader            = "setDynamicBackendHostFromHeader"
	SetDynamicBackendSchemeFromHeader          = "setDynamicBackendSchemeFromHeader"
	SetDynamicBackendUrlFromHeader             = "setDynamicBackendUrlFromHeader"
	SetDynamicBackendHost                      = "setDynamicBackendHost"
	SetDynamicBackendScheme                    = "setDynamicBackendScheme"
	SetDynamicBackendUrl                       = "setDynamicBackendUrl"
	ApiUsageMonitoringName                     = "apiUsageMonitoring"
	FifoName                                   = "fifo"
	FifoWithBodyName                           = "fifoWithBody"
	LifoName                                   = "lifo"
	LifoGroupName                              = "lifoGroup"
	RfcPathName                                = "rfcPath"
	RfcHostName                                = "rfcHost"
	BearerInjectorName                         = "bearerinjector"
	SetRequestHeaderFromSecretName             = "setRequestHeaderFromSecret"
	TracingBaggageToTagName                    = "tracingBaggageToTag"
	StateBagToTagName                          = "stateBagToTag"
	TracingTagName                             = "tracingTag"
	TracingTagFromResponseName                 = "tracingTagFromResponse"
	TracingTagFromResponseIfStatusName         = "tracingTagFromResponseIfStatus"
	TracingSpanNameName                        = "tracingSpanName"
	OriginMarkerName                           = "originMarker"
	FadeInName                                 = "fadeIn"
	EndpointCreatedName                        = "endpointCreated"
	ConsistentHashKeyName                      = "consistentHashKey"
	ConsistentHashBalanceFactorName            = "consistentHashBalanceFactor"
	OpaAuthorizeRequestName                    = "opaAuthorizeRequest"
	OpaAuthorizeRequestWithBodyName            = "opaAuthorizeRequestWithBody"
	OpaServeResponseName                       = "opaServeResponse"
	OpaServeResponseWithReqBodyName            = "opaServeResponseWithReqBody"
	TLSName                                    = "tlsPassClientCertificates"
	AWSSigV4Name                               = "awsSigv4"
	LoopbackIfStatus                           = "loopbackIfStatus"

	// Undocumented filters
	HealthCheckName        = "healthcheck"
	SetFastCgiFilenameName = "setFastCgiFilename"
	DisableRatelimitName   = "disableRatelimit"
	UnknownRatelimitName   = "unknownRatelimit"
)
