// Package skipper contains the main interface definitions of the program. The implementation packages use these
// interfaces to interact with each other instead of referencing directly. Some of these interfaces have mock
// implementation in the package called 'mock for testing purposes.
package skipper

import "net/http"

// Wrapper interface for receiving data from the etcd configuration storage.
// Note: the current json format will be replaced soon with a more maintainable routing specification format.
type RawData interface {

	// return the current routing settings as eskip
	//
	// pdp:
	//  PathRegexp(`.*\\.html`) ->
	//  customHeaders(3.14) ->
	//  xSessionId("v4") ->
	//  "https://www.zalando.de/pdp.html";
	//
	// humanstxt:
	//  Path(`humans.txt`) ->
	//  xSessionId("v4") ->
	//  humanstxt() ->
	//  <shunt>
	//
	Get() string
}

// Client receiving the configuraton from etcd or other.
type DataClient interface {

	// Returns a channel that sends the the data on initial start, and on any update in the
	// configuration. The channel blocks between two updates.
	Receive() <-chan RawData
}

// Backend definition parsed from the config data and used by the proxy.
type Backend interface {

	// http or https
	Scheme() string

	// server.example.com
	Host() string

	// shunt backends do not make requests to a server
	// they need a filter to initialize the response, otherwise the proxy will response 404
	IsShunt() bool
}

// Context object providing the request and response objects to the filters.
type FilterContext interface {
	ResponseWriter() http.ResponseWriter
	Request() *http.Request
	Response() *http.Response
}

// Filters are created by the FilterSpec components, optionally using filter specific settings.
// When implementing filters, it needs to be taken into consideration, that filter instances are route specific
// and not request specific, so any state stored with a filter is shared between all requests and can cause
// concurrency issues (as in don't do that).
type Filter interface {

	// The id of a filter, set from the configuration and used mainly for logging purpose.
	Id() string

	// The request method is called on a filter on incoming requests. At this stage, the
	// FilterContext.Response() method returns nil.
	Request(FilterContext)

	// The response method is called on a filter after the response was received from the backend. At this
	// stage, the FilterContext.Response() method returns the response object.
	Response(FilterContext)
}

// Routes are created based on the configuration data and provided to the proxy from the current settings,
// selected by the matching rules for each request.
type Route interface {

	// Tells the proxy which backend should be used when processing a request.
	Backend() Backend

	// Tells the proxy which set of filters should be applied to a request and the resulting response.
	Filters() []Filter
}

// Contains the routing rules and other settings.
type Settings interface {

	// Returns the matching route for a given request.
	Route(*http.Request) (Route, error)
}

// A SettingsSource object always sends the current settings to the channel passed in to Subscribe in a
// non-blocking way.
type SettingsSource interface {

	// Accepts a channel on which the calling code can receive the the current Settings anytime without
	// waiting for it.
	// It may be a good idea to use buffered channels in production environment.
	Subscribe(chan<- Settings)
}

// A dispatcher object for settings. To send the latest available settings to any request processing or other
// goroutines without blocking, clients who use the Subscribe method will always receive the latest settings,
// while goroutines responsible to process the incoming config data and create the next valid settings object
// can dispatch the new settings with the Push method.
type SettingsDispatcher interface {

	// Clients can use the subscribe method to receive the current settings.
	SettingsSource

	// When new settings are ready, use the returned channel to propagate them.
	Push() chan<- Settings
}

// Filter specific configuration.
type FilterConfig []interface{}

// FilterSpec objects can be used to create filter objects. They need to be registered in the registry.
// Typically, there is a single FilterSpec instance of each implementation in a running process, which can create multiple filter
// instances with different config defined in the configuration on every update.
type FilterSpec interface {

	// The name of the FilterSpec is used to identify in the configuration which spec a filter is based on.
	Name() string

	// When the program settings are updated, and they contain filters based on a spec, MakeFilter is
	// called, and the filter id and the filter specific settings are provided. Returns a filter.
	MakeFilter(id string, s FilterConfig) (Filter, error)
}

// The filter registry stores all available filter spec modules.
type FilterRegistry interface {
	Add(...FilterSpec)
	Get(string) FilterSpec
	Remove(string)
}
