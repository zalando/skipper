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

package testdataclient

import (
	"errors"
	"github.com/zalando/skipper/eskip"
)

type Client struct {
	initDoc      string
	routes       map[string]*eskip.Route
	upsert       []*eskip.Route
	deletedIds   []string
	failNext     int
	signalUpdate chan int
}

func New(initial []*eskip.Route) *Client {
	routes := make(map[string]*eskip.Route)
	for _, r := range initial {
		routes[r.Id] = r
	}

	return &Client{
		routes:       routes,
		signalUpdate: make(chan int)}
}

func NewDoc(doc string) (*Client, error) {
	routes, err := eskip.Parse(doc)
	if err != nil {
		return nil, err
	}

	return New(routes), nil
}

func (c *Client) GetInitial() ([]*eskip.Route, error) {
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

func (c *Client) GetUpdate() ([]*eskip.Route, []string, error) {
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

func (c *Client) Update(upsert []*eskip.Route, deletedIds []string) {
	c.upsert, c.deletedIds = upsert, deletedIds
	c.signalUpdate <- 42
}

func (c *Client) UpdateDoc(upsertDoc string, deletedIds []string) error {
	routes, err := eskip.Parse(upsertDoc)
	if err != nil {
		return err
	}

	c.Update(routes, deletedIds)
	return nil
}

func (c *Client) FailNext() {
	c.failNext++
}
