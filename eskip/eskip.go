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
	"strings"
)

// Represents a matcher condition for incoming requests.
type matcher struct {

	// The name of the matcher, e.g. Path or Header
	name string

	// The arguments of the matcher, e.g. the path to be matched.
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

// A Filter object represents a parsed, in-memory filter expression.
type Filter struct {

	// name of the filter specification
	Name string

	// filter arguments applied withing a particular route
	Args []interface{}
}

// A Route object represents a parsed, in-memory route expression.
type Route struct {

	// id of the route definition.
	// E.g. route1: ...
	Id string

	// exact path to be matched.
	// E.g. Path("/some/path")
	Path string

	// host regular expressions to match.
	// E.g. Host(/[.]example[.]org/)
	HostRegexps []string

	// path regular expressions to match.
	// E.g. PathRegexp(/\/api\//)
	PathRegexps []string

	// method to match.
	// E.g. Method("HEAD")
	Method string

	// exact header definitions to match.
	// E.g. Header("Accept", "application/json")
	Headers map[string]string

	// header regular expressions to match.
	// E.g. HeaderRegexp("Accept", /\Wapplication\/json\W/)
	HeaderRegexps map[string][]string

	// set of filters in a particular route.
	// E.g. redirect(302, "https://www.example.org/hello")
	Filters []*Filter

	// indicates that the parsed route has shunt backend
	// (<shunt>, no forwarding to a backend
	Shunt bool

	// the address of a backend for a parsed route.
	// E.g. "https://www.example.org"
	Backend string
}

// Returns the first argument of a matcher with the given name.
// (Used for Path and Method.)
func getFirstMatcherString(r *parsedRoute, name string) (string, error) {
	for _, m := range r.matchers {
		if (m.name == name) && len(m.args) > 0 {
			p, ok := m.args[0].(string)
			if !ok {
				return "", errors.New("invalid matcher argument")
			}

			return p, nil
		}
	}

	return "", nil
}

// Returns all arguments of a matcher with the given name.
// (Used for PathRegexp and Host.)
func getMatcherStrings(r *parsedRoute, name string) ([]string, error) {
	var ss []string
	for _, m := range r.matchers {
		if m.name == name && len(m.args) > 0 {
			s, ok := m.args[0].(string)
			if !ok {
				return nil, errors.New("invalid matcher argument")
			}

			ss = append(ss, s)
		}
	}

	return ss, nil
}

// returns a map of the first arguments and all second arguments for a matcher
// with the given name. (Used for HeaderRegexps and Header.)
func getMatcherArgMap(r *parsedRoute, name string) (map[string][]string, error) {
	argMap := make(map[string][]string)
	for _, m := range r.matchers {
		if m.name == name && len(m.args) >= 2 {
			k, ok := m.args[0].(string)
			if !ok {
				return nil, errors.New("invalid matcher key argument")
			}

			v, ok := m.args[1].(string)
			if !ok {
				return nil, errors.New("invalid matcher value argument")
			}

			argMap[k] = append(argMap[k], v)
		}
	}

	return argMap, nil
}

// Converts a parsing route objects to the exported route definition with
// pre-processed but not validated matchers.
func newRouteDefinition(r *parsedRoute) (*Route, error) {
	var err error
	withError := func(f func()) {
		if err != nil {
			return
		}

		f()
	}

	rd := &Route{}

	rd.Id = r.id
	rd.Filters = r.filters
	rd.Shunt = r.shunt
	rd.Backend = r.backend

	withError(func() { rd.Path, err = getFirstMatcherString(r, "Path") })
	withError(func() { rd.HostRegexps, err = getMatcherStrings(r, "Host") })
	withError(func() { rd.PathRegexps, err = getMatcherStrings(r, "PathRegexp") })
	withError(func() { rd.Method, err = getFirstMatcherString(r, "Method") })
	withError(func() { rd.HeaderRegexps, err = getMatcherArgMap(r, "HeaderRegexp") })

	withError(func() {
		var h map[string][]string
		h, err = getMatcherArgMap(r, "Header")
		if err == nil {
			rd.Headers = make(map[string]string)
			for k, v := range h {
				rd.Headers[k] = v[0]
			}
		}
	})

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
