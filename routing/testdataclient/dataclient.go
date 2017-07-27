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
Package testdataclient provides a test implementation for the DataClient
interface of the skipper/routing package.

It uses in-memory route definitions that are passed in on construction,
and can upserted/deleted programmatically.
*/
package testdataclient

import (
	"errors"
	"github.com/zalando/skipper/eskip"
)

// DataClient implementation.
type Client struct {
	initDoc      string
	routes       map[string]*eskip.Route
	upsert       []*eskip.Route
	deletedIds   []string
	failNext     int
	signalUpdate chan int
}

// Creates a Client with an initial set of route definitions.
func New(initial []*eskip.Route) *Client {
	routes := make(map[string]*eskip.Route)
	for _, r := range initial {
		routes[r.Id] = r
	}

	return &Client{
		routes:       routes,
		signalUpdate: make(chan int)}
}

// Creates a Client with an initial set of route definitions in eskip
// format. If parsing the eskip document fails, returns an error.
func NewDoc(doc string) (*Client, error) {
	routes, err := eskip.Parse(doc)
	if err != nil {
		return nil, err
	}

	return New(routes), nil
}

// Returns the initial/current set of route definitions.
func (c *Client) LoadAll() ([]*eskip.Route, error) {
	if c.failNext > 0 {
		c.upsert, c.deletedIds = nil, nil
		c.failNext--
		return nil, errors.New("failed to get routes")
	}

	var routes []*eskip.Route
	for _, r := range c.routes {
		routes = append(routes, r)
	}

	return routes, nil
}

// Returns the route definitions upserted/deleted since the last call to
// LoadAll.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	<-c.signalUpdate

	for _, id := range c.deletedIds {
		delete(c.routes, id)
	}

	for _, r := range c.upsert {
		c.routes[r.Id] = r
	}

	if c.failNext > 0 {
		c.upsert, c.deletedIds = nil, nil
		c.failNext--
		return nil, nil, errors.New("failed to get routes")
	}

	var (
		u []*eskip.Route
		d []string
	)

	u, d, c.upsert, c.deletedIds = c.upsert, c.deletedIds, nil, nil
	return u, d, nil
}

// Updates the current set of routes with new/modified and deleted
// route definitions.
func (c *Client) Update(upsert []*eskip.Route, deletedIds []string) {
	c.upsert, c.deletedIds = upsert, deletedIds
	c.signalUpdate <- 42
}

// Updates the current set of routes with new/modified and deleted
// route definitions in eskip format. In case the parsing of the
// document fails, it returns an error.
func (c *Client) UpdateDoc(upsertDoc string, deletedIds []string) error {
	routes, err := eskip.Parse(upsertDoc)
	if err != nil {
		return err
	}

	c.Update(routes, deletedIds)
	return nil
}

// Sets the Client to fail on the next call to LoadAll or LoadUpdate.
// Repeated call to FailNext will result the Client to fail as many
// times as it was called.
func (c *Client) FailNext() {
	c.failNext++
}
