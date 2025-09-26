package eskip

//go:generate goyacc -l -v "" -o parser.go -p eskip parser.y

import (
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

const duplicateHeaderPredicateErrorFmt = "duplicate header predicate: %s"

var (
	errDuplicatePathTreePredicate = errors.New("duplicate path tree predicate")
	errDuplicateMethodPredicate   = errors.New("duplicate method predicate")
)

// NewEditor creates an Editor PreProcessor, that matches routes and
// replaces the content. For example to replace Source predicates with
// ClientIP predicates you can use
// --edit-route='/Source[(](.*)[)]/ClientIP($1)/', which will change
// routes as you can see:
//
//	# input
//	r0: Source("127.0.0.1/8", "10.0.0.0/8") -> inlineContent("OK") -> <shunt>;
//	# actual route
//	edit_r0: ClientIP("127.0.0.1/8", "10.0.0.0/8") -> inlineContent("OK") -> <shunt>;
func NewEditor(reg *regexp.Regexp, repl string) *Editor {
	return &Editor{
		reg:  reg,
		repl: repl,
	}
}

type Editor struct {
	reg  *regexp.Regexp
	repl string
}

// NewClone creates a Clone PreProcessor, that matches routes and
// replaces the content of the cloned routes. For example to migrate from Source to
// ClientIP predicates you can use
// --clone-route='/Source[(](.*)[)]/ClientIP($1)/', which will change
// routes as you can see:
//
//	# input
//	r0: Source("127.0.0.1/8", "10.0.0.0/8") -> inlineContent("OK") -> <shunt>;
//	# actual route
//	clone_r0: ClientIP("127.0.0.1/8", "10.0.0.0/8") -> inlineContent("OK") -> <shunt>;
//	r0: Source("127.0.0.1/8", "10.0.0.0/8") -> inlineContent("OK") -> <shunt>;
func NewClone(reg *regexp.Regexp, repl string) *Clone {
	return &Clone{
		reg:  reg,
		repl: repl,
	}
}

type Clone struct {
	reg  *regexp.Regexp
	repl string
}

func (e *Editor) Do(routes []*Route) []*Route {
	if e.reg == nil {
		return routes
	}

	canonicalRoutes := CanonicalList(routes)

	for i, r := range canonicalRoutes {
		rr := new(Route)
		*rr = *r
		if doOneRoute(e.reg, e.repl, rr) {
			routes[i] = rr
		}
	}

	return routes
}

func (c *Clone) Do(routes []*Route) []*Route {
	if c.reg == nil {
		return routes
	}

	canonicalRoutes := CanonicalList(routes)

	result := make([]*Route, len(routes), 2*len(routes))
	copy(result, routes)
	for _, r := range canonicalRoutes {
		rr := new(Route)
		*rr = *r

		rr.Id = "clone_" + rr.Id
		predicates := make([]*Predicate, len(r.Predicates))
		for k, p := range r.Predicates {
			q := *p
			predicates[k] = &q
		}
		rr.Predicates = predicates

		filters := make([]*Filter, len(r.Filters))
		for k, f := range r.Filters {
			ff := *f
			filters[k] = &ff
		}
		rr.Filters = filters

		if doOneRoute(c.reg, c.repl, rr) {
			result = append(result, rr)
		}
	}

	return result
}

func doOneRoute(rx *regexp.Regexp, repl string, r *Route) bool {
	if rx == nil {
		return false
	}
	var changed bool

	for i, p := range r.Predicates {
		ps := p.String()
		pss := rx.ReplaceAllString(ps, repl)
		sps := string(pss)
		if ps == sps {
			continue
		}

		pp, err := ParsePredicates(sps)
		if err != nil {
			log.Errorf("Failed to parse predicate: %v", err)
			continue
		}

		r.Predicates[i] = pp[0]
		changed = true
	}

	for i, f := range r.Filters {
		fs := f.String()
		fss := rx.ReplaceAllString(fs, repl)
		sfs := string(fss)
		if fs == sfs {
			continue
		}

		ff, err := ParseFilters(sfs)
		if err != nil {
			log.Errorf("Failed to parse filter: %v", err)
			continue
		}

		r.Filters[i] = ff[0]
		changed = true
	}

	return changed
}

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

// BackendType indicates whether a route is a network backend, a shunt or a loopback.
type BackendType int

const (
	NetworkBackend = iota
	ShuntBackend
	LoopBackend
	DynamicBackend
	LBBackend
)

var errMixedProtocols = errors.New("loadbalancer endpoints cannot have mixed protocols")

// Route definition used during the parser processes the raw routing
// document.
type parsedRoute struct {
	id          string
	predicates  []*Predicate
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

func (p *Predicate) String() string {
	return fmt.Sprintf("%s(%s)", p.Name, argsString(p.Args))
}

// A Filter object represents a parsed, in-memory filter expression.
type Filter struct {
	// name of the filter specification
	Name string `json:"name"`

	// filter args applied within a particular route
	Args []interface{} `json:"args"`
}

func (f *Filter) String() string {
	return fmt.Sprintf("%s(%s)", f.Name, argsString(f.Args))
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
func getStringArgs(p *Predicate, n int) ([]string, error) {
	failure := func() ([]string, error) {
		if n == 1 {
			return nil, fmt.Errorf("%s predicate expects 1 string argument", p.Name)
		} else {
			return nil, fmt.Errorf("%s predicate expects %d string arguments", p.Name, n)
		}
	}

	if len(p.Args) != n {
		return failure()
	}

	sargs := make([]string, n)
	for i, a := range p.Args {
		if sa, ok := a.(string); ok {
			sargs[i] = sa
		} else {
			return failure()
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

	for _, p := range proute.predicates {
		switch p.Name {
		case "Path":
			if pathSet {
				return errDuplicatePathTreePredicate
			}

			if args, err = getStringArgs(p, 1); err == nil {
				route.Path = args[0]
				pathSet = true
			}
		case "Host":
			if args, err = getStringArgs(p, 1); err == nil {
				if strings.HasPrefix(args[0], "*.") {
					log.Infof("Host %q starts with '*.'; replacing with regex", args[0])
					args[0] = strings.Replace(args[0], "*", "[a-z0-9]+((-[a-z0-9]+)?)*", 1)
				}
				route.HostRegexps = append(route.HostRegexps, args[0])
			}
		case "PathRegexp":
			if args, err = getStringArgs(p, 1); err == nil {
				route.PathRegexps = append(route.PathRegexps, args[0])
			}
		case "Method":
			if methodSet {
				return errDuplicateMethodPredicate
			}

			if args, err = getStringArgs(p, 1); err == nil {
				route.Method = args[0]
				methodSet = true
			}
		case "HeaderRegexp":
			if args, err = getStringArgs(p, 2); err == nil {
				if route.HeaderRegexps == nil {
					route.HeaderRegexps = make(map[string][]string)
				}

				route.HeaderRegexps[args[0]] = append(route.HeaderRegexps[args[0]], args[1])
			}
		case "Header":
			if args, err = getStringArgs(p, 2); err == nil {
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
			route.Predicates = append(route.Predicates, p)
		}

		if err != nil {
			return fmt.Errorf("invalid route %q: %w", proute.id, err)
		}
	}

	return nil
}

// Converts a parsing route objects to the exported route definition with
// pre-processed but not validated matchers.
func newRouteDefinition(r *parsedRoute) (*Route, error) {
	if len(r.lbEndpoints) > 0 {
		scheme := ""
		for _, e := range r.lbEndpoints {
			eu, err := url.ParseRequestURI(e)
			if err != nil {
				return nil, err
			}

			if scheme != "" && scheme != eu.Scheme {
				return nil, errMixedProtocols
			}

			scheme = eu.Scheme
		}
	}

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

type eskipLexParser struct {
	lexer  eskipLex
	parser eskipParserImpl
}

var parserPool = sync.Pool{
	New: func() interface{} {
		return new(eskipLexParser)
	},
}

func parseDocument(code string) ([]*parsedRoute, error) {
	routes, _, _, err := parse(start_document, code)
	return routes, err
}

func parsePredicates(code string) ([]*Predicate, error) {
	_, predicates, _, err := parse(start_predicates, code)
	return predicates, err
}

func parseFilters(code string) ([]*Filter, error) {
	_, _, filters, err := parse(start_filters, code)
	return filters, err
}

func parse(start int, code string) ([]*parsedRoute, []*Predicate, []*Filter, error) {
	lp := parserPool.Get().(*eskipLexParser)
	defer func() {
		*lp = eskipLexParser{}
		parserPool.Put(lp)
	}()

	lexer := &lp.lexer
	lexer.init(start, code)

	lp.parser.Parse(lexer)

	// Do not return lexer to avoid reading lexer fields after returning eskipLexParser to the pool.
	// Let the caller decide which of return values to use based on the start token.
	return lexer.routes, lexer.predicates, lexer.filters, lexer.err
}

// Parses a route expression or a routing document to a set of route definitions.
func Parse(code string) ([]*Route, error) {
	parsedRoutes, err := parseDocument(code)
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

// MustParse parses a route expression or a routing document to a set of route definitions and
// panics in case of error
func MustParse(code string) []*Route {
	r, err := Parse(code)
	if err != nil {
		panic(err)
	}
	return r
}

// MustParsePredicates parses a set of predicates (combined by '&&') into
// a list of parsed predicate definitions and panics in case of error
func MustParsePredicates(s string) []*Predicate {
	p, err := ParsePredicates(s)
	if err != nil {
		panic(err)
	}
	return p
}

// MustParseFilters parses a set of filters (combined by '->') into
// a list of parsed filter definitions and panics in case of error
func MustParseFilters(s string) []*Filter {
	p, err := ParseFilters(s)
	if err != nil {
		panic(err)
	}
	return p
}

// Parses a filter chain into a list of parsed filter definitions.
func ParseFilters(f string) ([]*Filter, error) {
	f = strings.TrimSpace(f)
	if f == "" {
		return nil, nil
	}

	return parseFilters(f)
}

// ParsePredicates parses a set of predicates (combined by '&&') into
// a list of parsed predicate definitions.
func ParsePredicates(p string) ([]*Predicate, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil, nil
	}

	rs, err := parsePredicates(p)
	if err != nil {
		return nil, err
	}

	if len(rs) == 0 {
		return nil, nil
	}

	ps := make([]*Predicate, 0, len(rs))
	for _, p := range rs {
		if p.Name != "*" {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		ps = nil
	}

	return ps, nil
}

const (
	randomIdLength = 16
	// does not contain underscore to produce compatible output with previously used flow id generator
	alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// generate weak random id for a route if
// it doesn't have one.
//
// Deprecated: do not use, generate valid route id that matches [a-zA-Z_] yourself.
func GenerateIfNeeded(existingId string) string {
	if existingId != "" {
		return existingId
	}

	var sb strings.Builder
	sb.WriteString("route")

	for i := 0; i < randomIdLength; i++ {
		ai := rand.Intn(len(alphabet))
		sb.WriteByte(alphabet[ai])
	}

	return sb.String()
}
