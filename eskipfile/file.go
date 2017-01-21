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

It supports runtime updates of routes by watching changes to the file.

(See the DataClient interface in the skipper/routing package and the eskip
format in the skipper/eskip package.)
*/
package eskipfile

import (
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
	"github.com/rjeczalik/notify"
	"github.com/zalando/skipper/eskip"
)

// A Client contains the route definitions from an eskip file.
type Client struct {
	path     string
	routes   []*eskip.Route
	fsevents chan notify.EventInfo
}

// Open reads an eskip file and parses it, returning a DataClient implementation.
// If reading or parsing the file fails, returns an error.
func Open(p string) (*Client, error) {
	r, err := readRoutesFromFile(p)
	if err != nil {
		return nil, err
	}

	return &Client{routes: r, path: p}, nil
}

// Watch behaves like Open but it starts watching the eskip file for changes.
// This enabled runtime updates of routes when using this DataClient.
// The filesystem watcher created by this call should be properly Closed using this Client's Close function.
func Watch(p string) (*Client, error) {
	c, err := Open(p)
	if err != nil {
		return nil, err
	}
	c.fsevents = make(chan notify.EventInfo, 1)
	// We only care about Write events and Rename for "atomic" saves from some editors
	if err := notify.Watch(c.path, c.fsevents, notify.Write, notify.Rename); err != nil {
		return nil, err
	}
	return c, nil
}

// Close does a clean shutdown of the Client. It is not needed unless the Client was created using the
// Watch function, since it creates a filesystem watcher
func (c *Client) Close() {
	if c.fsevents != nil {
		notify.Stop(c.fsevents)
	}
}

func readRoutesFromFile(path string) ([]*eskip.Route, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return eskip.Parse(string(content))
}

func (c *Client) LoadAndParseAll() (routeInfos []*eskip.RouteInfo, err error) {
	for _, route := range c.routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{Route: *route})
	}
	return
}

// Returns the parsed route definitions found in the file.
func (c *Client) LoadAll() ([]*eskip.Route, error) { return c.routes, nil }

// LoadUpdate is No-Op if the Client was created using Open.
// When created using Watch, it monitors the filesystem for changes on the eskip file and enables runtime updates
// of the routes
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	if c.fsevents == nil {
		return nil, nil, nil
	}

	for {
		// TODO: should we have a timeout to bail every n secs or something?
		select {
		case _, ok := <-c.fsevents:
			if ok {
				log.Infof("detected changes in routes file %q, reloading ...", c.path)
				// TODO: do we care about deletes here?
				r, err := readRoutesFromFile(c.path)
				if err != nil {
					return nil, nil, err
				}
				return r, nil, nil
			}
		}
	}

	return nil, nil, nil
}
