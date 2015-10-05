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

// Package eskip implements parsing route definitions for Skipper.
//
// For documentation of the definition language, please, refer to the
// Skipper documentation. (URL tbd)
//
// First time install:
//
//  go generate && go install
//
// (Once the parser has been generated with `go generate`, it is enough
// to call `go install` only.)
package eskip

//go:generate go tool yacc -o parser.go -p eskip parser.y

import "errors"

// Represents a matcher condition for incoming requests.
type matcher struct {

	// The name of the matcher, e.g. Path or Header
	name string

	// The arguments of the matcher, e.g. the path to be matched.
	args []interface{}
}

// Structure containing a routing filter and its arguments for a route.
type Filter struct {

	// name of the filter
	Name string

	// filter arguments specific for the route
	Args []interface{}
}

type parsedRoute struct {
	id       string
	matchers []*matcher
	filters  []*Filter
	shunt    bool
	backend  string
}

// Structure containing information for a route.
type Route struct {

	// id of the route
	Id string

	// path to be matched
	Path string

	// host regular expression to match
	HostRegexps []string

	// path regular expressions to match
	PathRegexps []string

	// method to match
	Method string

	// exact header definitions to match
	Headers map[string]string

	// header regular expressions to match
	HeaderRegexps map[string][]string

	// set of filters parsed for the route
	Filters []*Filter

	// indicates that the parsed route has shunt backend
	// (<shunt>, no forwarding to a backend
	Shunt bool

	// the address of a backend for a parsed route
	Backend string
}

func getMatcherString(r *parsedRoute, name string) (string, error) {
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

func getMatcherStrings(r *parsedRoute, name string) ([]string, error) {
	var rxs []string
	for _, m := range r.matchers {
		if m.name == name && len(m.args) > 0 {
			rx, ok := m.args[0].(string)
			if !ok {
				return nil, errors.New("invalid matcher argument")
			}

			rxs = append(rxs, rx)
		}
	}

	return rxs, nil
}

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

	withError(func() { rd.Path, err = getMatcherString(r, "Path") })
	withError(func() { rd.HostRegexps, err = getMatcherStrings(r, "Host") })
	withError(func() { rd.PathRegexps, err = getMatcherStrings(r, "PathRegexp") })
	withError(func() { rd.Method, err = getMatcherString(r, "Method") })
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

func parse(code string) ([]*parsedRoute, error) {
	l := newLexer(code)
	eskipParse(l)
	return l.routes, l.err
}

// Parses a route or a routing document to a set of routes.
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
