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

package etcd

import (
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/eskip"
	"path"
	"strings"
)

const routesPath = "/routes"

type Client struct {
	routesRoot string
	etcd       *etcd.Client
	etcdIndex  uint64
}

func New(urls []string, storageRoot string) *Client {
	return &Client{storageRoot + routesPath, etcd.NewClient(urls), 0}
}

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

func getRoutes(data map[string]string) ([]*eskip.Route, error) {
	var routeDefs []string
	for _, r := range data {
		routeDefs = append(routeDefs, r)
	}

	doc := strings.Join(routeDefs, ";")
	return eskip.Parse(doc)
}

func getDeletedIds(data map[string]string) []string {
	var deletedIds []string
	for id, _ := range data {
		deletedIds = append(deletedIds, id)
	}

	return deletedIds
}

func (s *Client) GetInitial() ([]*eskip.Route, error) {
	response, err := s.etcd.Get(s.routesRoot, false, true)
	if err != nil {
		return nil, err
	}

	data, etcdIndex := s.iterateDefs(response.Node, 0)
	routes, err := getRoutes(data)
	if err != nil {
		return nil, err
	}

	s.etcdIndex = etcdIndex
	return routes, nil
}

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
		deletedIds = getDeletedIds(data)
	} else {
		routes, err = getRoutes(data)
		if err != nil {
			return nil, nil, err
		}
	}

	s.etcdIndex = etcdIndex
	return routes, deletedIds, nil
}
