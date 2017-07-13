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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"gopkg.in/fsnotify.v1"
)

const (
	defaultEskipFileExtension = ".eskip"
)

// A Client contains the route definitions from an eskip file.
type Client struct {
	routes       map[string][]*eskip.Route
	directories  []string
	updates      map[string]bool
	deletes      map[string]bool
	creates      map[string]bool
	mutex        *sync.Mutex
	watchChanges bool
}

func checkIfEskipFile(path string) bool {
	return strings.HasSuffix(path, defaultEskipFileExtension)
}

func parseFile(path string) ([]*eskip.Route, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(content))
	if err != nil {
		return nil, err
	}

	return routes, nil
}

// Opens an eskip file and parses it, returning a DataClient implementation.
// If reading or parsing the file fails, returns an error.
func Open(path string) (*Client, error) {

	f, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	routes := map[string][]*eskip.Route{}
	var directories []string

	switch mode := f.Mode(); {
	case mode.IsDir():
		filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.Mode().IsDir() {
				directories = append(directories, p)

				return nil
			}

			if !checkIfEskipFile(p) {
				return nil
			}

			log.WithFields(log.Fields{
				"path": p,
			}).Debugln("Reading file for routes")

			parsedRoutes, err := parseFile(p)

			if err != nil {
				return err
			}

			routes[p] = parsedRoutes

			return nil
		})
	case mode.IsRegular():
		parsedRoutes, err := parseFile(path)
		if err != nil {
			return nil, err
		}

		routes[path] = parsedRoutes
	}

	return &Client{
		routes:       routes,
		directories:  directories,
		watchChanges: false,
	}, nil
}

func Watch(path string) (*Client, error) {
	client, err := Open(path)

	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Warningln("Could not create a watcher")

		// could not create a watcher for the file but do not fail
		return client, nil
	}

	client.mutex = &sync.Mutex{}
	client.watchChanges = true

	go func() {
		defer watcher.Close()

		log.WithFields(log.Fields{
			"path": path,
		}).Debugln("Started watching file")

		for {
			select {
			case event := <-watcher.Events:
				client.mutex.Lock()

				if event.Op&fsnotify.Write == fsnotify.Write {
					client.handleUpdate(event.Name)
				} else if event.Op&fsnotify.Remove == fsnotify.Remove {
					client.handleDelete(event.Name)
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					f, err := os.Stat(event.Name)

					if err != nil {
						continue
					}

					if f.Mode().IsDir() {
						watcher.Add(event.Name)
					} else {
						client.handleCreate(event.Name)
					}
				} else if event.Op&fsnotify.Rename == fsnotify.Rename {
					/*
						handle rename event as delete

						let's assume './' directory is watched and '../' is not
						there are two cases:

						1. mv ./example.eskip ./example2.eskip

						we receive a rename event for example.eskip and
						a create event for example2.eskip which will delete
						routes and re-create again.

						2. mv ./example.eskip ..

						we receive only a rename event. as the parent folder
						is not watched we delete the routes in example.eskip
						and move on.
					*/
					client.handleDelete(event.Name)
				}
				client.mutex.Unlock()
			case err := <-watcher.Errors:
				log.WithFields(log.Fields{
					"error": err,
					"path":  path,
				}).Warningln("Received an error while watching")
				break
			}
		}
	}()

	for _, directory := range client.directories {
		err = watcher.Add(directory)

		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"path":  directory,
			}).Warningln("Could not add file to watcher")
		} else {
			log.WithFields(log.Fields{
				"path": directory,
			}).Debugln("Added watcher for the file")
		}
	}

	return client, nil
}

func (c *Client) handleCreate(path string) {
	if !checkIfEskipFile(path) {
		return
	}

	c.creates[path] = true
}

func (c *Client) handleUpdate(path string) {
	if !checkIfEskipFile(path) {
		return
	}

	_, isJustCreated := c.creates[path]

	if isJustCreated {
		return
	}

	value, ok := c.updates[path]
	if !value || !ok {
		c.updates[path] = true
	}
}

func (c *Client) handleDelete(path string) {
	if !checkIfEskipFile(path) {
		return
	}

	_, isJustCreated := c.creates[path]

	if isJustCreated {
		delete(c.creates, path)

		// no need to create it. so just return
		return
	}

	_, isJustUpdated := c.updates[path]

	if isJustUpdated {
		// ignore updates but delete the routes created before
		// so do not return
		delete(c.updates, path)
	}

	value, ok := c.deletes[path]
	if !value || !ok {
		c.deletes[path] = true
	}
}

func (c *Client) cleanupUpdates() {
	c.creates = map[string]bool{}
	c.updates = map[string]bool{}
	c.deletes = map[string]bool{}
}

func copyRoutes(source map[string][]*eskip.Route) map[string][]*eskip.Route {
	routes := map[string][]*eskip.Route{}

	for path, routesInPath := range source {
		routes[path] = routesInPath
	}

	return routes
}

func hasRouteWithId(routes []*eskip.Route, id string) bool {
	for _, route := range routes {
		if route.Id == id {
			return true
		}
	}

	return false
}

func (c *Client) getAllRoutes() (allRoutes []*eskip.Route, err error) {
	for _, routes := range c.routes {
		for _, route := range routes {
			allRoutes = append(allRoutes, route)
		}
	}
	return
}

func (c *Client) LoadAndParseAll() (routeInfos []*eskip.RouteInfo, err error) {
	routes, err := c.getAllRoutes()

	if err != nil {
		return nil, err
	}

	for _, route := range routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{Route: *route})
	}

	return
}

// Returns the parsed route definitions found in the file.
func (c *Client) LoadAll() (allRoutes []*eskip.Route, err error) {
	return c.getAllRoutes()
}

// Noop. The current implementation doesn't support watching the eskip
// file for changes.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	if !c.watchChanges {
		return nil, nil, nil
	}

	c.mutex.Lock()

	defer c.cleanupUpdates()
	defer c.mutex.Unlock()

	var routes []*eskip.Route
	var deletedRouteIds []string

	copyOfCurrentRoutes := copyRoutes(c.routes)

	for createdFilePath := range c.creates {
		parsedRoutes, err := parseFile(createdFilePath)

		if err != nil {
			return nil, nil, err
		}

		copyOfCurrentRoutes[createdFilePath] = parsedRoutes

		for _, route := range parsedRoutes {
			routes = append(routes, route)
		}
	}

	for updatedFilePath := range c.updates {
		parsedRoutes, err := parseFile(updatedFilePath)
		oldRoutes, ok := c.routes[updatedFilePath]

		if err != nil || !ok {
			return nil, nil, err
		}

		copyOfCurrentRoutes[updatedFilePath] = parsedRoutes

		for _, route := range oldRoutes {
			if !hasRouteWithId(parsedRoutes, route.Id) {
				deletedRouteIds = append(deletedRouteIds, route.Id)
			}
		}

		for _, route := range parsedRoutes {
			routes = append(routes, route)
		}
	}

	for deletedFilePath := range c.deletes {
		deletedRoutes := copyOfCurrentRoutes[deletedFilePath]

		for _, deletedRoute := range deletedRoutes {
			deletedRouteIds = append(deletedRouteIds, deletedRoute.Id)
		}

		delete(copyOfCurrentRoutes, deletedFilePath)
	}

	c.routes = copyOfCurrentRoutes

	log.WithFields(log.Fields{
		"routes":          routes,
		"deletedRouteIds": deletedRouteIds,
	}).Debugln("Returning updates")

	return routes, deletedRouteIds, nil
}
