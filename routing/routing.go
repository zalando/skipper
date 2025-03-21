package routing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/predicates"
)

const (
	// Deprecated, use predicates.PathName instead
	PathName = predicates.PathName

	// Deprecated, use predicates.PathSubtreeName instead
	PathSubtreeName = predicates.PathSubtreeName

	// Deprecated, use predicates.WeightName instead
	WeightPredicateName = predicates.WeightName

	routesTimestampName      = "X-Timestamp"
	RoutesCountName          = "X-Count"
	defaultRouteListingLimit = 1024
)

// MatchingOptions controls route matching.
type MatchingOptions uint

const (
	// MatchingOptionsNone indicates that all options are default.
	MatchingOptionsNone MatchingOptions = 0

	// IgnoreTrailingSlash indicates that trailing slashes in paths are ignored.
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

type WeightedPredicateSpec interface {
	PredicateSpec

	// Extra Weight of the predicate
	Weight() int
}

// Options for initialization for routing.
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

	// SuppressLogs indicates whether to log only a summary of the route changes.
	SuppressLogs bool

	// Metrics is used to collect monitoring data about the routes health, including
	// total number of routes applied and UNIX time of the last routes update.
	Metrics metrics.Metrics

	// PreProcessors contains custom eskip.Route pre-processors.
	PreProcessors []PreProcessor

	// PostProcessors contains custom route post-processors.
	PostProcessors []PostProcessor

	// SignalFirstLoad enables signaling on the first load
	// of the routing configuration during the startup.
	SignalFirstLoad bool
}

// RouteFilter contains extensions to generic filter
// interface, serving mainly logging/monitoring
// purpose.
type RouteFilter struct {
	filters.Filter
	Name string

	// Deprecated: currently not used, and post-processors may not maintain a correct value
	Index int
}

// LBEndpoint represents the scheme and the host of load balanced
// backends.
type LBEndpoint struct {
	Scheme, Host string
	Metrics      Metrics
}

// LBAlgorithm implementations apply a load balancing algorithm
// over the possible endpoints of a load balanced route.
type LBAlgorithm interface {
	Apply(*LBContext) LBEndpoint
}

// LBContext is used to pass data to the load balancer to decide based
// on that data which endpoint to call from the backends
type LBContext struct {
	Request     *http.Request
	Route       *Route
	LBEndpoints []LBEndpoint
	Params      map[string]interface{}
}

// NewLBContext is used to create a new LBContext, to pass data to the
// load balancer algorithms.
// Deprecated: create LBContext instead
func NewLBContext(r *http.Request, rt *Route) *LBContext {
	return &LBContext{
		Request: r,
		Route:   rt,
	}
}

// Route object with preprocessed filter instances.
type Route struct {

	// Fields from the static route definition.
	eskip.Route

	// weight used internally, received from the Weight() predicates.
	weight int

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

	// LBEndpoints contain the possible endpoints of a load
	// balanced route.
	LBEndpoints []LBEndpoint

	// LBAlgorithm is the selected load balancing algorithm
	// of a load balanced route.
	LBAlgorithm LBAlgorithm

	// LBFadeInDuration defines the duration of the fade-in
	// function to be applied to new LB endpoints associated
	// with this route.
	LBFadeInDuration time.Duration

	// LBExponent defines a secondary exponent modifier of
	// the fade-in function configured mainly by the LBFadeInDuration
	// field, adjusting the shape of the fade-in. By default,
	// its value is usually 1, meaning linear fade-in, and it's
	// configured by the post-processor found in the filters/fadein
	// package.
	LBFadeInExponent float64
}

// PostProcessor is an interface for custom post-processors applying changes
// to the routes after they were created from their data representation and
// before they were passed to the proxy.
type PostProcessor interface {
	Do([]*Route) []*Route
}

// PreProcessor is an interface for custom pre-processors applying changes
// to the routes before they were created from eskip.Route representation.
type PreProcessor interface {
	Do([]*eskip.Route) []*eskip.Route
}

// Routing ('router') instance providing live
// updatable request matching.
type Routing struct {
	routeTable        atomic.Value // of struct routeTable
	log               logging.Logger
	firstLoad         chan struct{}
	firstLoadSignaled bool
	quit              chan struct{}
	metrics           metrics.Metrics
}

// New initializes a routing instance, and starts listening for route
// definition updates.
func New(o Options) *Routing {
	if o.Log == nil {
		o.Log = &logging.DefaultLog{}
	}

	r := &Routing{log: o.Log, firstLoad: make(chan struct{}), quit: make(chan struct{})}
	r.metrics = o.Metrics
	if !o.SignalFirstLoad {
		close(r.firstLoad)
		r.firstLoadSignaled = true
	}

	initialMatcher, _ := newMatcher(nil, MatchingOptionsNone)
	rt := &routeTable{
		m:       initialMatcher,
		created: time.Now(),
	}
	r.routeTable.Store(rt)
	r.startReceivingUpdates(o)
	return r
}

// ServeHTTP renders the list of current routes.
func (r *Routing) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" && req.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rt := r.routeTable.Load().(*routeTable)
	req.ParseForm()
	createdUnix := strconv.FormatInt(rt.created.Unix(), 10)
	ts := req.Form.Get("timestamp")
	if ts != "" && createdUnix != ts {
		http.Error(w, "invalid timestamp", http.StatusBadRequest)
		return
	}

	if req.Method == "HEAD" {
		w.Header().Set(routesTimestampName, createdUnix)
		w.Header().Set(RoutesCountName, strconv.Itoa(len(rt.validRoutes)))

		if strings.Contains(req.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}

		return
	}

	offset, err := extractParam(req, "offset", 0)
	if err != nil {
		http.Error(w, "invalid offset", http.StatusBadRequest)
		return
	}

	limit, err := extractParam(req, "limit", defaultRouteListingLimit)
	if err != nil {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return
	}

	w.Header().Set(routesTimestampName, createdUnix)
	w.Header().Set(RoutesCountName, strconv.Itoa(len(rt.validRoutes)))

	routes := slice(rt.validRoutes, offset, limit)
	if strings.Contains(req.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(routes); err != nil {
			http.Error(
				w,
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError,
			)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	eskip.Fprint(w, extractPretty(req), routes...)
}

func (r *Routing) startReceivingUpdates(o Options) {
	c := make(chan *routeTable)
	go receiveRouteMatcher(o, c, r.quit)
	go func() {
		for {
			select {
			case rt := <-c:
				r.routeTable.Store(rt)
				if !r.firstLoadSignaled {
					if len(rt.clients) == len(o.DataClients) {
						close(r.firstLoad)
						r.firstLoadSignaled = true
					}
				}
				r.log.Infof("route settings applied, id: %d", rt.id)
				if r.metrics != nil { // existing codebases might not supply metrics instance
					r.metrics.UpdateGauge("routes.total", float64(len(rt.validRoutes)))
					r.metrics.UpdateGauge("routes.updated_timestamp", float64(rt.created.Unix()))
					r.metrics.MeasureSince("routes.update_latency", rt.created)
				}
			case <-r.quit:
				var rt *routeTable
				rt, ok := r.routeTable.Load().(*routeTable)
				if ok {
					rt.close()
				}
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

// FirstLoad, when enabled, blocks until the first routing configuration was received
// by the routing during the startup. When disabled, it doesn't block.
func (r *Routing) FirstLoad() <-chan struct{} {
	return r.firstLoad
}

// RouteLookup captures a single generation of the lookup tree, allowing multiple
// lookups to the same version of the lookup tree.
//
// Experimental feature. Using this solution potentially can cause large memory
// consumption in extreme cases, typically when:
// the total number routes is large, the backend responses to a subset of these
// routes is slow, and there's a rapid burst of consecutive updates to the
// routing table. This situation is considered an edge case, but until a protection
// against is found, the feature is experimental and its exported interface may
// change.
type RouteLookup struct {
	rt *routeTable
}

// Do executes the lookup against the captured routing table. Equivalent to
// Routing.Route().
func (rl *RouteLookup) Do(req *http.Request) (*Route, map[string]string) {
	return rl.rt.m.match(req)
}

// Get returns a captured generation of the lookup table. This feature is
// experimental. See the description of the RouteLookup type.
func (r *Routing) Get() *RouteLookup {
	rt := r.routeTable.Load().(*routeTable)
	return &RouteLookup{rt: rt}
}

// Close closes routing, routeTable and stops statemachine for receiving routes.
func (r *Routing) Close() {
	close(r.quit)
}

func slice(r []*eskip.Route, offset int, limit int) []*eskip.Route {
	if offset > len(r) {
		offset = len(r)
	}
	end := offset + limit
	if end > len(r) {
		end = len(r)
	}
	result := r[offset:end]
	if result == nil {
		return []*eskip.Route{}
	}
	return result
}

func extractParam(r *http.Request, key string, defaultValue int) (int, error) {
	param := r.Form.Get(key)
	if param == "" {
		return defaultValue, nil
	}
	val, err := strconv.Atoi(param)
	if err != nil {
		return 0, err
	}
	if val < 0 {
		return 0, fmt.Errorf("invalid value `%d` for `%s`", val, key)
	}
	return val, nil
}

func extractPretty(r *http.Request) eskip.PrettyPrintInfo {
	vals, ok := r.Form["nopretty"]
	if !ok || len(vals) == 0 {
		return eskip.PrettyPrintInfo{Pretty: true, IndentStr: "  "}
	}
	val := vals[0]
	if val == "0" || val == "false" {
		return eskip.PrettyPrintInfo{Pretty: true, IndentStr: "  "}
	}
	return eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}
}
