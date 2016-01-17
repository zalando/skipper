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
Package eskipfile implements a DataClient for reading the skipper route
definitions from an eskip formatted file when opened.

(See the DataClient interface in the skipper/routing package and the eskip
format in the skipper/eskip package.)
*/
package eskipfile

import (
	"github.com/zalando/skipper/eskip"
	"io/ioutil"
)

// A Client contains the route definitions from an eskip file.
type Client struct{ routes []*eskip.Route }

// Opens an eskip file and parses it, returning a DataClient implementation.
// If reading or parsing the file fails, returns an error.
func Open(path string) (*Client, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(content))
	if err != nil {
		return nil, err
	}

	return &Client{routes}, nil
}

func (c Client) LoadAndParseAll() (routeInfos []*eskip.RouteInfo, err error) {
	for _, route := range c.routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{*route, nil})
	}
	return
}

// Returns the parsed route definitions found in the file.
func (c Client) LoadAll() ([]*eskip.Route, error) { return c.routes, nil }

// Noop. The current implementation doesn't support watching the eskip
// file for changes.
func (c Client) LoadUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }
