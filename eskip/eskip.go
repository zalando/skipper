package eskip

//go:generate goyacc -o parser.go -p eskip parser.y

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/zalando/skipper/filters/flowid"
)

const duplicateHeaderPredicateErrorFmt = "duplicate header predicate: %s"

var (
	invalidPredicateArgError        = errors.New("invalid predicate arg")
	invalidPredicateArgCountError   = errors.New("invalid predicate count arg")
	duplicatePathTreePredicateError = errors.New("duplicate path tree predicate")
	duplicateMethodPredicateError   = errors.New("duplicate method predicate")
)

// DefaultFilters implements the routing.PreProcessor interface and
// should be used with the routing package.
type DefaultFilters struct {
	Prepend []*Filter
	Append  []*Filter
}

// Do implements the interface routing.PreProcessor. It appends and
// prepends filters stored to incoming routes and returns the modified
// version of routes.
func (df *DefaultFilters) Do(routes []*Route) []*Route {
	pn := len(df.Prepend)
	an := len(df.Append)
	if pn == 0 && an == 0 {
		return routes
	}

	nextRoutes := make([]*Route, len(routes))
	for i, r := range routes {
		nextRoutes[i] = new(Route)
		*nextRoutes[i] = *r

		fn := len(r.Filters)

		filters := make([]*Filter, fn+pn+an)
		copy(filters[:pn], df.Prepend)
		copy(filters[pn:pn+fn], r.Filters)
		copy(filters[pn+fn:], df.Append)

		nextRoutes[i].Filters = filters
	}

	return nextRoutes
}

// Represents a matcher condition for incoming requests.
type matcher struct {
	// The name of the matcher, e.g. Path or Header
	name string

	// The args of the matcher, e.g. the path to be matched.
	args []interface{}
}

// BackendType indicates whether a route is a network backend, a shunt or a loopback.
type BackendType int

const (
	NetworkBackend = iota
	ShuntBackend
	LoopBackend
	DynamicBackend
	LBBackend
)

// Route definition used during the parser processes the raw routing
// document.
type parsedRoute struct {
	id          string
	matchers    []*matcher
	filters     []*Filter
	shunt       bool
	loopback    bool
	dynamic     bool
	lbBackend   bool
	backend     string
	lbAlgorithm string
	lbEndpoints []string
}

// A Predicate object represents a parsed, in-memory, route matching predicate
// that is defined by extensions.
type Predicate struct {
	// The name of the custom predicate as referenced
	// in the route definition. E.g. 'Foo'.
	Name string `json:"name"`

	// The arguments of the predicate as defined in the
	// route definition. The arguments can be of type
	// float64 or string (string for both strings and
	// regular expressions).
	Args []interface{} `json:"args"`
}

// A Filter object represents a parsed, in-memory filter expression.
type Filter struct {
	// name of the filter specification
	Name string `json:"name"`

	// filter args applied within a particular route
	Args []interface{} `json:"args"`
}

// A Route object represents a parsed, in-memory route definition.
type Route struct {
	// Id of the route definition.
	// E.g. route1: ...
	Id string

	// Deprecated, use Predicate instances with the name "Path".
	//
	// Exact path to be matched.
	// E.g. Path("/some/path")
	Path string

	// Host regular expressions to match.
	// E.g. Host(/[.]example[.]org/)
	HostRegexps []string

	// Path regular expressions to match.
	// E.g. PathRegexp(/\/api\//)
	PathRegexps []string

	// Method to match.
	// E.g. Method("HEAD")
	Method string

	// Exact header definitions to match.
	// E.g. Header("Accept", "application/json")
	Headers map[string]string

	// Header regular expressions to match.
	// E.g. HeaderRegexp("Accept", /\Wapplication\/json\W/)
	HeaderRegexps map[string][]string

	// Custom predicates to match.
	// E.g. Traffic(.3)
	Predicates []*Predicate

	// Set of filters in a particular route.
	// E.g. redirect(302, "https://www.example.org/hello")
	Filters []*Filter

	// Indicates that the parsed route has a shunt backend.
	// (<shunt>, no forwarding to a backend)
	//
	// Deprecated, use the BackendType field instead.
	Shunt bool

	// Indicates that the parsed route is a shunt, loopback or
	// it is forwarding to a network backend.
	BackendType BackendType

	// The address of a backend for a parsed route.
	// E.g. "https://www.example.org"
	Backend string

	// LBAlgorithm stores the name of the load balancing algorithm
	// in case of load balancing backends.
	LBAlgorithm string

	// LBEndpoints stores one or more backend endpoint in case of
	// load balancing backends.
	LBEndpoints []string

	// Name is deprecated and not used.
	Name string

	// Namespace is deprecated and not used.
	Namespace string
}

type RoutePredicate func(*Route) bool

// RouteInfo contains a route id, plus the loaded and parsed route or
// the parse error in case of failure.
type RouteInfo struct {
	// The route id plus the route data or if parsing was successful.
	Route

	// The parsing error if the parsing failed.
	ParseError error
}

// Copy copies a filter to a new filter instance. The argument values are copied in a shallow way.
func (f *Filter) Copy() *Filter {
	c := *f
	c.Args = make([]interface{}, len(f.Args))
	copy(c.Args, f.Args)
	return &c
}

// Copy copies a predicate to a new filter instance. The argument values are copied in a shallow way.
func (p *Predicate) Copy() *Predicate {
	c := *p
	c.Args = make([]interface{}, len(p.Args))
	copy(c.Args, p.Args)
	return &c
}

// Copy copies a route to a new route instance with all the slice and map fields copied deep.
func (r *Route) Copy() *Route {
	c := *r

	if len(r.HostRegexps) > 0 {
		c.HostRegexps = make([]string, len(r.HostRegexps))
		copy(c.HostRegexps, r.HostRegexps)
	}

	if len(r.PathRegexps) > 0 {
		c.PathRegexps = make([]string, len(r.PathRegexps))
		copy(c.PathRegexps, r.PathRegexps)
	}

	if len(r.Headers) > 0 {
		c.Headers = make(map[string]string)
		for k, v := range r.Headers {
			c.Headers[k] = v
		}
	}

	if len(r.HeaderRegexps) > 0 {
		c.HeaderRegexps = make(map[string][]string)
		for k, vs := range r.HeaderRegexps {
			c.HeaderRegexps[k] = make([]string, len(vs))
			copy(c.HeaderRegexps[k], vs)
		}
	}

	if len(r.Predicates) > 0 {
		c.Predicates = make([]*Predicate, len(r.Predicates))
		for i, p := range r.Predicates {
			c.Predicates[i] = p.Copy()
		}
	}

	if len(r.Filters) > 0 {
		c.Filters = make([]*Filter, len(r.Filters))
		for i, p := range r.Filters {
			c.Filters[i] = p.Copy()
		}
	}

	if len(r.LBEndpoints) > 0 {
		c.LBEndpoints = make([]string, len(r.LBEndpoints))
		copy(c.LBEndpoints, r.LBEndpoints)
	}

	return &c
}

// BackendTypeFromString parses the string representation of a backend type definition.
func BackendTypeFromString(s string) (BackendType, error) {
	switch s {
	case "", "network":
		return NetworkBackend, nil
	case "shunt":
		return ShuntBackend, nil
	case "loopback":
		return LoopBackend, nil
	case "dynamic":
		return DynamicBackend, nil
	case "lb":
		return LBBackend, nil
	default:
		return -1, fmt.Errorf("unsupported backend type: %s", s)
	}
}

// String returns the string representation of a backend type definition.
func (t BackendType) String() string {
	switch t {
	case NetworkBackend:
		return "network"
	case ShuntBackend:
		return "shunt"
	case LoopBackend:
		return "loopback"
	case DynamicBackend:
		return "dynamic"
	case LBBackend:
		return "lb"
	default:
		return "unknown"
	}
}

// Expects exactly n arguments of type string, or fails.
func getStringArgs(n int, args []interface{}) ([]string, error) {
	if len(args) != n {
		return nil, invalidPredicateArgCountError
	}

	sargs := make([]string, n)
	for i, a := range args {
		if sa, ok := a.(string); ok {
			sargs[i] = sa
		} else {
			return nil, invalidPredicateArgError
		}
	}

	return sargs, nil
}

// Checks and sets the different predicates taken from the yacc result.
// As the syntax is getting stabilized, this logic soon should be defined as
// yacc rules. (https://github.com/zalando/skipper/issues/89)
func applyPredicates(route *Route, proute *parsedRoute) error {
	var (
		err       error
		args      []string
		pathSet   bool
		methodSet bool
	)

	for _, m := range proute.matchers {
		if err != nil {
			return err
		}

		switch m.name {
		case "Path":
			if pathSet {
				return duplicatePathTreePredicateError
			}

			if args, err = getStringArgs(1, m.args); err == nil {
				route.Path = args[0]
				pathSet = true
			}
		case "Host":
			if args, err = getStringArgs(1, m.args); err == nil {
				route.HostRegexps = append(route.HostRegexps, args[0])
			}
		case "PathRegexp":
			if args, err = getStringArgs(1, m.args); err == nil {
				route.PathRegexps = append(route.PathRegexps, args[0])
			}
		case "Method":
			if methodSet {
				return duplicateMethodPredicateError
			}

			if args, err = getStringArgs(1, m.args); err == nil {
				route.Method = args[0]
				methodSet = true
			}
		case "HeaderRegexp":
			if args, err = getStringArgs(2, m.args); err == nil {
				if route.HeaderRegexps == nil {
					route.HeaderRegexps = make(map[string][]string)
				}

				route.HeaderRegexps[args[0]] = append(route.HeaderRegexps[args[0]], args[1])
			}
		case "Header":
			if args, err = getStringArgs(2, m.args); err == nil {
				if route.Headers == nil {
					route.Headers = make(map[string]string)
				}

				if _, ok := route.Headers[args[0]]; ok {
					return fmt.Errorf(duplicateHeaderPredicateErrorFmt, args[0])
				}

				route.Headers[args[0]] = args[1]
			}
		case "*", "Any":
			// void
		default:
			route.Predicates = append(
				route.Predicates,
				&Predicate{m.name, m.args})
		}
	}

	return err
}

// Converts a parsing route objects to the exported route definition with
// pre-processed but not validated matchers.
func newRouteDefinition(r *parsedRoute) (*Route, error) {
	rd := &Route{}

	rd.Id = r.id
	rd.Filters = r.filters
	rd.Shunt = r.shunt
	rd.Backend = r.backend
	rd.LBAlgorithm = r.lbAlgorithm
	rd.LBEndpoints = r.lbEndpoints

	switch {
	case r.shunt:
		rd.BackendType = ShuntBackend
	case r.loopback:
		rd.BackendType = LoopBackend
	case r.dynamic:
		rd.BackendType = DynamicBackend
	case r.lbBackend:
		rd.BackendType = LBBackend
	default:
		rd.BackendType = NetworkBackend
	}

	err := applyPredicates(rd, r)

	return rd, err
}

// executes the parser.
func parse(code string) ([]*parsedRoute, error) {
	l := newLexer(code)
	eskipParse(l)
	return l.routes, l.err
}

func partialRouteToRoute(format, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	return fmt.Sprintf(format, p)
}

// hacks a filter expression into a route expression for parsing.
func filtersToRoute(f string) string {
	return partialRouteToRoute("* -> %s -> <shunt>", f)
}

// hacks a predicate expression into a route expression for parsing.
func predicatesToRoute(p string) string {
	return partialRouteToRoute("%s -> <shunt>", p)
}

// Parses a route expression or a routing document to a set of route definitions.
func Parse(code string) ([]*Route, error) {
	parsedRoutes, err := parse(code)
	if err != nil {
		return nil, err
	}

	routeDefinitions := make([]*Route, len(parsedRoutes))
	for i, r := range parsedRoutes {
		rd, err := newRouteDefinition(r)
		if err != nil {
			return nil, err
		}

		routeDefinitions[i] = rd
	}

	return routeDefinitions, nil
}

func partialParse(f string, partialToRoute func(string) string) (*parsedRoute, error) {
	rs, err := parse(partialToRoute(f))
	if err != nil {
		return nil, err
	}

	if len(rs) == 0 {
		return nil, nil
	}

	return rs[0], nil
}

// Parses a filter chain into a list of parsed filter definitions.
func ParseFilters(f string) ([]*Filter, error) {
	r, err := partialParse(f, filtersToRoute)
	if r == nil || err != nil {
		return nil, err
	}

	return r.filters, nil
}

// ParsePredicates parses a set of predicates (combined by '&&') into
// a list of parsed predicate definitions.
func ParsePredicates(p string) ([]*Predicate, error) {
	r, err := partialParse(p, predicatesToRoute)
	if r == nil || err != nil {
		return nil, err
	}

	var ps []*Predicate
	for i := range r.matchers {
		if r.matchers[i].name != "*" {
			ps = append(ps, &Predicate{
				Name: r.matchers[i].name,
				Args: r.matchers[i].args,
			})
		}
	}

	return ps, nil
}

const randomIdLength = 16

var routeIdRx = regexp.MustCompile(`\W`)

// generate weak random id for a route if
// it doesn't have one.
func GenerateIfNeeded(existingId string) string {
	if existingId != "" {
		return existingId
	}

	// using this to avoid adding a new dependency.
	g, err := flowid.NewStandardGenerator(randomIdLength)
	if err != nil {
		return existingId
	}
	id, err := g.Generate()
	if err != nil {
		return existingId
	}

	// replace characters that are not allowed
	// for eskip route ids.
	id = routeIdRx.ReplaceAllString(id, "x")
	return "route" + id
}
