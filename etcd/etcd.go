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

etcd is a generic, ditributed configuation service:
https://github.com/coreos/etcd. The route definitions are stored under
individual keys as eskip route expressions. When loaded from etcd, the
routes will get the etcd key as id.

Note to contributors: this package requires etcd for running the tests.

    go get github.com/coreos/etcd

The etcd packages need to be downloaded separately before running the
tests, because the skipper program itself doesn't depend on it, only the
tests.
*/
package etcd

import (
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/eskip"
	"path"
	"strings"
)

const routesPath = "/routes"

// A Client is used to load the whole set of routes and the updates from an
// etcd store.
type Client struct {
	routesRoot string
	etcd       *etcd.Client
	etcdIndex  uint64
}

// Creates a new Client, connecting to an etcd cluster reachable at 'urls'.
// The storage root argument specifies the etcd node under which the skipper
// routes are stored. E.g. if storageRoot is '/skipper-dev', the route
// definitions should be stored under /v2/keys/skipper-dev/routes/...
func New(urls []string, storageRoot string) *Client {
	return &Client{storageRoot + routesPath, etcd.NewClient(urls), 0}
}

// Finds all route expressions in the containing directory node.
// Prepends the expressions with the etcd key as the route id.
// Returns a map where the keys are the etcd keys and the values are the
// eskip route definitions.
func (s *Client) iterateDefs(n *etcd.Node, highestIndex uint64) (map[string]string, uint64) {
	if n.ModifiedIndex > highestIndex {
		highestIndex = n.ModifiedIndex
	}

	routes := make(map[string]string)
	if n.Key == s.routesRoot {
		for _, ni := range n.Nodes {
			routesi, hi := s.iterateDefs(ni, highestIndex)
			for id, r := range routesi {
				routes[id] = r
			}

			highestIndex = hi
		}
	}

	if path.Dir(n.Key) != s.routesRoot {
		return routes, highestIndex
	}

	id := path.Base(n.Key)
	r := id + ":" + n.Value
	return map[string]string{id: r}, highestIndex
}

// Parses a set of eskip routes.
func parseRoutes(data map[string]string) ([]*eskip.Route, error) {
	var routeDefs []string
	for _, r := range data {
		routeDefs = append(routeDefs, r)
	}

	doc := strings.Join(routeDefs, ";")
	return eskip.Parse(doc)
}

// Collects all the ids from a set of routes.
func getRouteIds(data map[string]string) []string {
	var ids []string
	for id, _ := range data {
		ids = append(ids, id)
	}

	return ids
}

// Returns all the route definitions currently stored in etcd.
func (s *Client) GetInitial() ([]*eskip.Route, error) {
	response, err := s.etcd.Get(s.routesRoot, false, true)
	if err != nil {
		return nil, err
	}

	data, etcdIndex := s.iterateDefs(response.Node, 0)
	routes, err := parseRoutes(data)
	if err != nil {
		return nil, err
	}

	if response.EtcdIndex > etcdIndex {
		etcdIndex = response.EtcdIndex
	}

	s.etcdIndex = etcdIndex
	return routes, nil
}

// Returns the updates (upserts and deletes) since the last initial request
// or update.
//
// It uses etcd's watch functionality that results in blocking this call
// until the next change is detected in etcd.
func (s *Client) GetUpdate() ([]*eskip.Route, []string, error) {
	response, err := s.etcd.Watch(s.routesRoot, s.etcdIndex+1, true, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	data, etcdIndex := s.iterateDefs(response.Node, s.etcdIndex)
	var (
		routes     []*eskip.Route
		deletedIds []string
	)

	if response.Action == "delete" {
		deletedIds = getRouteIds(data)
	} else {
		routes, err = parseRoutes(data)
		if err != nil {
			return nil, nil, err
		}
	}

	if response.EtcdIndex > etcdIndex {
		etcdIndex = response.EtcdIndex
	}

	s.etcdIndex = etcdIndex
	return routes, deletedIds, nil
}
