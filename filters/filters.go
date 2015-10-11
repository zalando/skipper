package filters

import "net/http"

// Context object providing the request and response objects to the filters.
type FilterContext interface {
	ResponseWriter() http.ResponseWriter
	Request() *http.Request
	Response() *http.Response
	Served() bool
	MarkServed()
	PathParam(string) string
	StateBag() map[string]interface{}
}

// Filters are created by the Spec components, optionally using filter specific settings.
// When implementing filters, it needs to be taken into consideration, that filter instances are route specific
// and not request specific, so any state stored with a filter is shared between all requests and can cause
// concurrency issues (as in don't do that).
type Filter interface {

	// The request method is called on a filter on incoming requests. At this stage, the
	// FilterContext.Response() method returns nil.
	Request(FilterContext)

	// The response method is called on a filter after the response was received from the backend. At this
	// stage, the FilterContext.Response() method returns the response object.
	Response(FilterContext)
}

// Spec objects can be used to create filter objects. They need to be registered in the registry.
// Typically, there is a single Spec instance of each implementation in a running process, which can create multiple filter
// instances with different config defined in the configuration on every update.
type Spec interface {

	// The name of the Spec is used to identify in the configuration which spec a filter is based on.
	Name() string

	// When the program settings are updated, and they contain filters based on a spec, CreateFilter is
	// called, and the filter id and the filter specific settings are provided. Returns a filter.
	CreateFilter(config []interface{}) (Filter, error)
}

type Registry map[string]Spec

func (r Registry) Register(s Spec) {
	r[s.Name()] = s
}

func Defaults() Registry {
	defaultSpecs := []Spec{
		CreateRequestHeader(),
		CreateResponseHeader(),
		&ModPath{},
		&HealthCheck{},
		&Static{},
		&Redirect{},
		&StripQuery{}}

	r := make(Registry)
	for _, s := range defaultSpecs {
		r.Register(s)
	}

	return r
}
