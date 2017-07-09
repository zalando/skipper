package routing

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"encoding/json"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
)

const (
	// Name of the builtin path predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://godoc.org/github.com/zalando/skipper/eskip)
	PathName = "Path"

	// Name of the builtin path subtree predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://godoc.org/github.com/zalando/skipper/eskip)
	PathSubtreeName = "PathSubtree"

	routeTableTimestampHeaderName = "X-Skipper-Route-Table-Timestamp"
	defaultRouteListingLimit      = 1024
)

// Control flags for route matching.
type MatchingOptions uint

const (
	// All options are default.
	MatchingOptionsNone MatchingOptions = 0

	// Ignore trailing slash in paths.
	IgnoreTrailingSlash MatchingOptions = 1 << iota
)

func (o MatchingOptions) ignoreTrailingSlash() bool {
	return o&IgnoreTrailingSlash > 0
}

// DataClient instances provide data sources for
// route definitions.
type DataClient interface {
	LoadAll() ([]*eskip.Route, error)
	LoadUpdate() ([]*eskip.Route, []string, error)
}

// Predicate instances are used as custom user defined route
// matching predicates.
type Predicate interface {

	// Returns true if the request matches the predicate.
	Match(*http.Request) bool
}

// PredicateSpec instances are used to create custom predicates
// (of type Predicate) with concrete arguments during the
// construction of the routing tree.
type PredicateSpec interface {

	// Name of the predicate as used in the route definitions.
	Name() string

	// Creates a predicate instance with concrete arguments.
	Create([]interface{}) (Predicate, error)
}

// Initialization options for routing.
type Options struct {

	// Registry containing the available filter
	// specifications that are used during processing
	// the filter chains in the route definitions.
	FilterRegistry filters.Registry

	// Matching options are flags that control the
	// route matching.
	MatchingOptions MatchingOptions

	// The timeout between requests to the data
	// clients for route definition updates.
	PollTimeout time.Duration

	// The set of different data clients where the
	// route definitions are read from.
	DataClients []DataClient

	// Specifications of custom, user defined predicates.
	Predicates []PredicateSpec

	// Performance tuning option.
	//
	// When zero, the newly constructed routing
	// tree will take effect on the next routing
	// query after every update from the data
	// clients. In case of higher values, the
	// routing queries have priority over the
	// update channel, but the next routing tree
	// takes effect only a few requests later.
	//
	// (Currently disabled and used with hard wired
	// 0, until the performance benefit is verified
	// by benchmarks.)
	UpdateBuffer int

	// Set a custom logger if necessary.
	Log logging.Logger
}

// Filter contains extensions to generic filter
// interface, serving mainly logging/monitoring
// purpose.
type RouteFilter struct {
	filters.Filter
	Name  string
	Index int
}

// Route object with preprocessed filter instances.
type Route struct {

	// Fields from the static route definition.
	eskip.Route

	// path predicate matching a subtree
	path string

	// path predicate matching a subtree
	pathSubtree string

	// The backend scheme and host.
	Scheme, Host string

	// The preprocessed custom predicate instances.
	Predicates []Predicate

	// The preprocessed filter instances.
	Filters []*RouteFilter
}

// Routing ('router') instance providing live
// updatable request matching.
type Routing struct {
	routeTable atomic.Value // of struct routeTable
	log        logging.Logger
	quit       chan struct{}
}

// New initializes a routing instance, and starts listening for route
// definition updates.
func New(o Options) *Routing {
	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	r := &Routing{log: o.Log, quit: make(chan struct{})}
	initialMatcher, _ := newMatcher(nil, MatchingOptionsNone)
	rt := &routeTable{
		m:       initialMatcher,
		created: time.Now().UTC(),
	}
	r.routeTable.Store(rt)
	r.startReceivingUpdates(o)
	return r
}

// ServeHTTP ...
func (r *Routing) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rt := r.routeTable.Load().(*routeTable)
	req.ParseForm()
	createdUnix := strconv.FormatInt(rt.created.Unix(), 10)

	ts := req.Form.Get("timestamp")
	if ts != "" {
		if createdUnix != ts {
			http.Error(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}

	offset := 0
	if off := req.Form.Get("offset"); off != "" {
		off, err := strconv.Atoi(off)
		if err != nil {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		if off < 0 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = off
	}

	limit := defaultRouteListingLimit
	if lim := req.Form.Get("limit"); lim != "" {
		lim, err := strconv.Atoi(lim)
		if err != nil {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		if lim < 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = lim
	}

	accept := req.Header.Get("Accept")
	routes := slice(rt.validRoutes, offset, limit)

	if accept == "application/json" {
		w.Header().Set(routeTableTimestampHeaderName, createdUnix)
		w.Header().Set("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(routes); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	if accept == "application/eskip" {
		w.Header().Set(routeTableTimestampHeaderName, createdUnix)
		w.Header().Set("content-type", "application/eskip")
		eskipRoutes := make([]*eskip.Route, len(routes))
		for i, r := range routes {
			eskipRoutes[i] = &r.Route
		}
		fmt.Fprint(w, eskip.String(eskipRoutes...))
		return
	}

	http.Error(w, http.StatusText(http.StatusNotAcceptable), http.StatusNotAcceptable)
}

func (r *Routing) startReceivingUpdates(o Options) {
	c := make(chan *routeTable)
	go receiveRouteMatcher(o, c, r.quit)
	go func() {
		for {
			select {
			case rt := <-c:
				r.routeTable.Store(rt)
				r.log.Info("route settings applied")
			case <-r.quit:
				return
			}
		}
	}()
}

// Route matches a request in the current routing tree.
//
// If the request matches a route, returns the route and a map of
// parameters constructed from the wildcard parameters in the path
// condition if any. If there is no match, it returns nil.
func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	rt := r.routeTable.Load().(*routeTable)
	return rt.m.match(req)
}

// Close closes routing, stops receiving routes.
func (r *Routing) Close() {
	close(r.quit)
}

func slice(r []*Route, offset int, limit int) []*Route {
	if offset > len(r) {
		offset = len(r)
	}
	end := offset + limit
	if end > len(r) {
		end = len(r)
	}
	result := r[offset:end]
	if result == nil {
		return []*Route{}
	}
	return result
}
