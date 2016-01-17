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
*/
package etcd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
)

const (
	routesPath      = "/routes"
	etcdIndexHeader = "X-Etcd-Index"
)

type node struct {
	Key           string  `json:"key"`
	Value         string  `json:"value"`
	ModifiedIndex uint64  `json:"modifiedIndex"`
	Nodes         []*node `json:"nodes"`
}

type response struct {
	etcdIndex uint64
	Action    string `json:"action"`
	Node      *node  `json:"node"`
}

// A Client is used to load the whole set of routes and the updates from an
// etcd store.
type Client struct {
	routesRoot string
	addresses  []string
	client     *http.Client
	etcdIndex  uint64
}

var (
	missingEtcdAddress     = errors.New("missing etcd address")
	missingRouteId         = errors.New("missing route id")
	unexpectedHttpResponse = errors.New("unexpected http response")
	notFound               = errors.New("not found")
	missingEtcdIndex       = errors.New("missing etcd index")
)

// Creates a new Client, connecting to an etcd cluster reachable at 'urls'.
// The prefix argument specifies the etcd node under which the skipper
// routes are stored. E.g. if prefix is '/skipper-dev', the route
// definitions should be stored under /v2/keys/skipper-dev/routes/...
func New(addresses []string, prefix string) (*Client, error) {
	if len(addresses) == 0 {
		return nil, missingEtcdAddress
	}

	return &Client{
		routesRoot: prefix + routesPath,
		addresses:  addresses,
		client:     &http.Client{},
		etcdIndex:  0}, nil
}

type makeRequest func(string) (*http.Request, error)

func (c *Client) tryEndpoints(mreq makeRequest) (*http.Response, error) {
	var (
		addresses []string
		req       *http.Request
		rsp       *http.Response
		err       error
	)

	addresses = c.addresses
	for len(addresses) > 0 {
		req, err = mreq(addresses[0] + "/v2/keys")
		if err != nil {
			return nil, err
		}

		rsp, err = c.client.Do(req)
		if err == nil {
			break
		}

		addresses = addresses[1:]
	}

	rotate := len(c.addresses) - len(addresses)
	c.addresses = append(c.addresses[rotate:], c.addresses[:rotate]...)

	return rsp, err
}

func parseResponse(rsp *http.Response) (*response, error) {
	d, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	r := &response{}
	err = json.Unmarshal(d, &r)
	if err != nil {
		return nil, err
	}

	r.etcdIndex, err = strconv.ParseUint(rsp.Header.Get(etcdIndexHeader), 10, 64)
	return r, err
}

func httpError(code int) (error, bool) {
	if code == http.StatusNotFound {
		return notFound, true
	}

	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		return unexpectedHttpResponse, true
	}

	return nil, false
}

func (c *Client) etcdRequest(method, path, data string) (*response, error) {
	rsp, err := c.tryEndpoints(func(a string) (*http.Request, error) {
		var body io.Reader
		if data != "" {
			v := make(url.Values)
			v.Add("value", data)
			body = bytes.NewBufferString(v.Encode())
		}

		r, err := http.NewRequest(method, a+path, body)
		if err != nil {
			return nil, err
		}

		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r, nil
	})

	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	if err, hasErr := httpError(rsp.StatusCode); hasErr {
		return nil, err
	}

	return parseResponse(rsp)
}

func (c *Client) etcdGet() (*response, error) {
	return c.etcdRequest("GET", c.routesRoot, "")
}

func (c *Client) etcdWatch() (*response, error) {
	return c.etcdRequest("GET",
		fmt.Sprintf("%s?wait=true&waitIndex=%d&recursive=true",
			c.routesRoot, c.etcdIndex+1), "")
}

func (c *Client) etcdSet(r *eskip.Route) error {
	_, err := c.etcdRequest("PUT", c.routesRoot+"/"+r.Id, r.String())
	return err
}

func (c *Client) etcdDelete(id string) error {
	_, err := c.etcdRequest("DELETE", c.routesRoot+"/"+id, "")
	return err
}

// Finds all route expressions in the containing directory node.
// Prepends the expressions with the etcd key as the route id.
// Returns a map where the keys are the etcd keys and the values are the
// eskip route definitions.
func (c *Client) iterateDefs(n *node, highestIndex uint64) (map[string]string, uint64) {
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
func parseRoutes(data map[string]string) []*eskip.RouteInfo {
	allInfo := make([]*eskip.RouteInfo, len(data))
	index := 0
	for id, d := range data {
		info := &eskip.RouteInfo{}

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
func infoToRoutesLogged(info []*eskip.RouteInfo) []*eskip.Route {
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
func (c *Client) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	response, err := c.etcdGet()
	if err == notFound {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	data, etcdIndex := c.iterateDefs(response.Node, 0)
	if response.etcdIndex > etcdIndex {
		etcdIndex = response.etcdIndex
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
	response, err := c.etcdWatch()
	if err != nil {
		return nil, nil, err
	}

	// this is going to be the tricky part
	data, etcdIndex := c.iterateDefs(response.Node, c.etcdIndex)
	if response.etcdIndex > etcdIndex {
		etcdIndex = response.etcdIndex
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

	return c.etcdSet(r)
}

// Deletes a route from etcd.
func (c *Client) Delete(id string) error {
	if id == "" {
		return missingRouteId
	}

	err := c.etcdDelete(id)
	if err == notFound {
		err = nil
	}

	return err
}

func (c *Client) UpsertAll(routes []*eskip.Route) error {
	for _, r := range routes {
		r.Id = eskip.GenerateIfNeeded(r.Id)
		err := c.Upsert(r)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) DeleteAllIf(routes []*eskip.Route, cond eskip.RoutePredicate) error {
	for _, r := range routes {
		if !cond(r) {
			continue
		}

		err := c.Delete(r.Id)
		if err != nil {
			return err
		}
	}

	return nil
}
