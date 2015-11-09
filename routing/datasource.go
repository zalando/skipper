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
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"log"
	"net/url"
	"time"
)

type incomingType uint

const (
	incomingReset incomingType = iota
	incomingUpdate
)

type routeDefs map[string]*eskip.Route

type incomingData struct {
	typ            incomingType
	client         DataClient
	upsertedRoutes []*eskip.Route
	deletedIds     []string
}

// continously receives route definitions from a data client on the the output channel.
// The function does not return. When started, it request for the whole current set of
// routes, and continues polling for the subsequent updates. When a communication error
// occurs, it re-requests the whole valid set, and continues polling. Currently, the
// routes with the same id coming from different sources are merged in an
// undeterministic way, but this may change in the future.
func receiveFromClient(c DataClient, pollTimeout time.Duration, out chan<- *incomingData) {
	receiveInitial := func() {
		for {
			routes, err := c.LoadAll()
			if err != nil {
				log.Println("error while receiveing initial data;", err)
				time.Sleep(pollTimeout)
				continue
			}

			out <- &incomingData{incomingReset, c, routes, nil}
			return
		}
	}

	receiveUpdates := func() {
		for {
			time.Sleep(pollTimeout)
			routes, deletedIds, err := c.LoadUpdate()
			if err != nil {
				log.Println("error while receiving update;", err)
				return
			}

			if len(routes) > 0 || len(deletedIds) > 0 {
				out <- &incomingData{incomingUpdate, c, routes, deletedIds}
			}
		}
	}

	for {
		receiveInitial()
		receiveUpdates()
	}
}

// applies incoming route definitions to key/route map, where
// the keys are the route ids.
func applyIncoming(defs routeDefs, d *incomingData) routeDefs {
	if d.typ == incomingReset || defs == nil {
		defs = make(routeDefs)
	}

	if d.typ == incomingUpdate {
		for _, id := range d.deletedIds {
			delete(defs, id)
		}
	}

	if d.typ == incomingReset || d.typ == incomingUpdate {
		for _, def := range d.upsertedRoutes {
			defs[def.Id] = def
		}
	}

	return defs
}

// merges the route definitions from multiple data clients by route id
func mergeDefs(defsByClient map[DataClient]routeDefs) []*eskip.Route {
	mergeById := make(routeDefs)
	for _, defs := range defsByClient {
		for id, def := range defs {
			mergeById[id] = def
		}
	}

	var all []*eskip.Route
	for _, def := range mergeById {
		all = append(all, def)
	}

	return all
}

// receives the initial set of the route definitiosn and their
// updates from multiple data clients, merges them by route id
// and sends the merged route definitions to the output channel.
//
// The active set of routes from last successful update are used until the
// next successful update.
func receiveRouteDefs(o Options) <-chan []*eskip.Route {
	in := make(chan *incomingData)
	out := make(chan []*eskip.Route)
	defsByClient := make(map[DataClient]routeDefs)

	for _, c := range o.DataClients {
		go receiveFromClient(c, o.PollTimeout, in)
	}

	go func() {
		for {
			incoming := <-in
			c := incoming.client
			defsByClient[c] = applyIncoming(defsByClient[c], incoming)
			out <- mergeDefs(defsByClient)
		}
	}()

	return out
}

// splits the backend address of a route definition into separate
// scheme and host variables.
func splitBackend(r *eskip.Route) (string, string, error) {
	if r.Shunt {
		return "", "", nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return "", "", err
	}

	return bu.Scheme, bu.Host, nil
}

// creates a filter instance based on its definition and its
// specification in the filter registry.
func createFilter(fr filters.Registry, def *eskip.Filter) (filters.Filter, error) {
	spec, ok := fr[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

// creates filter instances based on their definition
// and the filter registry.
func createFilters(fr filters.Registry, defs []*eskip.Filter) ([]*Filter, error) {
	var fs []*Filter
	for i, def := range defs {
		f, err := createFilter(fr, def)
		if err != nil {
			return nil, err
		}

		fs = append(fs, &Filter{f, def.Name, i})
	}

	return fs, nil
}

// processes a route definition for the routing table
func processRouteDef(fr filters.Registry, def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, def.Filters)
	if err != nil {
		return nil, err
	}

	return &Route{*def, scheme, host, fs}, nil
}

// processes a set of route definitions for the routing table
func processRouteDefs(fr filters.Registry, defs []*eskip.Route) []*Route {
	var routes []*Route
	for _, def := range defs {
		route, err := processRouteDef(fr, def)
		if err == nil {
			routes = append(routes, route)
		} else {
			log.Println(err)
		}
	}

	return routes
}

// receives the next version of the routing table on the output channel,
// when an update is received on one of the data clients.
func receiveRouteMatcher(o Options, out chan<- *matcher) {
	updates := receiveRouteDefs(o)
	for {
		defs := <-updates
		routes := processRouteDefs(o.FilterRegistry, defs)
		m, errs := newMatcher(routes, o.MatchingOptions)
		for _, err := range errs {
			log.Println(err)
		}

		out <- m
	}
}
