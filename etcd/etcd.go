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

/*
Package etcd implements a DataClient for reading the skipper route
definitions from an etcd service.

(See the DataClient interface in the skipper/routing package.)

etcd is a generic, distributed configuration service:
https://github.com/coreos/etcd. The route definitions are stored under
individual keys as eskip route expressions. When loaded from etcd, the
routes will get the etcd key as id.

In addition to the DataClient implementation, type Client provides
methods to Upsert and Delete routes.

The etcd packages need to be downloaded separately before running the
tests, because the skipper program itself doesn't depend on it, only the
tests.
*/
package etcd

import (
	"errors"
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/eskip"
	"log"
	"net/http"
	"path"
)

const routesPath = "/routes"

// RouteInfo contains a route id, plus the loaded and parsed route or
// the parse error in case of failure.
type RouteInfo struct {

	// The route id plus the route data or if parsing was successful.
	eskip.Route

	// The parsing error if the parsing failed.
	ParseError error
}

// A Client is used to load the whole set of routes and the updates from an
// etcd store.
type Client struct {
	routesRoot string
	etcd       *etcd.Client
	etcdIndex  uint64
}

var missingRouteId = errors.New("missing route id")

// Creates a new Client, connecting to an etcd cluster reachable at 'urls'.
// The prefix argument specifies the etcd node under which the skipper
// routes are stored. E.g. if prefix is '/skipper-dev', the route
// definitions should be stored under /v2/keys/skipper-dev/routes/...
func New(urls []string, prefix string) *Client {
	return &Client{prefix + routesPath, etcd.NewClient(urls), 0}
}

// Finds all route expressions in the containing directory node.
// Prepends the expressions with the etcd key as the route id.
// Returns a map where the keys are the etcd keys and the values are the
// eskip route definitions.
func (c *Client) iterateDefs(n *etcd.Node, highestIndex uint64) (map[string]string, uint64) {
	if n.ModifiedIndex > highestIndex {
		highestIndex = n.ModifiedIndex
	}

	routes := make(map[string]string)
	if n.Key == c.routesRoot {
		for _, ni := range n.Nodes {
			routesi, hi := c.iterateDefs(ni, highestIndex)
			for id, r := range routesi {
				routes[id] = r
			}

			highestIndex = hi
		}
	}

	if path.Dir(n.Key) != c.routesRoot {
		return routes, highestIndex
	}

	id := path.Base(n.Key)
	r := id + ": " + n.Value
	return map[string]string{id: r}, highestIndex
}

// Parses a single route expression, fails if more than one
// expressions in the data.
func parseOne(data string) (*eskip.Route, error) {
	r, err := eskip.Parse(data)
	if err != nil {
		return nil, err
	}

	if len(r) != 1 {
		return nil, errors.New("invalid route entry: multiple route expressions")
	}

	return r[0], nil
}

// Parses a set of eskip routes.
func parseRoutes(data map[string]string) []*RouteInfo {
	allInfo := make([]*RouteInfo, len(data))
	index := 0
	for id, d := range data {
		info := &RouteInfo{}

		r, err := parseOne(d)
		if err == nil {
			info.Route = *r
		} else {
			info.ParseError = err
		}

		info.Id = id

		allInfo[index] = info
		index++
	}

	return allInfo
}

// Collects all the ids from a set of routes.
func getRouteIds(data map[string]string) []string {
	ids := make([]string, len(data))
	index := 0
	for id, _ := range data {
		ids[index] = id
		index++
	}

	return ids
}

// Converts route info to route objects logging those whose
// parsing failed.
func infoToRoutesLogged(info []*RouteInfo) []*eskip.Route {
	var routes []*eskip.Route
	for _, ri := range info {
		if ri.ParseError == nil {
			routes = append(routes, &ri.Route)
		} else {
			log.Println("error while parsing routes", ri.Id, ri.ParseError)
		}
	}

	return routes
}

// Returns all the route definitions currently stored in etcd,
// or the parsing error in case of failure.
func (c *Client) LoadAndParseAll() ([]*RouteInfo, error) {
	response, err := c.etcd.Get(c.routesRoot, false, true)
	if err != nil {
		return nil, err
	}

	data, etcdIndex := c.iterateDefs(response.Node, 0)
	if response.EtcdIndex > etcdIndex {
		etcdIndex = response.EtcdIndex
	}

	c.etcdIndex = etcdIndex
	return parseRoutes(data), nil
}

// Returns all the route definitions currently stored in etcd.
func (c *Client) LoadAll() ([]*eskip.Route, error) {
	routeInfo, err := c.LoadAndParseAll()
	if err != nil {
		return nil, err
	}

	return infoToRoutesLogged(routeInfo), nil
}

// Returns the updates (upserts and deletes) since the last initial request
// or update.
//
// It uses etcd's watch functionality that results in blocking this call
// until the next change is detected in etcd.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	response, err := c.etcd.Watch(c.routesRoot, c.etcdIndex+1, true, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	data, etcdIndex := c.iterateDefs(response.Node, c.etcdIndex)
	if response.EtcdIndex > etcdIndex {
		etcdIndex = response.EtcdIndex
	}

	c.etcdIndex = etcdIndex

	var (
		routes     []*eskip.Route
		deletedIds []string
	)

	if response.Action == "delete" {
		deletedIds = getRouteIds(data)
	} else {
		routeInfo := parseRoutes(data)
		routes = infoToRoutesLogged(routeInfo)
	}

	return routes, deletedIds, nil
}

// Inserts or updates a routes in etcd.
func (c *Client) Upsert(r *eskip.Route) error {
	if r.Id == "" {
		return missingRouteId
	}

	_, err := c.etcd.Set(c.routesRoot+"/"+r.Id, r.String(), 0)
	return err
}

// Deletes a route from etcd.
func (c *Client) Delete(id string) error {
	if id == "" {
		return missingRouteId
	}

	response, err := c.etcd.RawDelete(c.routesRoot+"/"+id, false, false)
	if response.StatusCode == http.StatusNotFound {
		return nil
	}

	return err
}
