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

// Package requestmatch implements matching http requests to associated values.
//
// Matching is based on the attributes of http requests, where a request matches
// a definition if it fulfills all condition in it. The evaluation happens in the
// following order:
//
// 1. The request path is used to find leaf definitions in a lookup tree. If no
// path match was found, the leaf definitions in the root are taken that don't
// have a condition for path matching.
//
// 2. If any leaf definitions were found, they are evaluated against the request
// and the associated value of the first matching definition is returned. The order
// of the evaluation happens from the strictest definition to the least strict
// definition, where strictness is proportional to the number of non-empty
// conditions in the definition.
//
// Path matching supports two kind of wildcards:
//
// - a simple wildcard matches a single tag in a path. E.g: /users/:name/roles
// will be matched by /users/jdoe/roles, and the value of the parameter 'name' will
// be 'jdoe'
//
// - a freeform wildcard matches the last segment of a path, with any number of
// tags in it. E.g: /assets/*assetpath will be matched by /assets/images/logo.png,
// and the value of the parameter 'assetpath' will be '/images/logo.png'.

/*
mathcing http requests to skipper routes

using an internal lookup tree

matches if all conditions fulfilled in the route

evaluation order:

1. path in the lookup tree for leaf definitions, if no match leaf definitions in the
root. root leaf that have no path condition (no regexp)

2. in the leaf matching based on the rest of the conditions, from the most strict to the
least strict

path matching supports two kinds of wildcards

- simple wildcard matching a single name in the path

- freeform wildcard at the end of the path matching multiple names

wildcards in the response

routing definitions from data clients, merged, poll timeout

route definitions converted to routes with real filter instances using the registry

tail slash option
*/
package routing

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"net/http"
	"time"
)

type MatchingOptions uint

const (
	MatchingOptionsNone MatchingOptions = 0
	IgnoreTrailingSlash MatchingOptions = 1 << iota
)

func (o MatchingOptions) ignoreTrailingSlash() bool {
	return o&IgnoreTrailingSlash > 0
}

type DataClient interface {
	GetInitial() ([]*eskip.Route, error)
	GetUpdate() ([]*eskip.Route, []string, error)
}

type Options struct {
	FilterRegistry  filters.Registry
	MatchingOptions MatchingOptions
	PollTimeout     time.Duration
	DataClients     []DataClient
	UpdateBuffer    int
}

type Route struct {
	eskip.Route
	Scheme, Host string
	Filters      []filters.Filter
}

type Routing struct {
	getMatcher <-chan *matcher
}

func feedMatchers(updateBuffer int, current *matcher) (chan<- *matcher, <-chan *matcher) {
	// todo: use updateBuffer, when benchmarks show that it matters
	in := make(chan *matcher)
	out := make(chan *matcher, 0)

	go func() {
		for {
			select {
			case current = <-in:
			case out <- current:
			}
		}
	}()

	return in, out
}

func New(o Options) *Routing {
	initialMatcher, _ := newMatcher(nil, MatchingOptionsNone)
	matchersIn, matchersOut := feedMatchers(o.UpdateBuffer, initialMatcher)
	go receiveRouteMatcher(o, matchersIn)
	return &Routing{matchersOut}
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}
