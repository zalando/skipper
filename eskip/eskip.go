// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eskip

//go:generate go tool yacc -o parser.go -p eskip parser.y

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/filters/flowid"
	"regexp"
	"strings"
)

const duplicateHeaderPredicateErrorFmt = "duplicate header predicate: %s"

var (
	invalidPredicateArgError        = errors.New("invalid predicate arg")
	invalidPredicateArgCountError   = errors.New("invalid predicate count arg")
	duplicatePathTreePredicateError = errors.New("duplicate path tree predicate")
	duplicateMethodPredicateError   = errors.New("duplicate method predicate")
)

// Represents a matcher condition for incoming requests.
type matcher struct {

	// The name of the matcher, e.g. Path or Header
	name string

	// The args of the matcher, e.g. the path to be matched.
	args []interface{}
}

// Route definition used during the parser processes the raw routing
// document.
type parsedRoute struct {
	id       string
	matchers []*matcher
	filters  []*Filter
	shunt    bool
	backend  string
}

type CustomPredicate struct {
	Name string
	Args []interface{}
}

// A Filter object represents a parsed, in-memory filter expression.
type Filter struct {

	// name of the filter specification
	Name string

	// filter args applied withing a particular route
	Args []interface{}
}

// A Route object represents a parsed, in-memory route definition.
type Route struct {

	// Id of the route definition.
	// E.g. route1: ...
	Id string

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

	CustomPredicates []*CustomPredicate

	// Set of filters in a particular route.
	// E.g. redirect(302, "https://www.example.org/hello")
	Filters []*Filter

	// Indicates that the parsed route has a shunt backend.
	// (<shunt>, no forwarding to a backend)
	Shunt bool

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

// Returns the first arg of a matcher with the given name.
// (Used for Path and Method.)
func getFirstMatcherString(r *parsedRoute, name string) (string, error) {
	for _, m := range r.matchers {
		if (m.name == name) && len(m.args) > 0 {
			p, ok := m.args[0].(string)
			if !ok {
				return "", invalidPredicateArgError
			}

			return p, nil
		}
	}

	return "", nil
}

// Returns all args of a matcher with the given name.
// (Used for PathRegexp and Host.)
func getMatcherStrings(r *parsedRoute, name string) ([]string, error) {
	var ss []string
	for _, m := range r.matchers {
		if m.name == name && len(m.args) > 0 {
			s, ok := m.args[0].(string)
			if !ok {
				return nil, invalidPredicateArgError
			}

			ss = append(ss, s)
		}
	}

	return ss, nil
}

// returns a map of the first args and all second args for a matcher
// with the given name. (Used for HeaderRegexps and Header.)
func getMatcherArgMap(r *parsedRoute, name string) (map[string][]string, error) {
	argMap := make(map[string][]string)
	for _, m := range r.matchers {
		if m.name == name && len(m.args) >= 2 {
			k, ok := m.args[0].(string)
			if !ok {
				return nil, invalidPredicateArgError
			}

			v, ok := m.args[1].(string)
			if !ok {
				return nil, invalidPredicateArgError
			}

			argMap[k] = append(argMap[k], v)
		}
	}

	return argMap, nil
}

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
			}
		case "HeaderRegexp":
			// TODO: use only regexps for the headers, and mark either Header or HeaderRegexp as deprecated
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
			// TODO: mark Any() as deprecated
			// void
		default:
			route.CustomPredicates = append(
				route.CustomPredicates,
				&CustomPredicate{m.name, m.args})
		}
	}

	return nil
}

// Converts a parsing route objects to the exported route definition with
// pre-processed but not validated matchers.
func newRouteDefinition(r *parsedRoute) (*Route, error) {
	rd := &Route{}

	rd.Id = r.id
	rd.Filters = r.filters
	rd.Shunt = r.shunt
	rd.Backend = r.backend

	err := applyPredicates(rd, r)

	return rd, err
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

	return fmt.Sprintf("Any() -> %s -> <shunt>", f)
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
