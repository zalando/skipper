package filters

import (
	"errors"
	"net/http"
)

const (
	RequestHeaderName  = "requestHeader"
	ResponseHeaderName = "responseHeader"
	ModPathName        = "modPath"
	RedirectName       = "redirect"
	StaticName         = "static"
	StripQueryName     = "stripQuery"
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

	// Returns true if the request was served by any of the filters in a
	// route.
	Served() bool

	// Marks a request served. Used by filters that handle the requests
	// themselves.
	MarkServed()

	// Provides the wildcard parameter values from the request path by their
	// name as the key.
	PathParam(string) string

	// Provides a read-write state bag, unique to a request and shared by all
	// the filters in the route.
	StateBag() map[string]interface{}
}

// Error used in case of invalid filter parameters.
var ErrInvalidFilterParameters = errors.New("Invalid filter parameters")

// Filters are created by the Spec components, optionally using filter specific settings.
// When implementing filters, it needs to be taken into consideration, that filter instances are route specific
// and not request specific, so any state stored with a filter is shared between all requests and can cause
// concurrency issues (as in don't do that).
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

	// The name of the Spec is used to identify filters in a route definition.
	Name() string

	// Creates a Filter instance. Called with the arguments in the route
	// definition while initializing a route.
	CreateFilter(config []interface{}) (Filter, error)
}

// Registry used to lookup Spec objects while initializing routes.
type Registry map[string]Spec

// Registers a filter specification.
func (r Registry) Register(s Spec) {
	r[s.Name()] = s
}

// Returns a Registry object initialized with the default set of filter
// specifications found in the filters package.
func Defaults() Registry {
	defaultSpecs := []Spec{
		NewRequestHeader(),
		NewResponseHeader(),
		NewModPath(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirect(),
		NewStripQuery(),
		NewFlowId()}

	r := make(Registry)
	for _, s := range defaultSpecs {
		r.Register(s)
	}

	return r
}
