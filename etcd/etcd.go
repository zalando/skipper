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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

const (
	routesPath      = "/routes"
	etcdIndexHeader = "X-Etcd-Index"
	defaultTimeout  = time.Second
)

// etcd serialization objects
type (
	node struct {
		Key           string  `json:"key"`
		Value         string  `json:"value"`
		Dir           bool    `json:"Dir"`
		ModifiedIndex uint64  `json:"modifiedIndex"`
		Nodes         []*node `json:"nodes"`
	}

	response struct {
		etcdIndex uint64
		Action    string `json:"action"`
		Node      *node  `json:"node"`
	}
)

// common error object for errors coming from multiple
// etcd instances
type endpointErrors struct {
	errors []error
}

func (ee *endpointErrors) Error() string {
	err := "request to one or more endpoints failed"

	for _, e := range ee.errors {
		err = err + ";" + e.Error()
	}

	return err
}

func (ee *endpointErrors) String() string {
	return ee.Error()
}

// Initialization options.
type Options struct {

	// A slice of etcd endpoint addresses.
	// (Schema and host.)
	Endpoints []string

	// Etcd path to a directory where the
	// Skipper related settings are stored.
	Prefix string

	// A timeout value for etcd long-polling.
	// The default timeout is 1 second.
	Timeout time.Duration

	// Skip TLS certificate check.
	Insecure bool
}

// A Client is used to load the whole set of routes and the updates from an
// etcd store.
type Client struct {
	endpoints  []string
	routesRoot string
	client     *http.Client
	etcdIndex  uint64
}

var (
	missingEtcdEndpoint     = errors.New("missing etcd endpoint")
	missingRouteId          = errors.New("missing route id")
	invalidNode             = errors.New("invalid node")
	unexpectedHttpResponse  = errors.New("unexpected http response")
	notFound                = errors.New("not found")
	missingEtcdIndex        = errors.New("missing etcd index")
	invalidResponseDocument = errors.New("invalid response document")
)

// Creates a new Client with the provided options.
func New(o Options) (*Client, error) {
	if len(o.Endpoints) == 0 {
		return nil, missingEtcdEndpoint
	}

	if o.Timeout == 0 {
		o.Timeout = defaultTimeout
	}

	httpClient := &http.Client{Timeout: o.Timeout}

	if o.Insecure {
		httpClient.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		}
	}

	return &Client{
		endpoints:  o.Endpoints,
		routesRoot: o.Prefix + routesPath,
		client:     httpClient,
		etcdIndex:  0}, nil
}

func isTimeout(err error) bool {
	nerr, ok := err.(net.Error)
	return ok && nerr.Timeout()
}

// Makes a request to an etcd endpoint. If it fails due to connection problems,
// it makes a new request to the next available endpoint, until all endpoints
// are tried. It returns the response to the first successful request.
func (c *Client) tryEndpoints(mreq func(string) (*http.Request, error)) (*http.Response, error) {
	var (
		req          *http.Request
		rsp          *http.Response
		err          error
		endpointErrs []error
	)

	for index, endpoint := range c.endpoints {
		req, err = mreq(endpoint + "/v2/keys")
		if err != nil {
			return nil, err
		}

		rsp, err = c.client.Do(req)

		isTimeoutError := false

		if err != nil {
			isTimeoutError = isTimeout(err)

			if !isTimeoutError {
				uerr, ok := err.(*url.Error)

				if ok && isTimeout(uerr.Err) {
					isTimeoutError = true
					err = uerr.Err
				}
			}
		}

		if err == nil || isTimeoutError {
			if index != 0 {
				c.endpoints = append(c.endpoints[index:], c.endpoints[:index]...)
			}

			return rsp, err
		}

		endpointErrs = append(endpointErrs, err)
	}

	return nil, &endpointErrors{endpointErrs}
}

// Converts an http response to a parsed etcd response object.
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

	if r.Node == nil || r.Node.Key == "" {
		return nil, invalidResponseDocument
	}

	r.etcdIndex, err = strconv.ParseUint(rsp.Header.Get(etcdIndexHeader), 10, 64)
	return r, err
}

// Converts a non-success http status code into an in-memory error object.
// As the second argument, returns true in case of error.
func httpError(code int) (error, bool) {
	if code == http.StatusNotFound {
		return notFound, true
	}

	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		return unexpectedHttpResponse, true
	}

	return nil, false
}

// Makes a request to an available etcd endpoint, with retries in case of
// failure, and converts the http response to a parsed etcd response object.
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

// Calls etcd 'watch' but with a timeout configured for
// the http client.
func (c *Client) etcdGetUpdates() (*response, error) {
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
// Returns a map where the keys are the etcd keys and the values are the
// eskip route expressions.
func (c *Client) iterateNodes(dir *node, highestIndex uint64) (map[string]string, uint64) {
	routes := make(map[string]string)
	for _, n := range dir.Nodes {
		if n.Dir {
			continue
		}

		routes[path.Base(n.Key)] = n.Value
		if n.ModifiedIndex > highestIndex {
			highestIndex = n.ModifiedIndex
		}
	}

	return routes, highestIndex
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
	allInfo := make([]*eskip.RouteInfo, 0, len(data))
	for id, d := range data {
		info := &eskip.RouteInfo{}

		r, err := parseOne(d)
		if err == nil {
			info.Route = *r
		} else {
			info.ParseError = err
		}

		info.Id = id
		allInfo = append(allInfo, info)
	}

	return allInfo
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

	if !response.Node.Dir {
		return nil, invalidNode
	}

	data, etcdIndex := c.iterateNodes(response.Node, 0)
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
// until the next change is detected in etcd or reaches the configured hard
// timeout.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	updates := make(map[string]string)
	deletes := make(map[string]bool)

	for {
		response, err := c.etcdGetUpdates()
		if isTimeout(err) {
			break
		} else if err != nil {
			return nil, nil, err
		} else if response.Node.Dir {
			if response.Node.ModifiedIndex > c.etcdIndex {
				c.etcdIndex = response.Node.ModifiedIndex
			}
			continue
		}

		id := path.Base(response.Node.Key)
		if response.Action == "delete" {
			deletes[id] = true
			delete(updates, id)
		} else {
			updates[id] = response.Node.Value
			deletes[id] = false
		}

		if response.Node.ModifiedIndex > c.etcdIndex {
			c.etcdIndex = response.Node.ModifiedIndex
		}
	}

	routeInfo := parseRoutes(updates)
	routes := infoToRoutesLogged(routeInfo)

	deletedIds := make([]string, 0, len(deletes))
	for id, deleted := range deletes {
		if deleted {
			deletedIds = append(deletedIds, id)
		}
	}

	return routes, deletedIds, nil
}

// Inserts or updates a route in etcd.
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
