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
	errInvalidBackend               = errors.New("invalid backend")
)

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
)

// Route definition used during the parser processes the raw routing
// document.
type parsedRoute struct {
	id       string
	matchers []*matcher
	filters  []*Filter
	shunt    bool
	loopback bool
	backend  string
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

	// filter args applied withing a particular route
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

func (t BackendType) String() string {
	switch t {
	case NetworkBackend:
		return "network"
	case ShuntBackend:
		return "shunt"
	case LoopBackend:
		return "loopback"
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

	bt, err := backendType(r.shunt, r.loopback)
	if err != nil {
		return nil, err
	}

	rd.BackendType = bt

	err = applyPredicates(rd, r)

	return rd, err
}

func backendType(shunt, loopback bool) (bt BackendType, err error) {
	if shunt && loopback {
		err = errInvalidBackend
		return
	}

	if shunt {
		bt = ShuntBackend
	} else if loopback {
		bt = LoopBackend
	} else {
		bt = NetworkBackend
	}

	return
}

// executes the parser.
func parse(code string) ([]*parsedRoute, error) {
	l := newLexer(code)
	eskipParse(l)
	return l.routes, l.err
}

// hacks a filter expression into a route expression for parsing.
func filtersToRoute(f string) string {
	f = strings.TrimSpace(f)
	if f == "" {
		return ""
	}

	return fmt.Sprintf("* -> %s -> <shunt>", f)
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

// Parses a filter chain into a list of parsed filter definitions.
func ParseFilters(f string) ([]*Filter, error) {
	rs, err := parse(filtersToRoute(f))
	if err != nil {
		return nil, err
	}

	if len(rs) == 0 {
		return nil, nil
	}

	return rs[0].filters, nil
}

const randomIdLength = 16

var routeIdRx = regexp.MustCompile("\\W")

// generate weak random id for a route if
// it doesn't have one.
func GenerateIfNeeded(existingId string) string {
	if existingId != "" {
		return existingId
	}

	// using this to avoid adding a new dependency.
	id, err := flowid.NewFlowId(randomIdLength)
	if err != nil {
		return existingId
	}

	// replace characters that are not allowed
	// for eskip route ids.
	id = routeIdRx.ReplaceAllString(id, "x")
	return "route" + id
}
