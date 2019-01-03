package filters

import (
	"errors"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
)

// Context object providing state and information that is unique to a request.
type FilterContext interface {
	// The response writer object belonging to the incoming request. Used by
	// filters that handle the requests themselves.
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
	// value in case it's a shunt or loopback
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
	Tracer() opentracing.Tracer

	// Allow filters to create their own spans
	ParentSpan() opentracing.Span
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

// Filters are created by the Spec components, optionally using filter
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

// Error used in case of invalid filter parameters.
var ErrInvalidFilterParameters = errors.New("invalid filter parameters")

// Registers a filter specification.
func (r Registry) Register(s Spec) {
	r[s.Name()] = s
}
