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

package routing

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"net/http"
	"time"
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

	// The backend scheme and host.
	Scheme, Host string

	// The preprocessed filter instances.
	Filters []*RouteFilter
}

// Routing ('router') instance providing live
// updatable request matching.
type Routing struct {
	getMatcher <-chan *matcher
}

// starts a goroutine that continuously feeds the latest routing settings
// on the output channel, and receives the next updated settings on the
// input channel.
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

// Initializes a new routing instance, and starts listening for route
// definition updates.
func New(o Options) *Routing {
	initialMatcher, _ := newMatcher(nil, MatchingOptionsNone)
	matchersIn, matchersOut := feedMatchers(o.UpdateBuffer, initialMatcher)
	go receiveRouteMatcher(o, matchersIn)
	return &Routing{matchersOut}
}

// Matches a request in the current routing tree.
//
// If the request matches a route, returns the route and a map of
// parameters constructed from the wildcard parameters in the path
// condition if any. If there is no match, it returns nil.
func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}
